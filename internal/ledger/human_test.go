package ledger

import "testing"

func TestProjectUsesProductBeforeMerchant(t *testing.T) {
	projection := ProjectHumanTransaction(SourceFact{ProductDescription: "扫地机器人滤网", CounterpartyName: "淘宝", SourceCategory: "购物", Direction: "expense", AmountCents: -9900, Status: "交易成功"})
	if projection.HumanTitle != "扫地机器人滤网" || projection.TransactionType != "expense" || projection.ActualAmountCents != 9900 {
		t.Fatalf("unexpected projection: %#v", projection)
	}
}

func TestProjectKeepsClosedOutOfExpense(t *testing.T) {
	projection := ProjectHumanTransaction(SourceFact{ProductDescription: "已关闭订单", Direction: "expense", AmountCents: -1000, Status: "交易关闭"})
	if projection.TransactionType != "closed" || projection.ActualAmountCents != 0 {
		t.Fatalf("closed transaction was counted: %#v", projection)
	}
}

func TestProjectKeepsRefundIndependent(t *testing.T) {
	projection := ProjectHumanTransaction(SourceFact{CounterpartyName: "商户退款", SourceCategory: "退款", Direction: "income", AmountCents: 1800, Status: "退款成功"})
	if projection.TransactionType != "refund" || projection.RefundedAmountCents != 1800 || projection.ActualAmountCents != 0 {
		t.Fatalf("refund projection mismatch: %#v", projection)
	}
}

func TestProjectKeepsRepaymentOutOfExpense(t *testing.T) {
	projection := ProjectHumanTransaction(SourceFact{ProductDescription: "信用卡还款", Direction: "expense", AmountCents: -50000, Status: "成功"})
	if projection.TransactionType != "internal" || projection.ActualAmountCents != 0 {
		t.Fatalf("repayment was counted as expense: %#v", projection)
	}
}

func TestProjectPendingSeparatelyFromClosed(t *testing.T) {
	projection := ProjectHumanTransaction(SourceFact{ProductDescription: "等待确认收货", Direction: "expense", AmountCents: -1000, Status: "等待确认收货"})
	if projection.TransactionType != "pending" || projection.ActualAmountCents != 0 {
		t.Fatalf("pending transaction mismatch: %#v", projection)
	}
}

func TestProjectDoesNotTreatHuabeiPaymentMethodAsRepayment(t *testing.T) {
	projection := ProjectHumanTransaction(SourceFact{
		ProductDescription: "碰一下交通卡NFC手机交通卡刷卡扣费",
		CounterpartyName:   "北京一卡通",
		SourceCategory:     "交通出行",
		Direction:          "expense",
		AmountCents:        -500,
		PaymentMethod:      "花呗",
		Status:             "交易成功",
	})
	if projection.TransactionType != "expense" || projection.ActualAmountCents != 500 {
		t.Fatalf("ordinary purchase paid by Huabei must remain an expense: %#v", projection)
	}
}

func TestProjectTreatsMobileTopUpAsExpense(t *testing.T) {
	projection := ProjectHumanTransaction(SourceFact{
		ProductDescription: "话费自动充值",
		CounterpartyName:   "中国移动",
		SourceCategory:     "充值缴费",
		Direction:          "expense",
		AmountCents:        -5000,
		Status:             "交易成功",
	})
	if projection.TransactionType != "expense" || projection.ActualAmountCents != 5000 {
		t.Fatalf("mobile top-up is consumption, not an internal transfer: %#v", projection)
	}
}

func TestProjectTreatsMerchantBatchCompensationAsRefund(t *testing.T) {
	projection := ProjectHumanTransaction(SourceFact{
		ProductDescription: "批量付款-订单号/补差价/天猫石头生活电器旗舰店",
		CounterpartyName:   "北京石头世纪科技股份有限公司",
		SourceCategory:     "转账红包",
		Direction:          "income",
		AmountCents:        57135,
		Status:             "交易成功",
	})
	if projection.TransactionType != "refund" || projection.RefundedAmountCents != 57135 || projection.ActualAmountCents != 0 {
		t.Fatalf("merchant compensation must be represented as a refund: %#v", projection)
	}
}

func TestProjectKeepsZeroAmountSuccessDistinctFromCancellation(t *testing.T) {
	projection := ProjectHumanTransaction(SourceFact{
		ProductDescription: "皇家猫粮",
		Direction:          "expense",
		AmountCents:        0,
		Status:             "支付成功",
	})
	if projection.TransactionType != "zero" || projection.OriginalAmountCents != 0 || projection.ActualAmountCents != 0 {
		t.Fatalf("zero amount success must remain explicit rather than being invented as a closed transaction: %#v", projection)
	}
}
