package ledger

import "time"

const DefaultWindowDays = 30

var allowedWindowDays = map[int]struct{}{7: {}, 30: {}, 90: {}, 365: {}}

type BudgetTreatment string

const (
	BudgetFixed           BudgetTreatment = "fixed"
	BudgetFlexible        BudgetTreatment = "flexible"
	BudgetFundedIrregular BudgetTreatment = "funded_irregular"
	BudgetExceptional     BudgetTreatment = "exceptional"
)

func ValidWindowDays(days int) bool {
	_, ok := allowedWindowDays[days]
	return ok
}

func InclusiveDays(from, to time.Time) int {
	from = dateOnly(from)
	to = dateOnly(to)
	if to.Before(from) {
		return 0
	}
	return int(to.Sub(from).Hours()/24) + 1
}

func ValidExactRange(from, to time.Time, minimumDays, maximumDays int) bool {
	days := InclusiveDays(from, to)
	return days >= minimumDays && days <= maximumDays
}

func ValidBudgetTreatment(value string) bool {
	switch BudgetTreatment(value) {
	case BudgetFixed, BudgetFlexible, BudgetFundedIrregular, BudgetExceptional:
		return true
	default:
		return false
	}
}

func MonthsInclusive(start, end time.Time) int64 {
	start = dateOnly(start)
	end = dateOnly(end)
	if end.Before(start) {
		return 0
	}
	return int64((end.Year()-start.Year())*12 + int(end.Month()-start.Month()) + 1)
}

func AnnualizeWindow(amountCents int64, coveredDays int) int64 {
	if amountCents <= 0 || coveredDays <= 0 {
		return 0
	}
	return (amountCents*365 + int64(coveredDays)/2) / int64(coveredDays)
}

func FlexibleBalance(monthlyBudgetCents, flexibleSpentCents int64, startedOn, asOf time.Time) int64 {
	return monthlyBudgetCents*MonthsInclusive(startedOn, asOf) - flexibleSpentCents
}

func SinkingFundBalance(openingBalanceCents, monthlyContributionCents, spentCents int64, startedOn, accrualThrough time.Time) int64 {
	return openingBalanceCents + monthlyContributionCents*MonthsInclusive(startedOn, accrualThrough) - spentCents
}

func dateOnly(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
}
