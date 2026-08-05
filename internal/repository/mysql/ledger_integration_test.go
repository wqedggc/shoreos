package mysql

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/wqedggc/shoreos/internal/config"
)

// This test is intentionally opt-in: it validates the local, private acceptance
// dataset without printing transaction facts or requiring it in normal CI.
func TestLocalLedgerMaterializationAcceptance(t *testing.T) {
	if os.Getenv("SHOREOS_RUN_LOCAL_LEDGER_INTEGRATION") != "1" {
		t.Skip("set SHOREOS_RUN_LOCAL_LEDGER_INTEGRATION=1 to verify the private local ledger dataset")
	}

	cfg := config.Load()
	cfg.AdminPassword = "local-acceptance-only-password"
	store, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	if _, _, err := store.Bootstrap(ctx); !errors.Is(err, ErrAlreadyInitialized) {
		t.Fatalf("bootstrap must not issue a session for an initialized ShoreOS: %v", err)
	}

	var userID int64
	err = store.db.QueryRowContext(ctx, `
SELECT user_id
FROM ledger_v2_source_transactions
GROUP BY user_id
ORDER BY COUNT(*) DESC
LIMIT 1
`).Scan(&userID)
	if err != nil {
		t.Fatalf("find local acceptance user: %v", err)
	}

	var expectedHuman, expectedBank, expectedCandidates int
	if err := store.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM ledger_v2_source_transactions
WHERE user_id = ? AND source_type IN ('alipay_csv', 'wechat_xlsx')
`, userID).Scan(&expectedHuman); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM ledger_v2_source_transactions
WHERE user_id = ? AND source_type = 'cmb_bank_pdf_text'
`, userID).Scan(&expectedBank); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM ledger_v2_source_transactions bank_source
JOIN ledger_v2_source_transactions payment_source
  ON payment_source.user_id = bank_source.user_id
 AND payment_source.source_type IN ('alipay_csv', 'wechat_xlsx')
 AND LEFT(payment_source.occurred_at, 10) = LEFT(bank_source.occurred_at, 10)
 AND ABS(payment_source.amount_cents) = ABS(bank_source.amount_cents)
 AND payment_source.direction = bank_source.direction
WHERE bank_source.user_id = ? AND bank_source.source_type = 'cmb_bank_pdf_text'
`, userID).Scan(&expectedCandidates); err != nil {
		t.Fatal(err)
	}

	result, err := store.MaterializeLedger(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if result.HumanTransactionCount != expectedHuman || result.BankEvidenceCount != expectedBank || result.CandidateLinkCount != expectedCandidates {
		t.Fatalf("unexpected materialization summary: %+v", result)
	}

}
