package mysql

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/wqedggc/shoreos/internal/config"
)

// This opt-in test reads aggregate values only. It never prints or changes
// private transaction facts.
func TestLocalLedgerPeriodAnalysisAcceptance(t *testing.T) {
	if os.Getenv("SHOREOS_RUN_LOCAL_LEDGER_INTEGRATION") != "1" {
		t.Skip("set SHOREOS_RUN_LOCAL_LEDGER_INTEGRATION=1 to verify the private local ledger dataset")
	}
	store, err := Open(config.Load())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	var userID int64
	var latestRaw string
	if err := store.db.QueryRowContext(ctx, `
SELECT user_id, MAX(LEFT(occurred_at, 10))
FROM ledger_v2_human_transactions
GROUP BY user_id ORDER BY COUNT(*) DESC LIMIT 1
`).Scan(&userID, &latestRaw); err != nil {
		t.Fatal(err)
	}
	to, err := time.ParseInLocation(dateLayout, latestRaw, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	from := to.AddDate(0, 0, -29)
	analysis, err := store.PeriodAnalysis(ctx, userID, from, to, "day")
	if err != nil {
		t.Fatal(err)
	}
	var timelineExpense int64
	for _, item := range analysis.Timeline {
		timelineExpense += item.ActualExpenseCents
	}
	if timelineExpense != analysis.Totals.ActualExpenseCents {
		t.Fatalf("timeline actual expense = %d, totals = %d", timelineExpense, analysis.Totals.ActualExpenseCents)
	}
	var categoryExpense int64
	for _, item := range analysis.Breakdowns.CategoryGroup {
		categoryExpense += item.AmountCents
	}
	if categoryExpense != analysis.Totals.ActualExpenseCents {
		t.Fatalf("category actual expense = %d, totals = %d", categoryExpense, analysis.Totals.ActualExpenseCents)
	}
}
