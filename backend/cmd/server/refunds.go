package main

// З11-и1 (OWNER.md, трек З; №17; решения основателя 30.09): способ возврата у
// отмен. Отмена депозита, продажи и чека, выданного пакета и оплаченного заказа
// кухни принимает «как вернули» (refund_method): наличными, на карту, по BLIK,
// на кошелёк или «не возвращали»; не сказали — как платили. Способ пишется в
// строку (deposits, sales, user_packages, миграция 079), и ящик уменьшают
// только наличные возвраты (drawer.go).
//
// Админ возвращает тем же способом, каким платили, или не возвращает; другой
// способ выбирает владелец (решение основателя 30.09): иначе оплаченное картой
// можно «вернуть» наличными из ящика. На кошелёк возвращается только то, что
// платили кошельком (решение основателя 30.09). Неоплаченный долг вернуть
// нельзя — только «не возвращали».

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const refundNone = "none" // «не возвращали»

// refundMethods — как можно вернуть деньги отмены.
var refundMethods = map[string]bool{"cash": true, "card": true, "blik": true, "wallet": true, refundNone: true}

var refundErrors = map[string]string{
	"bad_refund_method":  "Как вернули: наличными, на карту, по BLIK, на кошелёк или «не возвращали»",
	"refund_unpaid":      "Денег за это не брали — вернуть нечего: только «не возвращали»",
	"refund_wallet_only": "На кошелёк возвращается только то, что платили кошельком",
	"refund_owner_only":  "Вернуть не тем способом, каким платили, может только владелец — админ возвращает тем же способом или не возвращает",
}

// refundFail — отказ по способу возврата: чужой способ у админа — 403, остальное — 400.
func refundFail(c *gin.Context, code string) {
	status := http.StatusBadRequest
	if code == "refund_owner_only" {
		status = http.StatusForbidden
	}
	c.JSON(status, gin.H{"code": code, "message": refundErrors[code]})
}

// pickRefund — как вернули деньги отмены (чистая, тест). paid — как платили
// ("" — не платили: долг, неоплаченный заказ; З11-и5: comp — подарок, денег тоже
// не брали), asked — что выбрали ("" — как платили), owner — отменяет владелец.
// Второе значение — код отказа.
func pickRefund(paid, asked string, owner bool) (string, string) {
	if asked == "" {
		if !refundMethods[paid] {
			return refundNone, "" // долг или способ из старых данных: из ящика не уходит
		}
		return paid, ""
	}
	switch {
	case !refundMethods[asked]:
		return "", "bad_refund_method"
	case asked == refundNone:
		return refundNone, ""
	case !refundMethods[paid]: // долг, подарок, способ из старых данных — денег не брали
		return "", "refund_unpaid"
	case asked == "wallet" && paid != "wallet":
		return "", "refund_wallet_only"
	case asked != paid && !owner:
		return "", "refund_owner_only"
	}
	return asked, ""
}

// refundAsked — «как вернули» из тела отмены: тела нет — "" (как платили);
// кривой JSON — 400 и false.
func refundAsked(c *gin.Context) (string, bool) {
	var req struct {
		RefundMethod string `json:"refund_method"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Тело отмены — JSON: {\"refund_method\": …}"})
		return "", false
	}
	return strings.TrimSpace(req.RefundMethod), true
}

// refundByOwner — отменяет владелец (ему доступен любой способ возврата).
func refundByOwner(c *gin.Context) bool {
	return c.GetString("user_role") == string(models.UserRoleOwner)
}

// salePaid — как платили продажу: неоплаченный долг — "".
func salePaid(s *models.Sale) string {
	if s.PaidAt == nil {
		return ""
	}
	return s.Method
}

// refundRu — как вернули, словами для журнала (чистая, тест).
func refundRu(method string) string {
	switch method {
	case "cash":
		return "вернули наличными"
	case "card":
		return "вернули на карту"
	case "blik":
		return "вернули по BLIK"
	case "wallet":
		return "вернули на кошелёк"
	case refundNone:
		return "денег не возвращали"
	}
	return method
}
