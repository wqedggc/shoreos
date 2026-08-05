package mysql

import (
	"context"
	"fmt"
	"time"
)

type PeriodAnalysisTotals struct {
	ActualExpenseCents          int64 `json:"actualExpenseCents"`
	IncomeAmountCents           int64 `json:"incomeAmountCents"`
	RefundAmountCents           int64 `json:"refundAmountCents"`
	InternalTransferAmountCents int64 `json:"internalTransferAmountCents"`
	PendingAmountCents          int64 `json:"pendingAmountCents"`
	ClosedAmountCents           int64 `json:"closedAmountCents"`
	TransactionCount            int   `json:"transactionCount"`
	ExpenseCount                int   `json:"expenseCount"`
	NeedsReviewCount            int   `json:"needsReviewCount"`
	ZeroCount                   int   `json:"zeroCount"`
}

type PeriodTimelineItem struct {
	PeriodStart        string `json:"periodStart"`
	ActualExpenseCents int64  `json:"actualExpenseCents"`
	IncomeAmountCents  int64  `json:"incomeAmountCents"`
	RefundAmountCents  int64  `json:"refundAmountCents"`
	PendingAmountCents int64  `json:"pendingAmountCents"`
	TransactionCount   int    `json:"transactionCount"`
	NeedsReviewCount   int    `json:"needsReviewCount"`
}

type PeriodBreakdownItem struct {
	Key         string `json:"key"`
	AmountCents int64  `json:"amountCents"`
	Count       int    `json:"count"`
}

type PeriodBreakdowns struct {
	CategoryGroup []PeriodBreakdownItem `json:"categoryGroup"`
	Purpose       []PeriodBreakdownItem `json:"purpose"`
	Necessity     []PeriodBreakdownItem `json:"necessity"`
	Planning      []PeriodBreakdownItem `json:"planning"`
	Recurrence    []PeriodBreakdownItem `json:"recurrence"`
}

type LedgerPeriodAnalysis struct {
	From         string               `json:"from"`
	To           string               `json:"to"`
	Currency     string               `json:"currency"`
	DataCoverage string               `json:"dataCoverage"`
	Totals       PeriodAnalysisTotals `json:"totals"`
	Timeline     []PeriodTimelineItem `json:"timeline"`
	Breakdowns   PeriodBreakdowns     `json:"breakdowns"`
}

