package main

// Денежный кошелёк гостя (трек Г, спринт Г0; GUEST.md, решения Р1/Р2/Р8).
//
// Кошелёк — предоплаченные деньги в грошах: депозит кладёт их сюда, сессия
// будет списывать поминутно (Г1), остаток живёт на аккаунте между визитами —
// «гость может выйти и не потерять деньги» (решение основателя 2026-08-23).
// С монетами не смешивается: монеты — кэшбек (тают, гасятся только временем),
// кошелёк не тает и в монеты не конвертируется.
//
// ЕДИНСТВЕННАЯ дверь к деньгам — walletApply: блокирует строку гостя
// (SELECT … FOR UPDATE), меняет users.wallet_grosz и пишет строку журнала
// wallet_transactions с балансом после операции. Прямые UPDATE wallet_grosz
// в любом другом месте запрещены — так журнал остаётся полным (инвариант:
// сумма amount_grosz гостя == wallet_grosz), а «взлом баланса» с клиента
// невозможен в принципе: сервер не принимает баланс извне.
//
// Р8: в долг не уходим — списание больше остатка отклоняется
// errWalletInsufficient, вызывающий код решает, что делать (предупреждение,
// завершение сессии).

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

var errWalletInsufficient = errors.New("wallet_insufficient")

// walletNewBalance — чистая арифметика применения суммы к балансу (тест):
// отрицательный итог запрещён (Р8 — долгов не существует).
func walletNewBalance(balance, amount int64) (int64, error) {
	next := balance + amount
	if next < 0 {
		return balance, errWalletInsufficient
	}
	return next, nil
}

// walletOp — одна операция кошелька для walletApply.
type walletOp struct {
	UserID    uuid.UUID
	Kind      models.WalletTxKind
	Amount    int64 // гроши, со знаком: + пополнение, − списание
	RefType   string
	RefID     *uuid.UUID
	Note      string
	CreatedBy *uuid.UUID
}

// walletApply — применить операцию внутри переданной транзакции БД.
// Возвращает баланс после операции. При нехватке денег — errWalletInsufficient
// (и транзакцию снаружи следует откатить либо не выполнять списание).
func walletApply(tx *gorm.DB, op walletOp) (int64, error) {
	var user models.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&user, "id = ?", op.UserID).Error; err != nil {
		return 0, err
	}
	next, err := walletNewBalance(user.WalletGrosz, op.Amount)
	if err != nil {
		return user.WalletGrosz, err
	}
	if err := tx.Model(&models.User{}).Where("id = ?", op.UserID).
		Update("wallet_grosz", next).Error; err != nil {
		return 0, err
	}
	rec := models.WalletTransaction{
		UserID:       op.UserID,
		Kind:         op.Kind,
		AmountGrosz:  op.Amount,
		BalanceAfter: next,
		RefID:        op.RefID,
		CreatedBy:    op.CreatedBy,
	}
	if op.RefType != "" {
		rec.RefType = &op.RefType
	}
	if op.Note != "" {
		rec.Note = &op.Note
	}
	if err := tx.Create(&rec).Error; err != nil {
		return 0, err
	}
	return next, nil
}

// GET /me/wallet — кошелёк гостя: баланс и последние операции журнала
// (вкладка «Деньги» в PWA и шапка гостевого экрана, Г0-и3).
func handleGetMyWallet(c *gin.Context) {
	userID := c.GetString("user_id")

	var user models.User
	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "not_found", "message": "Пользователь не найден"})
		return
	}

	var txs []models.WalletTransaction
	db.Where("user_id = ?", userID).
		Order("created_at DESC").Limit(50).Find(&txs)

	c.JSON(http.StatusOK, gin.H{
		"wallet_grosz": user.WalletGrosz,
		"wallet_pln":   models.PLNFromGrosz(user.WalletGrosz),
		"count":        len(txs),
		"transactions": txs,
	})
}
