package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/wqedggc/shoreos/internal/ledger"
)

const dateLayout = "2006-01-02"

type FixedExpense struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	CategoryGroup      string `json:"categoryGroup"`
	CategoryDetail     string `json:"categoryDetail"`
	MonthlyAmountCents int64  `json:"monthlyAmountCents"`
	Status             string `json:"status"`
}

type SinkingFund struct {
	ID                       int64  `json:"id"`
	Name                     string `json:"name"`
	MonthlyContributionCents int64  `json:"monthlyContributionCents"`
	OpeningBalanceCents      int64  `json:"openingBalanceCents"`
	CurrentBalanceCents      int64  `json:"currentBalanceCents"`
	StartedOn                string `json:"startedOn"`
	AccrualEndedOn           string `json:"accrualEndedOn,omitempty"`
	Status                   string `json:"status"`
}

type BudgetPlan struct {
	Configured                    bool           `json:"configured"`
	FlexibleBudgetMonthlyCents    int64          `json:"flexibleBudgetMonthlyCents"`
	FlexibleSuggestion30DaysCents int64          `json:"flexibleSuggestion30DaysCents"`
	FlexibleSuggestion90DaysCents int64          `json:"flexibleSuggestion90DaysCents"`
	StartedOn                     string         `json:"startedOn"`
	FixedExpenses                 []FixedExpense `json:"fixedExpenses"`
	SinkingFunds                  []SinkingFund  `json:"sinkingFunds"`
}

type SaveBudgetPlanInput struct {
	FlexibleBudgetMonthlyCents int64
	StartedOn                  time.Time
	FixedExpenses              []FixedExpense
}

type SaveSinkingFundInput struct {
	Name                     string
	MonthlyContributionCents int64
	OpeningBalanceCents      int64
	StartedOn                time.Time
	Status                   string
}

type RollingSpendingBaseline struct {
	AsOf                               string `json:"asOf"`
	WindowDays                         int    `json:"windowDays"`
	CoveredFrom                        string `json:"coveredFrom"`
	CoveredTo                          string `json:"coveredTo"`
	Currency                           string `json:"currency"`
	HasImportedData                    bool   `json:"hasImportedData"`
	ActualExpenseCents                 int64  `json:"actualExpenseCents"`
	FixedAnnualCents                   int64  `json:"fixedAnnualCents"`
	FlexibleActualCents                int64  `json:"flexibleActualCents"`
	FlexibleBudgetMonthlyCents         int64  `json:"flexibleBudgetMonthlyCents"`
	FlexibleBalanceCents               int64  `json:"flexibleBalanceCents"`
	SinkingFundAnnualContributionCents int64  `json:"sinkingFundAnnualContributionCents"`
	SinkingFundBalanceCents            int64  `json:"sinkingFundBalanceCents"`
	ExceptionalActualCents             int64  `json:"exceptionalActualCents"`
	TargetAnnualExpenseCents           int64  `json:"targetAnnualExpenseCents"`
	ActualPaceAnnualExpenseCents       int64  `json:"actualPaceAnnualExpenseCents"`
	PendingAmountCents                 int64  `json:"pendingAmountCents"`
	RefundAmountCents                  int64  `json:"refundAmountCents"`
	InternalTransferAmountCents        int64  `json:"internalTransferAmountCents"`
	NeedsReviewExpenseCents            int64  `json:"needsReviewExpenseCents"`
	DataCoverage                       string `json:"dataCoverage"`
}

type spendingSums struct {
	ActualExpense int64
	Flexible      int64
	Exceptional   int64
	Pending       int64
	Refund        int64
	Internal      int64
	NeedsReview   int64
}

func (s *Store) SaveBudgetPlan(ctx context.Context, userID int64, input SaveBudgetPlanInput) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
INSERT INTO ledger_v2_budget_settings (
  user_id, flexible_budget_monthly_cents, started_on, created_at, updated_at
) VALUES (?, ?, ?, NOW(3), NOW(3))
ON DUPLICATE KEY UPDATE flexible_budget_monthly_cents = VALUES(flexible_budget_monthly_cents),
  started_on = VALUES(started_on), updated_at = NOW(3)
