package mysql

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/wqedggc/shoreos/internal/config"
)

func TestLocalClassificationRuleAcceptance(t *testing.T) {
	if os.Getenv("SHOREOS_RUN_LOCAL_LEDGER_INTEGRATION") != "1" {
		t.Skip("set SHOREOS_RUN_LOCAL_LEDGER_INTEGRATION=1 to verify classification rules against the local ledger")
	}

	store, err := Open(config.Load())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	var userID, humanID int64
	if err := store.db.QueryRowContext(ctx, `
SELECT user_id, id
FROM ledger_v2_human_transactions
WHERE transaction_type = 'expense'
ORDER BY id DESC
LIMIT 1
`).Scan(&userID, &humanID); err != nil {
		t.Fatal(err)
	}

	completeRule := ClassificationRule{
		MatchField:      "exact_merchant",
		MatchValue:      "__shoreos_rule_acceptance_" + time.Now().Format("20060102150405.000000000"),
		CategoryGroup:   "交通出行",
		CategoryDetail:  "公交地铁",
		Purpose:         "生存与责任",
		Necessity:       "必需",
		Planning:        "计划内",
		Recurrence:      "不规律重复",
		BudgetTreatment: "flexible",
	}

	created, _, wasCreated, err := store.CreateClassificationRule(ctx, userID, completeRule)
	if err != nil {
		t.Fatalf("create classification rule: %v", err)
	}
	if !wasCreated {
		t.Fatal("first save must create a rule")
	}
	defer store.db.ExecContext(ctx, `DELETE FROM ledger_v2_classification_rules WHERE id = ? AND user_id = ?`, created.ID, userID)

	completeRule.CategoryDetail = "打车"
	updated, _, wasCreated, err := store.CreateClassificationRule(ctx, userID, completeRule)
	if err != nil {
		t.Fatalf("saving an existing match condition must update the rule: %v", err)
	}
	if updated.ID != created.ID || updated.CategoryDetail != "打车" {
		t.Fatalf("expected rule %d to be updated, got %+v", created.ID, updated)
	}
	if wasCreated {
		t.Fatal("second save of the same match condition must update, not create")
	}

	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
INSERT INTO ledger_v2_classification_rules (
  user_id, match_field, match_value, category_group, category_detail, purpose, necessity,
  planning, recurrence, budget_treatment, enabled, hit_count, created_at, updated_at
) VALUES (?, 'exact_merchant', ?, ?, ?, ?, ?, ?, ?, ?, 1, 0, NOW(3), NOW(3))
`, userID, completeRule.MatchValue+"_application", completeRule.CategoryGroup, completeRule.CategoryDetail,
		completeRule.Purpose, completeRule.Necessity, completeRule.Planning, completeRule.Recurrence, completeRule.BudgetTreatment)
	if err != nil {
		t.Fatal(err)
	}
	completeRule.ID, err = result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := applyRule(ctx, tx, userID, completeRule, []RuleMatch{{HumanTransactionID: humanID}}); err != nil {
		t.Fatal(err)
	}
	var confidence, source string
	var needsReview bool
	if err := tx.QueryRowContext(ctx, `
SELECT o.confidence, o.interpretation_source, h.needs_review
FROM ledger_v2_human_transactions h
JOIN ledger_v2_transaction_overrides o ON o.human_transaction_id = h.id AND o.user_id = h.user_id
WHERE h.id = ? AND h.user_id = ?
`, humanID, userID).Scan(&confidence, &source, &needsReview); err != nil {
		t.Fatal(err)
	}
	if confidence != "confirmed" || source != "user_rule" || needsReview {
		t.Fatalf("complete user rule must be confirmed and clear review: confidence=%s source=%s needsReview=%v", confidence, source, needsReview)
	}
}
