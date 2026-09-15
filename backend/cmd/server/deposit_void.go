package main

// Ж1-и1 (ADMIN.md, трек Ж, решение Ж-Р4): отмена ошибочного депозита.
//
// Депозит был единственной денежной операцией стойки без отката (ревью 14.09,
// №4): продажу, выдачу пакета и минут за монеты админ отменяет сам, а «500
// вместо 50» жило в выручке, пока владелец не поправит базу руками. Правила
// — те же, что у отмены продажи (canVoidSale): свою и в текущих клубных
// сутках отменяет админ, чужую и вчерашнюю — владелец.
//
// Что можно забрать назад, решает planDepositVoid: с кошелька снимается
// остаток, но не больше суммы депозита — уже отыгранное долгом не становится
// (Р8 трека Г: долгов нет), а уходит в отчёт строкой «отыграно по ошибочному
// депозиту»; кэшбек-монеты за депозит забираются, но не ниже нуля.
//
// Порядок в транзакции важен: сперва строка гостя берётся под замок
// (FOR UPDATE — тем же замком пользуется walletApply), по ней считается план,
// потом CAS по voided_at IS NULL — двойной клик и две стойки дают ровно одну
// отмену, вторая получает 409.

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

var depositVoidErrors = map[string]string{
	"deposit_not_found": "Депозит не найден",
	"already_void":      "Депозит уже отменён",
	"not_yours":         "Чужой депозит отменяет владелец",
	"too_old":           "Депозит не с текущей смены — отменяет владелец",
	"db_error":          "Не удалось отменить депозит — попробуй ещё раз",
}

func depositVoidFail(c *gin.Context, status int, code string) {
	msg := depositVoidErrors[code]
	if msg == "" {
		msg = code
	}
	c.JSON(status, gin.H{"code": code, "message": msg})
}

// depositVoidPlan — что реально забирается при отмене.
type depositVoidPlan struct {
	Refund      int64 // грошей снять с кошелька (остаток, не больше суммы)
	Unrecovered int64 // грошей уже отыграно — в потери отчёта
	CoinsTaken  int64 // кэшбек-монет забрать (не ниже нуля на балансе)
}

// planDepositVoid — чистая арифметика отмены (тест). Кошелёк и монеты
// берутся по состоянию на момент отмены: если гость успел отыграть часть
// денег или потратить монеты, забирается только то, что осталось.
func planDepositVoid(walletGrosz, coinsBalance, amountGrosz, coinsGiven int64) depositVoidPlan {
	var p depositVoidPlan
	if amountGrosz > 0 && walletGrosz > 0 {
		p.Refund = min(walletGrosz, amountGrosz)
	}
	if amountGrosz > 0 {
		p.Unrecovered = amountGrosz - p.Refund
	}
	if coinsGiven > 0 && coinsBalance > 0 {
		p.CoinsTaken = min(coinsBalance, coinsGiven)
	}
	return p
}

// POST /admin/deposits/:id/void — отменить ошибочный депозит (staff).
func handleAdminDepositVoid(c *gin.Context) {
	var dep models.Deposit
	if err := db.First(&dep, "id = ?", c.Param("id")).Error; err != nil {
		depositVoidFail(c, http.StatusNotFound, "deposit_not_found")
		return
	}
	if dep.VoidedAt != nil {
		depositVoidFail(c, http.StatusConflict, "already_void")
		return
	}
	reportHour := int(settingInt64("report_hour", 8))
	from, to, _, _ := shiftWindow("", reportHour, time.Now())
	createdBy := ""
	if dep.CreatedBy != nil {
		createdBy = dep.CreatedBy.String()
	}
	if ok, code := canVoidSale(c.GetString("user_role"), createdBy,
		c.GetString("user_id"), dep.CreatedAt, from, to); !ok {
		depositVoidFail(c, http.StatusForbidden, code)
		return
	}

	adminID, _ := uuid.Parse(c.GetString("user_id"))
	now := time.Now()
	amountGrosz := models.GroszFromPLN(dep.AmountPLN)
	coinsGiven := dep.CoinsGranted + dep.BonusCoins
	var plan depositVoidPlan
	var walletAfter int64
	err := db.Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id, wallet_grosz, coins_balance").
			First(&user, "id = ?", dep.UserID).Error; err != nil {
			return err
		}
		plan = planDepositVoid(user.WalletGrosz, user.CoinsBalance, amountGrosz, coinsGiven)
		walletAfter = user.WalletGrosz

		res := tx.Model(&models.Deposit{}).
			Where("id = ? AND voided_at IS NULL", dep.ID).
			Updates(map[string]any{"voided_at": now, "voided_by": adminID,
				"refund_grosz": plan.Refund, "coins_taken": plan.CoinsTaken})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errAlreadyVoid
		}
		if plan.Refund > 0 {
			depID := dep.ID
			after, werr := walletApply(tx, walletOp{
				UserID: dep.UserID, Kind: models.WalletTxDepositVoid, Amount: -plan.Refund,
				RefType: "deposit", RefID: &depID, CreatedBy: &adminID,
				Note: fmt.Sprintf("отмена депозита %.2f zł", dep.AmountPLN),
			})
			if werr != nil {
				return werr
			}
			walletAfter = after
		}
		if plan.CoinsTaken > 0 {
			if err := tx.Model(&models.User{}).
				Where("id = ? AND coins_balance >= ?", dep.UserID, plan.CoinsTaken).
				UpdateColumn("coins_balance", gorm.Expr("coins_balance - ?", plan.CoinsTaken)).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, errAlreadyVoid) {
		depositVoidFail(c, http.StatusConflict, "already_void")
		return
	}
	if err != nil {
		depositVoidFail(c, http.StatusInternalServerError, "db_error")
		return
	}

	var nickname string
	db.Model(&models.User{}).Select("nickname").Where("id = ?", dep.UserID).Scan(&nickname)
	refundPLN := models.PLNFromGrosz(plan.Refund)
	unrecoveredPLN := models.PLNFromGrosz(plan.Unrecovered)

	hub.AdminBroadcast("deposit", map[string]any{
		"kind": "void", "nickname": nickname, "amount_pln": dep.AmountPLN,
		"refund_pln": refundPLN, "unrecovered_pln": unrecoveredPLN, "coins": plan.CoinsTaken,
	})
	notifyUser(dep.UserID, "deposit_void", map[string]any{
		"amount_pln": dep.AmountPLN, "refund_pln": refundPLN, "coins": plan.CoinsTaken,
		"wallet_pln": models.PLNFromGrosz(walletAfter),
	})

	c.JSON(http.StatusOK, gin.H{
		"voided":          dep.ID,
		"nickname":        nickname,
		"amount_pln":      dep.AmountPLN,
		"refund_pln":      refundPLN,
		"unrecovered_pln": unrecoveredPLN,
		"coins_taken":     plan.CoinsTaken,
		"wallet_pln":      models.PLNFromGrosz(walletAfter),
	})
}
