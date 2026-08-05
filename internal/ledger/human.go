package ledger

import "strings"

type SourceFact struct {
	SourceType         string
	Platform           string
	OccurredAt         string
	SourceCategory     string
	CounterpartyName   string
	ProductDescription string
	Direction          string
	AmountCents        int64
	PaymentMethod      string
	Status             string
}

type HumanProjection struct {
	EntryKind           string
	TransactionType     string
	HumanTitle          string
	MerchantName        string
	OriginalAmountCents int64
	RefundedAmountCents int64
	ActualAmountCents   int64
	NeedsReview         bool
}

func ProjectHumanTransaction(fact SourceFact) HumanProjection {
	title := firstMeaningful(fact.ProductDescription, fact.CounterpartyName, fact.SourceCategory, "待解释交易")
	kind := "expense"
	transactionType := "expense"
	actual := abs(fact.AmountCents)
	refunded := int64(0)

	if containsAny(fact.Status, "关闭", "失败") {
		kind, transactionType, actual = "closed", "closed", 0
	} else if containsAny(fact.Status, "等待", "待收货", "未完成", "处理中") {
		kind, transactionType, actual = "pending", "pending", 0
	} else if isRefund(fact) {
		kind, transactionType, refunded, actual = "refund", "refund", abs(fact.AmountCents), 0
	} else if fact.Direction == "expense" && fact.AmountCents == 0 {
		kind, transactionType, actual = "zero", "zero", 0
	} else if isInternalMovement(fact) {
		kind, transactionType, actual = "internal", "internal", 0
	} else if fact.Direction == "income" {
		kind, transactionType, actual = "income", "income", 0
	} else if fact.Direction == "neutral" {
		kind, transactionType, actual = "internal", "internal", 0
	}

	return HumanProjection{
		EntryKind:           kind,
		TransactionType:     transactionType,
		HumanTitle:          title,
		MerchantName:        strings.TrimSpace(fact.CounterpartyName),
		OriginalAmountCents: abs(fact.AmountCents),
		RefundedAmountCents: refunded,
		ActualAmountCents:   actual,
		NeedsReview:         title == "待解释交易",
	}
}

func isRefund(fact SourceFact) bool {
	if containsAny(strings.Join([]string{fact.Status, fact.SourceCategory, fact.CounterpartyName, fact.ProductDescription}, " "), "退款") {
		return true
	}
	return fact.Direction == "income" && containsAny(fact.ProductDescription, "批量付款") && containsAny(fact.ProductDescription, "补差价", "价保")
}

func isInternalMovement(fact SourceFact) bool {
	if containsAny(fact.SourceCategory, "转账红包", "信用借还") {
		return true
	}
	return containsAny(strings.Join([]string{fact.SourceCategory, fact.CounterpartyName, fact.ProductDescription}, " "),
		"还款", "余额宝", "余利宝", "自动转入", "转入到", "转出到", "提现")
}

func firstMeaningful(values ...string) string {
	for _, value := range values {
		if normalized := strings.Join(strings.Fields(value), " "); normalized != "" {
			return normalized
		}
	}
	return ""
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func abs(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}