`, userID, input.FlexibleBudgetMonthlyCents, input.StartedOn.Format(dateLayout)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM ledger_v2_fixed_expenses WHERE user_id = ?`, userID); err != nil {
		return err
	}
	for _, item := range input.FixedExpenses {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO ledger_v2_fixed_expenses (
  user_id, name, category_group, category_detail, monthly_amount_cents, status, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, 'active', NOW(3), NOW(3))
`, userID, item.Name, item.CategoryGroup, item.CategoryDetail, item.MonthlyAmountCents); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) CreateSinkingFund(ctx context.Context, userID int64, input SaveSinkingFundInput) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
INSERT INTO ledger_v2_sinking_funds (
  user_id, name, monthly_contribution_cents, opening_balance_cents, started_on,
  status, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, 'active', NOW(3), NOW(3))
`, userID, input.Name, input.MonthlyContributionCents, input.OpeningBalanceCents, input.StartedOn.Format(dateLayout))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) UpdateSinkingFund(ctx context.Context, userID, fundID int64, input SaveSinkingFundInput, asOf time.Time) error {
	var currentStatus string
	if err := s.db.QueryRowContext(ctx, `
SELECT status FROM ledger_v2_sinking_funds WHERE id = ? AND user_id = ?
`, fundID, userID).Scan(&currentStatus); err != nil {
		return err
	}
	if currentStatus != "active" && input.Status == "active" {
		return errors.New("paused or closed sinking fund cannot be reactivated")
	}
	var endedOn any
	if input.Status != "active" {
		endedOn = asOf.Format(dateLayout)
	}
	result, err := s.db.ExecContext(ctx, `
UPDATE ledger_v2_sinking_funds
SET name = ?, monthly_contribution_cents = ?, opening_balance_cents = ?, started_on = ?,
    accrual_ended_on = COALESCE(accrual_ended_on, ?), status = ?, updated_at = NOW(3)
WHERE id = ? AND user_id = ?
`, input.Name, input.MonthlyContributionCents, input.OpeningBalanceCents, input.StartedOn.Format(dateLayout), endedOn, input.Status, fundID, userID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) BudgetPlan(ctx context.Context, userID int64, asOf time.Time) (BudgetPlan, error) {
	plan := BudgetPlan{FixedExpenses: []FixedExpense{}, SinkingFunds: []SinkingFund{}}
	var startedOn time.Time
	err := s.db.QueryRowContext(ctx, `
SELECT flexible_budget_monthly_cents, started_on
FROM ledger_v2_budget_settings WHERE user_id = ?
`, userID).Scan(&plan.FlexibleBudgetMonthlyCents, &startedOn)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return BudgetPlan{}, err
	}
	if err == nil {
		plan.Configured = true
		plan.StartedOn = startedOn.Format(dateLayout)
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, COALESCE(category_group, ''), COALESCE(category_detail, ''), monthly_amount_cents, status
FROM ledger_v2_fixed_expenses WHERE user_id = ? ORDER BY id
`, userID)
	if err != nil {
		return BudgetPlan{}, err
	}
	for rows.Next() {
		var item FixedExpense
		if err := rows.Scan(&item.ID, &item.Name, &item.CategoryGroup, &item.CategoryDetail, &item.MonthlyAmountCents, &item.Status); err != nil {
			rows.Close()
			return BudgetPlan{}, err
		}
		plan.FixedExpenses = append(plan.FixedExpenses, item)
	}
	if err := rows.Close(); err != nil {
		return BudgetPlan{}, err
	}

	funds, err := s.sinkingFunds(ctx, userID, asOf)
	if err != nil {
		return BudgetPlan{}, err
	}
	plan.SinkingFunds = funds
	plan.FlexibleSuggestion30DaysCents, err = s.flexibleMonthlySuggestion(ctx, userID, asOf, 30)
	if err != nil {
		return BudgetPlan{}, err
	}
	plan.FlexibleSuggestion90DaysCents, err = s.flexibleMonthlySuggestion(ctx, userID, asOf, 90)
	if err != nil {
		return BudgetPlan{}, err
	}
	return plan, nil
}

func (s *Store) SpendingBaseline(ctx context.Context, userID int64, asOf time.Time, windowDays int) (RollingSpendingBaseline, error) {
	from := asOf.AddDate(0, 0, -(windowDays - 1))
	return s.spendingBaselineRange(ctx, userID, from, asOf)
}

func (s *Store) SpendingBaselineRange(ctx context.Context, userID int64, from, to time.Time) (RollingSpendingBaseline, error) {
	return s.spendingBaselineRange(ctx, userID, from, to)
}

func (s *Store) spendingBaselineRange(ctx context.Context, userID int64, from, asOf time.Time) (RollingSpendingBaseline, error) {
	plan, err := s.BudgetPlan(ctx, userID, asOf)
	if err != nil {
		return RollingSpendingBaseline{}, err
	}
	sums, err := s.spendingSums(ctx, userID, from, asOf)
	if err != nil {
		return RollingSpendingBaseline{}, err
	}

	baseline := RollingSpendingBaseline{
		AsOf: asOf.Format(dateLayout), WindowDays: ledger.InclusiveDays(from, asOf), Currency: "CNY",
		ActualExpenseCents: sums.ActualExpense, FlexibleActualCents: sums.Flexible,
		FlexibleBudgetMonthlyCents: plan.FlexibleBudgetMonthlyCents,
		ExceptionalActualCents:     sums.Exceptional, PendingAmountCents: sums.Pending,
		RefundAmountCents: sums.Refund, InternalTransferAmountCents: sums.Internal,
		NeedsReviewExpenseCents: sums.NeedsReview,
	}
	for _, item := range plan.FixedExpenses {
		if item.Status == "active" {
			baseline.FixedAnnualCents += item.MonthlyAmountCents * 12
		}
	}
	for _, fund := range plan.SinkingFunds {
		baseline.SinkingFundBalanceCents += fund.CurrentBalanceCents
		if fund.Status == "active" {
			baseline.SinkingFundAnnualContributionCents += fund.MonthlyContributionCents * 12
		}
	}
	if plan.Configured {
		startedOn, _ := time.ParseInLocation(dateLayout, plan.StartedOn, asOf.Location())
		allFlexible, err := s.spendingSums(ctx, userID, startedOn, asOf)
		if err != nil {
			return RollingSpendingBaseline{}, err
		}
		baseline.FlexibleBalanceCents = ledger.FlexibleBalance(plan.FlexibleBudgetMonthlyCents, allFlexible.Flexible, startedOn, asOf)
	}

	coveredFrom, coveredTo, hasData, err := s.importCoverage(ctx, userID, from, asOf)
	if err != nil {
		return RollingSpendingBaseline{}, err
	}
	baseline.HasImportedData = hasData
	baseline.CoveredFrom = coveredFrom.Format(dateLayout)
	baseline.CoveredTo = coveredTo.Format(dateLayout)
	coveredDays := int(coveredTo.Sub(coveredFrom).Hours()/24) + 1
	if !hasData || coveredDays <= 0 {
		coveredDays = 0
		baseline.CoveredFrom, baseline.CoveredTo = "", ""
		baseline.DataCoverage = "insufficient"
	} else if !plan.Configured {
		baseline.DataCoverage = "insufficient"
	} else if coveredFrom.After(from) || coveredTo.Before(asOf) || sums.NeedsReview > 0 {
		baseline.DataCoverage = "partial"
	} else {
		baseline.DataCoverage = "complete"
	}
	baseline.TargetAnnualExpenseCents = baseline.FixedAnnualCents + plan.FlexibleBudgetMonthlyCents*12 + baseline.SinkingFundAnnualContributionCents
	baseline.ActualPaceAnnualExpenseCents = baseline.FixedAnnualCents + ledger.AnnualizeWindow(sums.Flexible, coveredDays) + baseline.SinkingFundAnnualContributionCents
	return baseline, nil
}

func (s *Store) spendingSums(ctx context.Context, userID int64, from, to time.Time) (spendingSums, error) {
	var sums spendingSums
	err := s.db.QueryRowContext(ctx, fmt.Sprintf(`
SELECT
  COALESCE(SUM(CASE WHEN h.transaction_type = 'expense' THEN h.actual_amount_cents ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN h.transaction_type = 'expense' AND COALESCE(NULLIF(o.budget_treatment, ''), 'flexible') = 'flexible' THEN h.actual_amount_cents ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN h.transaction_type = 'expense' AND o.budget_treatment = 'exceptional' THEN h.actual_amount_cents ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN h.transaction_type = 'pending' THEN h.original_amount_cents ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN h.transaction_type = 'refund' THEN h.refunded_amount_cents ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN h.transaction_type = 'internal' THEN h.original_amount_cents ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN %s THEN h.actual_amount_cents ELSE 0 END), 0)
FROM ledger_v2_human_transactions h
LEFT JOIN ledger_v2_transaction_overrides o ON o.human_transaction_id = h.id AND o.user_id = h.user_id
WHERE h.user_id = ? AND LEFT(h.occurred_at, 10) BETWEEN ? AND ?
`, transactionNeedsReviewSQL), userID, from.Format(dateLayout), to.Format(dateLayout)).Scan(&sums.ActualExpense, &sums.Flexible, &sums.Exceptional,
		&sums.Pending, &sums.Refund, &sums.Internal, &sums.NeedsReview)
	return sums, err
}

func (s *Store) flexibleMonthlySuggestion(ctx context.Context, userID int64, asOf time.Time, days int) (int64, error) {
	from := asOf.AddDate(0, 0, -(days - 1))
	sums, err := s.spendingSums(ctx, userID, from, asOf)
	if err != nil {
		return 0, err
	}
	return (sums.Flexible*30 + int64(days)/2) / int64(days), nil
}

func (s *Store) sinkingFunds(ctx context.Context, userID int64, asOf time.Time) ([]SinkingFund, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT f.id, f.name, f.monthly_contribution_cents, f.opening_balance_cents, f.started_on,
       f.accrual_ended_on, f.status,
       COALESCE(SUM(CASE WHEN h.transaction_type = 'expense'
         AND LEFT(h.occurred_at, 10) BETWEEN f.started_on AND ?
         THEN h.actual_amount_cents ELSE 0 END), 0)
FROM ledger_v2_sinking_funds f
LEFT JOIN ledger_v2_transaction_overrides o ON o.sinking_fund_id = f.id AND o.user_id = f.user_id
LEFT JOIN ledger_v2_human_transactions h ON h.id = o.human_transaction_id AND h.user_id = o.user_id
WHERE f.user_id = ?
GROUP BY f.id, f.name, f.monthly_contribution_cents, f.opening_balance_cents, f.started_on,
         f.accrual_ended_on, f.status
ORDER BY f.id
`, asOf.Format(dateLayout), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	funds := make([]SinkingFund, 0)
	for rows.Next() {
		var item SinkingFund
		var startedOn time.Time
		var endedOn sql.NullTime
		var spent int64
		if err := rows.Scan(&item.ID, &item.Name, &item.MonthlyContributionCents, &item.OpeningBalanceCents,
			&startedOn, &endedOn, &item.Status, &spent); err != nil {
			return nil, err
		}
		accrualThrough := asOf
		if endedOn.Valid && endedOn.Time.Before(accrualThrough) {
			accrualThrough = endedOn.Time
			item.AccrualEndedOn = endedOn.Time.Format(dateLayout)
		}
		item.StartedOn = startedOn.Format(dateLayout)
		item.CurrentBalanceCents = ledger.SinkingFundBalance(item.OpeningBalanceCents, item.MonthlyContributionCents, spent, startedOn, accrualThrough)
		funds = append(funds, item)
	}
	return funds, rows.Err()
}

func (s *Store) importCoverage(ctx context.Context, userID int64, requestedFrom, requestedTo time.Time) (time.Time, time.Time, bool, error) {
	var minRaw, maxRaw sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT MIN(LEFT(start_at, 10)), MAX(LEFT(end_at, 10))
FROM ledger_v2_import_batches
WHERE user_id = ? AND status = 'IMPORTED' AND source_type IN ('alipay_csv', 'wechat_xlsx')
`, userID).Scan(&minRaw, &maxRaw)
	if err != nil {
		return time.Time{}, time.Time{}, false, err
	}
	if !minRaw.Valid || !maxRaw.Valid {
		return time.Time{}, time.Time{}, false, nil
	}
	minDate, minErr := time.ParseInLocation(dateLayout, minRaw.String, requestedTo.Location())
	maxDate, maxErr := time.ParseInLocation(dateLayout, maxRaw.String, requestedTo.Location())
	if minErr != nil || maxErr != nil {
		return time.Time{}, time.Time{}, false, fmt.Errorf("parse import coverage: %v %v", minErr, maxErr)
	}
	if minDate.Before(requestedFrom) {
		minDate = requestedFrom
	}
	if maxDate.After(requestedTo) {
		maxDate = requestedTo
	}
	if maxDate.Before(minDate) {
		return time.Time{}, time.Time{}, false, nil
	}
	return minDate, maxDate, true, nil
}