func (s *Store) PeriodAnalysis(ctx context.Context, userID int64, from, to time.Time, bucket string) (LedgerPeriodAnalysis, error) {
	analysis := LedgerPeriodAnalysis{
		From: from.Format(dateLayout), To: to.Format(dateLayout), Currency: "CNY",
		Timeline: []PeriodTimelineItem{}, Breakdowns: PeriodBreakdowns{
			CategoryGroup: []PeriodBreakdownItem{}, Purpose: []PeriodBreakdownItem{}, Necessity: []PeriodBreakdownItem{},
			Planning: []PeriodBreakdownItem{}, Recurrence: []PeriodBreakdownItem{},
		},
	}
	if err := s.db.QueryRowContext(ctx, fmt.Sprintf(`
SELECT
  COALESCE(SUM(CASE WHEN h.transaction_type = 'expense' THEN h.actual_amount_cents ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN h.transaction_type = 'income' THEN h.original_amount_cents ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN h.transaction_type = 'refund' THEN h.refunded_amount_cents ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN h.transaction_type = 'internal' THEN h.original_amount_cents ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN h.transaction_type = 'pending' THEN h.original_amount_cents ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN h.transaction_type = 'closed' THEN h.original_amount_cents ELSE 0 END), 0),
  COUNT(*),
  COALESCE(SUM(h.transaction_type = 'expense'), 0),
  COALESCE(SUM(%s), 0),
  COALESCE(SUM(h.transaction_type = 'zero'), 0)
FROM ledger_v2_human_transactions h
LEFT JOIN ledger_v2_transaction_overrides o ON o.human_transaction_id = h.id AND o.user_id = h.user_id
WHERE h.user_id = ? AND LEFT(h.occurred_at, 10) BETWEEN ? AND ?
`, transactionNeedsReviewSQL), userID, analysis.From, analysis.To).Scan(
		&analysis.Totals.ActualExpenseCents, &analysis.Totals.IncomeAmountCents, &analysis.Totals.RefundAmountCents,
		&analysis.Totals.InternalTransferAmountCents, &analysis.Totals.PendingAmountCents, &analysis.Totals.ClosedAmountCents,
		&analysis.Totals.TransactionCount, &analysis.Totals.ExpenseCount, &analysis.Totals.NeedsReviewCount, &analysis.Totals.ZeroCount,
	); err != nil {
		return LedgerPeriodAnalysis{}, err
	}

	periodExpression := "LEFT(h.occurred_at, 10)"
	if bucket == "month" {
		periodExpression = "CONCAT(LEFT(h.occurred_at, 7), '-01')"
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT %s,
  COALESCE(SUM(CASE WHEN h.transaction_type = 'expense' THEN h.actual_amount_cents ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN h.transaction_type = 'income' THEN h.original_amount_cents ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN h.transaction_type = 'refund' THEN h.refunded_amount_cents ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN h.transaction_type = 'pending' THEN h.original_amount_cents ELSE 0 END), 0),
  COUNT(*), SUM(%s)
FROM ledger_v2_human_transactions h
LEFT JOIN ledger_v2_transaction_overrides o ON o.human_transaction_id = h.id AND o.user_id = h.user_id
WHERE h.user_id = ? AND LEFT(h.occurred_at, 10) BETWEEN ? AND ?
GROUP BY %s ORDER BY %s
`, periodExpression, transactionNeedsReviewSQL, periodExpression, periodExpression), userID, analysis.From, analysis.To)
	if err != nil {
		return LedgerPeriodAnalysis{}, err
	}
	for rows.Next() {
		var item PeriodTimelineItem
		if err := rows.Scan(&item.PeriodStart, &item.ActualExpenseCents, &item.IncomeAmountCents, &item.RefundAmountCents,
			&item.PendingAmountCents, &item.TransactionCount, &item.NeedsReviewCount); err != nil {
			rows.Close()
			return LedgerPeriodAnalysis{}, err
		}
		analysis.Timeline = append(analysis.Timeline, item)
	}
	if err := rows.Close(); err != nil {
		return LedgerPeriodAnalysis{}, err
	}

	dimensions := []struct {
		target     *[]PeriodBreakdownItem
		expression string
	}{
		{&analysis.Breakdowns.CategoryGroup, "COALESCE(NULLIF(o.category_group, ''), '待确认')"},
		{&analysis.Breakdowns.Purpose, "COALESCE(NULLIF(o.purpose, ''), '未知')"},
		{&analysis.Breakdowns.Necessity, "COALESCE(NULLIF(o.necessity, ''), '未知')"},
		{&analysis.Breakdowns.Planning, "COALESCE(NULLIF(o.planning, ''), '未知')"},
		{&analysis.Breakdowns.Recurrence, "COALESCE(NULLIF(o.recurrence, ''), '未知')"},
	}
	for _, dimension := range dimensions {
		items, err := s.periodBreakdown(ctx, userID, analysis.From, analysis.To, dimension.expression)
		if err != nil {
			return LedgerPeriodAnalysis{}, err
		}
		*dimension.target = items
	}

	coveredFrom, coveredTo, hasData, err := s.importCoverage(ctx, userID, from, to)
	if err != nil {
		return LedgerPeriodAnalysis{}, err
	}
	switch {
	case !hasData:
		analysis.DataCoverage = "insufficient"
	case coveredFrom.After(from) || coveredTo.Before(to) || analysis.Totals.NeedsReviewCount > 0:
		analysis.DataCoverage = "partial"
	default:
		analysis.DataCoverage = "complete"
	}
	return analysis, nil
}

func (s *Store) periodBreakdown(ctx context.Context, userID int64, from, to, expression string) ([]PeriodBreakdownItem, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT %s, COALESCE(SUM(h.actual_amount_cents), 0), COUNT(*)
FROM ledger_v2_human_transactions h
LEFT JOIN ledger_v2_transaction_overrides o ON o.human_transaction_id = h.id AND o.user_id = h.user_id
WHERE h.user_id = ? AND h.transaction_type = 'expense' AND LEFT(h.occurred_at, 10) BETWEEN ? AND ?
GROUP BY %s ORDER BY SUM(h.actual_amount_cents) DESC, %s
`, expression, expression, expression), userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]PeriodBreakdownItem, 0)
	for rows.Next() {
		var item PeriodBreakdownItem
		if err := rows.Scan(&item.Key, &item.AmountCents, &item.Count); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
