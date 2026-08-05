package ledger

import (
	"testing"
	"time"
)

func TestMonthsInclusive(t *testing.T) {
	start := time.Date(2026, time.January, 20, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.Local)
	if got := MonthsInclusive(start, end); got != 12 {
		t.Fatalf("MonthsInclusive() = %d, want 12", got)
	}
}

func TestFlexibleBalanceOnlyAccumulatesReminder(t *testing.T) {
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.Local)
	asOf := time.Date(2026, time.March, 31, 0, 0, 0, 0, time.Local)
	if got := FlexibleBalance(100_000, 350_000, start, asOf); got != -50_000 {
		t.Fatalf("FlexibleBalance() = %d, want -50000", got)
	}
}

func TestSinkingFundBalanceCanBeNegative(t *testing.T) {
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.Local)
	asOf := time.Date(2026, time.March, 31, 0, 0, 0, 0, time.Local)
	if got := SinkingFundBalance(10_000, 20_000, 80_000, start, asOf); got != -10_000 {
		t.Fatalf("SinkingFundBalance() = %d, want -10000", got)
	}
}

func TestAnnualizeWindow(t *testing.T) {
	if got := AnnualizeWindow(300_000, 30); got != 3_650_000 {
		t.Fatalf("AnnualizeWindow() = %d, want 3650000", got)
	}
}

func TestInclusiveDaysAcrossLeapDay(t *testing.T) {
	from := time.Date(2024, time.February, 28, 0, 0, 0, 0, time.Local)
	to := time.Date(2024, time.March, 1, 0, 0, 0, 0, time.Local)
	if got := InclusiveDays(from, to); got != 3 {
		t.Fatalf("InclusiveDays() = %d, want 3", got)
	}
}

func TestValidExactRange(t *testing.T) {
	from := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.Local)
	if !ValidExactRange(from, from.AddDate(0, 0, 365), 7, 366) {
		t.Fatal("366-day range should be valid")
	}
	if ValidExactRange(from, from.AddDate(0, 0, 366), 7, 366) {
		t.Fatal("367-day range should be rejected")
	}
	if ValidExactRange(from, from.AddDate(0, 0, 5), 7, 366) {
		t.Fatal("6-day prediction range should be rejected")
	}
}
