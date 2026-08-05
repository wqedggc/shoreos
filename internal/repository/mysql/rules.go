package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type ClassificationRule struct {
	ID              int64  `json:"id"`
	Source          string `json:"source"`
	MatchField      string `json:"matchField"`
	MatchValue      string `json:"matchValue"`
	CategoryGroup   string `json:"categoryGroup"`
	CategoryDetail  string `json:"categoryDetail"`
	Purpose         string `json:"purpose"`
	Necessity       string `json:"necessity"`
	Planning        string `json:"planning"`
	Recurrence      string `json:"recurrence"`
	BudgetTreatment string `json:"budgetTreatment"`
	SinkingFundID   *int64 `json:"sinkingFundId"`
	Enabled         bool   `json:"enabled"`
	HitCount        int    `json:"hitCount"`
}

type RuleMatch struct {
	HumanTransactionID int64  `json:"humanTransactionId"`
	HumanTitle         string `json:"humanTitle"`
	MerchantName       string `json:"merchantName"`
	OccurredAt         string `json:"occurredAt"`
	AmountCents        int64  `json:"amountCents"`
}

type RulePreview struct {
	MatchCount int         `json:"matchCount"`
	Matches    []RuleMatch `json:"matches"`
	allMatches []RuleMatch
}

func (s *Store) ClassificationRules(ctx context.Context, userID int64) ([]ClassificationRule, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, 'user', match_field, match_value, COALESCE(category_group, ''), COALESCE(category_detail, ''),
       COALESCE(purpose, ''), COALESCE(necessity, ''), COALESCE(planning, ''), COALESCE(recurrence, ''),
       COALESCE(budget_treatment, ''), sinking_fund_id, enabled, hit_count
FROM ledger_v2_classification_rules WHERE user_id = ? ORDER BY id DESC
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := make([]ClassificationRule, 0)
	for rows.Next() {
		var rule ClassificationRule
		if err := rows.Scan(&rule.ID, &rule.Source, &rule.MatchField, &rule.MatchValue, &rule.CategoryGroup, &rule.CategoryDetail,
			&rule.Purpose, &rule.Necessity, &rule.Planning, &rule.Recurrence, &rule.BudgetTreatment,
			&rule.SinkingFundID, &rule.Enabled, &rule.HitCount); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (s *Store) PreviewClassificationRule(ctx context.Context, userID int64, matchField, matchValue string) (RulePreview, error) {
	return previewRule(ctx, s.db, userID, matchField, matchValue)
}

func (s *Store) CreateClassificationRule(ctx context.Context, userID int64, rule ClassificationRule) (ClassificationRule, RulePreview, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ClassificationRule{}, RulePreview{}, false, err
	}
	defer tx.Rollback()
	if rule.SinkingFundID != nil {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM ledger_v2_sinking_funds WHERE id = ? AND user_id = ?`, *rule.SinkingFundID, userID).Scan(&exists); err != nil {
			return ClassificationRule{}, RulePreview{}, false, err
		}
	}
	result, err := tx.ExecContext(ctx, `
INSERT INTO ledger_v2_classification_rules (
  user_id, match_field, match_value, category_group, category_detail, purpose, necessity,
  planning, recurrence, budget_treatment, sinking_fund_id, enabled, hit_count, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 0, NOW(3), NOW(3))
ON DUPLICATE KEY UPDATE
  id = LAST_INSERT_ID(id), category_group = VALUES(category_group), category_detail = VALUES(category_detail),
  purpose = VALUES(purpose), necessity = VALUES(necessity), planning = VALUES(planning),
  recurrence = VALUES(recurrence), budget_treatment = VALUES(budget_treatment),
  sinking_fund_id = VALUES(sinking_fund_id), enabled = 1, updated_at = NOW(3)
`, userID, rule.MatchField, rule.MatchValue, rule.CategoryGroup, rule.CategoryDetail, rule.Purpose,
		rule.Necessity, rule.Planning, rule.Recurrence, rule.BudgetTreatment, rule.SinkingFundID)
	if err != nil {
		return ClassificationRule{}, RulePreview{}, false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return ClassificationRule{}, RulePreview{}, false, err
	}
	created := affected == 1
	rule.ID, err = result.LastInsertId()
	if err != nil {
		return ClassificationRule{}, RulePreview{}, false, err
	}
	rule.Enabled = true
	rule.Source = "user"
	preview, err := previewRule(ctx, tx, userID, rule.MatchField, rule.MatchValue)
	if err != nil {
		return ClassificationRule{}, RulePreview{}, false, err
	}
	hits, err := applyRule(ctx, tx, userID, rule, preview.allMatches)
	if err != nil {
		return ClassificationRule{}, RulePreview{}, false, err
	}
	rule.HitCount = hits
	if _, err := tx.ExecContext(ctx, `UPDATE ledger_v2_classification_rules SET hit_count = ?, updated_at = NOW(3) WHERE id = ? AND user_id = ?`, hits, rule.ID, userID); err != nil {
		return ClassificationRule{}, RulePreview{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return ClassificationRule{}, RulePreview{}, false, err
	}
	return rule, preview, created, nil
}

func (s *Store) SetClassificationRuleEnabled(ctx context.Context, userID, ruleID int64, enabled bool) error {
	result, err := s.db.ExecContext(ctx, `UPDATE ledger_v2_classification_rules SET enabled = ?, updated_at = NOW(3) WHERE id = ? AND user_id = ?`, enabled, ruleID, userID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ApplyClassificationRules(ctx context.Context, userID int64) error {
	rules, err := s.ClassificationRules(ctx, userID)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		preview, err := s.PreviewClassificationRule(ctx, userID, rule.MatchField, rule.MatchValue)
		if err != nil {
			return err
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		hits, err := applyRule(ctx, tx, userID, rule, preview.allMatches)
		if err != nil {
			tx.Rollback()
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE ledger_v2_classification_rules SET hit_count = ?, updated_at = NOW(3) WHERE id = ? AND user_id = ?`, hits, rule.ID, userID); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func previewRule(ctx context.Context, queryer queryer, userID int64, matchField, matchValue string) (RulePreview, error) {
	condition, arg, err := ruleCondition(matchField, matchValue)
	if err != nil {
		return RulePreview{}, err
	}
	rows, err := queryer.QueryContext(ctx, `
SELECT h.id, COALESCE(NULLIF(o.human_title, ''), h.human_title), COALESCE(h.merchant_name, ''),
       h.occurred_at, h.actual_amount_cents
FROM ledger_v2_human_transactions h
JOIN ledger_v2_human_transaction_sources hs ON hs.human_transaction_id = h.id AND hs.user_id = h.user_id AND hs.source_role = 'primary'
JOIN ledger_v2_normalized_entries e ON e.id = hs.normalized_entry_id AND e.user_id = hs.user_id
JOIN ledger_v2_source_transactions t ON t.id = e.source_transaction_id AND t.user_id = e.user_id
LEFT JOIN ledger_v2_transaction_overrides o ON o.human_transaction_id = h.id AND o.user_id = h.user_id
WHERE h.user_id = ? AND h.transaction_type IN ('expense', 'pending', 'closed', 'zero')
  AND (o.interpretation_source IS NULL OR o.interpretation_source <> 'user_override')
  AND `+condition+`
ORDER BY h.occurred_at DESC, h.id DESC
`, userID, arg)
	if err != nil {
		return RulePreview{}, err
	}
	defer rows.Close()
	preview := RulePreview{Matches: []RuleMatch{}}
	for rows.Next() {
		var item RuleMatch
		if err := rows.Scan(&item.HumanTransactionID, &item.HumanTitle, &item.MerchantName, &item.OccurredAt, &item.AmountCents); err != nil {
			return RulePreview{}, err
		}
		preview.MatchCount++
		preview.allMatches = append(preview.allMatches, item)
		if len(preview.Matches) < 20 {
			preview.Matches = append(preview.Matches, item)
		}
	}
	return preview, rows.Err()
}

func ruleCondition(matchField, matchValue string) (string, any, error) {
	matchValue = strings.TrimSpace(matchValue)
	if matchValue == "" {
		return "", nil, errors.New("empty rule match value")
	}
	switch matchField {
	case "exact_merchant":
		return "h.merchant_name = ?", matchValue, nil
	case "product_keyword":
		return "t.product_description LIKE ?", "%" + matchValue + "%", nil
	case "source_category":
		return "t.source_category = ?", matchValue, nil
	default:
		return "", nil, fmt.Errorf("unsupported rule match field %q", matchField)
	}
}

func applyRule(ctx context.Context, tx *sql.Tx, userID int64, rule ClassificationRule, matches []RuleMatch) (int, error) {
	needsReview := classificationRuleNeedsReview(rule)
	for _, match := range matches {
		_, err := tx.ExecContext(ctx, `
INSERT INTO ledger_v2_transaction_overrides (
  human_transaction_id, user_id, category_group, category_detail, purpose, necessity, planning, recurrence,
  budget_treatment, sinking_fund_id, confidence, evidence_basis, decision_scope, rationale,
  interpretation_source, classification_rule_id, updated_at
) SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'confirmed', 'source_text', 'single_transaction', NULL,
         'user_rule', ?, NOW(3)
WHERE NOT EXISTS (
  SELECT 1 FROM ledger_v2_transaction_overrides existing
  WHERE existing.human_transaction_id = ? AND existing.user_id = ? AND existing.interpretation_source = 'user_override'
)
ON DUPLICATE KEY UPDATE
  category_group = VALUES(category_group), category_detail = VALUES(category_detail), purpose = VALUES(purpose),
  necessity = VALUES(necessity), planning = VALUES(planning), recurrence = VALUES(recurrence),
  budget_treatment = VALUES(budget_treatment), sinking_fund_id = VALUES(sinking_fund_id),
  confidence = 'confirmed', evidence_basis = 'source_text', interpretation_source = 'user_rule',
  classification_rule_id = VALUES(classification_rule_id), updated_at = NOW(3)
`, match.HumanTransactionID, userID, rule.CategoryGroup, rule.CategoryDetail, rule.Purpose, rule.Necessity,
			rule.Planning, rule.Recurrence, rule.BudgetTreatment, rule.SinkingFundID, rule.ID,
			match.HumanTransactionID, userID)
		if err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE ledger_v2_human_transactions h
JOIN ledger_v2_transaction_overrides o
  ON o.human_transaction_id = h.id AND o.user_id = h.user_id
SET h.needs_review = ?, h.updated_at = NOW(3)
WHERE h.id = ? AND h.user_id = ?
  AND o.interpretation_source = 'user_rule' AND o.classification_rule_id = ?
`, needsReview, match.HumanTransactionID, userID, rule.ID); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT IGNORE INTO ledger_v2_rule_applications (rule_id, human_transaction_id, user_id, applied_at)
VALUES (?, ?, ?, NOW(3))
`, rule.ID, match.HumanTransactionID, userID); err != nil {
			return 0, err
		}
	}
	var hits int
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM ledger_v2_rule_applications
WHERE rule_id = ? AND user_id = ?
`, rule.ID, userID).Scan(&hits); err != nil {
		return 0, err
	}
	return hits, nil
}

func classificationRuleNeedsReview(rule ClassificationRule) bool {
	return rule.CategoryGroup == "" || rule.CategoryGroup == "待确认" ||
		rule.CategoryDetail == "" || rule.CategoryDetail == "信息不足" || rule.CategoryDetail == "需要人工判断" ||
		rule.Purpose == "" || rule.Purpose == "未知" ||
		rule.Necessity == "" || rule.Necessity == "未知" ||
		rule.Planning == "" || rule.Planning == "未知" ||
		rule.Recurrence == "" || rule.Recurrence == "未知"
}
