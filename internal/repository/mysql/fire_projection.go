package mysql

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type FireScenario struct {
	ID         int64
	ProfileUID string
	Profile    map[string]any
}

type FireProjectionRun struct {
	ID            int64          `json:"id"`
	ScenarioUID   string         `json:"scenarioId"`
	RunMode       string         `json:"runMode"`
	InputSnapshot map[string]any `json:"inputSnapshot"`
	CreatedAt     time.Time      `json:"createdAt"`
}

type SpendingImpactCategory struct {
	CategoryGroup string `json:"categoryGroup"`
	AmountCents   int64  `json:"amountCents"`
	Count         int    `json:"count"`
}

type SpendingImpactTransaction struct {
	HumanTransactionID int64  `json:"humanTransactionId"`
	HumanTitle         string `json:"humanTitle"`
	CategoryGroup      string `json:"categoryGroup"`
	BudgetTreatment    string `json:"budgetTreatment"`
	AmountCents        int64  `json:"amountCents"`
}

func (s *Store) FireScenarioByUID(ctx context.Context, userID int64, profileUID string) (FireScenario, error) {
	var scenario FireScenario
	var raw string
	err := s.db.QueryRowContext(ctx, `
SELECT id, profile_uid, scenario_json
FROM fire_scenarios WHERE user_id = ? AND profile_uid = ?
`, userID, profileUID).Scan(&scenario.ID, &scenario.ProfileUID, &raw)
	if err != nil {
		return FireScenario{}, err
	}
	if err := json.Unmarshal([]byte(raw), &scenario.Profile); err != nil {
		return FireScenario{}, fmt.Errorf("decode FIRE scenario: %w", err)
	}
	return scenario, nil
}

func (s *Store) CreateFireProjectionRun(ctx context.Context, userID, scenarioID int64, runMode string, snapshot map[string]any) (FireProjectionRun, error) {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return FireProjectionRun{}, fmt.Errorf("encode FIRE projection input: %w", err)
	}
	result, err := s.db.ExecContext(ctx, `
INSERT INTO fire_projection_runs (user_id, scenario_id, run_mode, input_snapshot, created_at)
VALUES (?, ?, ?, ?, NOW(3))
`, userID, scenarioID, runMode, string(raw))
	if err != nil {
		return FireProjectionRun{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return FireProjectionRun{}, err
	}
	return s.FireProjectionRun(ctx, userID, id)
}

func (s *Store) FireProjectionRun(ctx context.Context, userID, runID int64) (FireProjectionRun, error) {
	var run FireProjectionRun
	var raw string
	err := s.db.QueryRowContext(ctx, `
SELECT r.id, COALESCE(s.profile_uid, ''), r.run_mode, r.input_snapshot, r.created_at
FROM fire_projection_runs r
LEFT JOIN fire_scenarios s ON s.id = r.scenario_id AND s.user_id = r.user_id
WHERE r.id = ? AND r.user_id = ?
`, runID, userID).Scan(&run.ID, &run.ScenarioUID, &run.RunMode, &raw, &run.CreatedAt)
	if err != nil {
		return FireProjectionRun{}, err
	}
	if err := json.Unmarshal([]byte(raw), &run.InputSnapshot); err != nil {
		return FireProjectionRun{}, fmt.Errorf("decode FIRE projection input: %w", err)
	}
	return run, nil
}

func (s *Store) SpendingImpactCategories(ctx context.Context, userID int64, from, to time.Time, limit int) ([]SpendingImpactCategory, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT COALESCE(NULLIF(o.category_group, ''), '待解释'), SUM(h.actual_amount_cents), COUNT(*)
FROM ledger_v2_human_transactions h
LEFT JOIN ledger_v2_transaction_overrides o ON o.human_transaction_id = h.id AND o.user_id = h.user_id
WHERE h.user_id = ? AND h.transaction_type = 'expense'
  AND COALESCE(NULLIF(o.budget_treatment, ''), 'flexible') = 'flexible'
  AND LEFT(h.occurred_at, 10) BETWEEN ? AND ?
GROUP BY COALESCE(NULLIF(o.category_group, ''), '待解释')
ORDER BY SUM(h.actual_amount_cents) DESC
LIMIT ?
`, userID, from.Format(dateLayout), to.Format(dateLayout), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]SpendingImpactCategory, 0)
	for rows.Next() {
		var item SpendingImpactCategory
		if err := rows.Scan(&item.CategoryGroup, &item.AmountCents, &item.Count); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) SpendingImpactTransactions(ctx context.Context, userID int64, from, to time.Time, limit int) ([]SpendingImpactTransaction, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT h.id, COALESCE(NULLIF(o.human_title, ''), h.human_title),
       COALESCE(NULLIF(o.category_group, ''), '待解释'),
       COALESCE(NULLIF(o.budget_treatment, ''), 'flexible'), h.actual_amount_cents
FROM ledger_v2_human_transactions h
LEFT JOIN ledger_v2_transaction_overrides o ON o.human_transaction_id = h.id AND o.user_id = h.user_id
WHERE h.user_id = ? AND h.transaction_type = 'expense'
  AND COALESCE(NULLIF(o.budget_treatment, ''), 'flexible') IN ('flexible', 'exceptional')
  AND LEFT(h.occurred_at, 10) BETWEEN ? AND ?
ORDER BY h.actual_amount_cents DESC, h.id DESC
LIMIT ?
`, userID, from.Format(dateLayout), to.Format(dateLayout), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]SpendingImpactTransaction, 0)
	for rows.Next() {
		var item SpendingImpactTransaction
		if err := rows.Scan(&item.HumanTransactionID, &item.HumanTitle, &item.CategoryGroup, &item.BudgetTreatment, &item.AmountCents); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
