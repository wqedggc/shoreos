package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/wqedggc/shoreos/internal/ledger"
)

type LedgerImport struct {
	ID               int64     `json:"id"`
	SourceType       string    `json:"sourceType"`
	Status           string    `json:"status"`
	OriginalFilename string    `json:"originalFilename"`
	StatementStartAt string    `json:"statementStartAt"`
	StatementEndAt   string    `json:"statementEndAt"`
	ParsedCount      int       `json:"parsedTransactionCount"`
	ImportedCount    int       `json:"importedTransactionCount"`
	DuplicateCount   int       `json:"duplicateTransactionCount"`
	ParserVersion    string    `json:"parserVersion"`
	ParseError       string    `json:"parseError"`
	ImportedAt       time.Time `json:"importedAt"`
	AlreadyImported  bool      `json:"alreadyImported"`
	Reprocessed      bool      `json:"reprocessed"`
}

type LedgerTransaction struct {
	ID                 int64          `json:"id"`
	SourceType         string         `json:"sourceType"`
	Platform           string         `json:"platform"`
	OccurredAt         string         `json:"occurredAt"`
	SourceCategory     string         `json:"sourceCategory"`
	CounterpartyName   string         `json:"counterpartyName"`
	ProductDescription string         `json:"productDescription"`
	Direction          string         `json:"direction"`
	AmountCents        int64          `json:"amountCents"`
	PaymentMethod      string         `json:"paymentMethod"`
	Status             string         `json:"status"`
	NeedsExplanation   bool           `json:"needsExplanation"`
	RawSnapshot        map[string]any `json:"rawSnapshot,omitempty"`
}

type HumanTransaction struct {
	ID                   int64  `json:"id"`
	OccurredAt           string `json:"occurredAt"`
	HumanTitle           string `json:"humanTitle"`
	MerchantName         string `json:"merchantName"`
	SourcePlatform       string `json:"sourcePlatform"`
	TransactionType      string `json:"transactionType"`
	OriginalAmountCents  int64  `json:"originalAmountCents"`
	RefundedAmountCents  int64  `json:"refundedAmountCents"`
	ActualAmountCents    int64  `json:"actualAmountCents"`
	NeedsReview          bool   `json:"needsReview"`
	CategoryGroup        string `json:"categoryGroup"`
	CategoryDetail       string `json:"categoryDetail"`
	Purpose              string `json:"purpose"`
	Necessity            string `json:"necessity"`
	Planning             string `json:"planning"`
	Recurrence           string `json:"recurrence"`
	BudgetTreatment      string `json:"budgetTreatment"`
	SinkingFundID        *int64 `json:"sinkingFundId"`
	Confidence           string `json:"confidence"`
	InterpretationSource string `json:"interpretationSource"`
}

type HumanTransactionFilter struct {
	Limit           int
	From            string
	To              string
	SourcePlatform  string
	TransactionType string
	CategoryGroup   string
	CategoryDetail  string
	Purpose         string
	Necessity       string
	Planning        string
	Recurrence      string
	BudgetTreatment string
	NeedsReview     *bool
}

type LedgerMaterialization struct {
	HumanTransactionCount int `json:"humanTransactionCount"`
	BankEvidenceCount     int `json:"bankEvidenceCount"`
	CandidateLinkCount    int `json:"candidateLinkCount"`
}

type HumanTransactionDetail struct {
	HumanTransaction
	UserNote      string              `json:"userNote"`
	EvidenceBasis string              `json:"evidenceBasis"`
	DecisionScope string              `json:"decisionScope"`
	Rationale     string              `json:"rationale"`
	Sources       []LedgerTransaction `json:"sources"`
}

type LedgerLinkCandidate struct {
	ID                 int64  `json:"id"`
	BankOccurredAt     string `json:"bankOccurredAt"`
	BankCounterparty   string `json:"bankCounterparty"`
	AmountCents        int64  `json:"amountCents"`
	HumanTransactionID int64  `json:"humanTransactionId"`
	HumanTitle         string `json:"humanTitle"`
	CandidateReason    string `json:"candidateReason"`
	MatchStatus        string `json:"matchStatus"`
}

type HumanTransactionOverride struct {
	HumanTitle      string
	UserNote        string
	CategoryGroup   string
	CategoryDetail  string
	Purpose         string
	Necessity       string
	Planning        string
	Recurrence      string
	BudgetTreatment string
	SinkingFundID   *int64
	Confidence      string
	EvidenceBasis   string
	DecisionScope   string
	Rationale       string
}

type sourceFactRow struct {
	ID       int64
	Platform string
	ledger.SourceFact
}

const transactionNeedsReviewSQL = `(h.transaction_type = 'expense' AND (
  h.needs_review = 1 OR COALESCE(o.confidence, 'unknown') <> 'confirmed'
  OR COALESCE(NULLIF(o.category_group, ''), '待确认') = '待确认'
  OR COALESCE(NULLIF(o.category_detail, ''), '信息不足') IN ('信息不足', '需要人工判断')
  OR COALESCE(NULLIF(o.purpose, ''), '未知') = '未知'
  OR COALESCE(NULLIF(o.necessity, ''), '未知') = '未知'
  OR COALESCE(NULLIF(o.planning, ''), '未知') = '未知'
  OR COALESCE(NULLIF(o.recurrence, ''), '未知') = '未知'
))`

func (s *Store) ImportLedger(ctx context.Context, userID int64, storageKey string, parsed ledger.ParsedImport) (LedgerImport, error) {
	var existing LedgerImport
	err := s.db.QueryRowContext(ctx, `
SELECT id, source_type, status, original_filename, COALESCE(start_at, ''), COALESCE(end_at, ''),
       parser_version, parsed_transaction_count, imported_transaction_count, duplicate_transaction_count,
       COALESCE(parse_error, ''), created_at
FROM ledger_v2_import_batches
WHERE user_id = ? AND source_file_hash = ?
`, userID, parsed.File.SHA256).Scan(&existing.ID, &existing.SourceType, &existing.Status, &existing.OriginalFilename,
		&existing.StatementStartAt, &existing.StatementEndAt, &existing.ParserVersion, &existing.ParsedCount,
		&existing.ImportedCount, &existing.DuplicateCount, &existing.ParseError, &existing.ImportedAt)
	if err == nil {
		if existing.ParserVersion == parsed.ParserVersion {
			existing.AlreadyImported = true
			return existing, nil
		}
	}
	if err != nil && err != sql.ErrNoRows {
		return LedgerImport{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return LedgerImport{}, err
	}
	defer tx.Rollback()

	batchID := existing.ID
	if existing.ID == 0 {
		batchResult, err := tx.ExecContext(ctx, `
INSERT INTO ledger_v2_import_batches (
  user_id, source_type, parser_version, original_filename, source_file_hash, storage_key,
  status, start_at, end_at, parsed_transaction_count, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, 'IMPORTING', ?, ?, ?, NOW(3), NOW(3))
`, userID, parsed.SourceType, parsed.ParserVersion, parsed.File.Name, parsed.File.SHA256, storageKey, parsed.Statement.StartAt, parsed.Statement.EndAt, parsed.TransactionCount)
		if err != nil {
			return LedgerImport{}, err
		}
		batchID, err = batchResult.LastInsertId()
		if err != nil {
			return LedgerImport{}, err
		}
	} else if _, err := tx.ExecContext(ctx, `
UPDATE ledger_v2_import_batches
SET parser_version = ?, status = 'IMPORTING', parsed_transaction_count = ?, updated_at = NOW(3)
WHERE id = ? AND user_id = ?
`, parsed.ParserVersion, parsed.TransactionCount, batchID, userID); err != nil {
		return LedgerImport{}, err
	}

	accountResult, err := tx.ExecContext(ctx, `
INSERT INTO ledger_v2_source_accounts (user_id, platform, account_key, created_at, updated_at)
VALUES (?, ?, ?, NOW(3), NOW(3))
ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id), updated_at = NOW(3)
`, userID, parsed.Statement.Platform, parsed.Statement.AccountKey)
	if err != nil {
		return LedgerImport{}, err
	}
	accountID, err := accountResult.LastInsertId()
	if err != nil {
		return LedgerImport{}, err
	}

	for _, transaction := range parsed.Transactions {
		rawSnapshot, err := json.Marshal(transaction.RawSnapshot)
		if err != nil {
			return LedgerImport{}, fmt.Errorf("encode source transaction snapshot: %w", err)
		}
		result, err := tx.ExecContext(ctx, `
INSERT IGNORE INTO ledger_v2_source_transactions (
  user_id, source_account_id, import_batch_id, source_type, occurred_at, source_category,
  counterparty_name, counterparty_account, product_description, direction, amount_cents,
  payment_method, transaction_status, platform_order_no, merchant_order_no, remark,
  raw_snapshot, raw_row_hash, transaction_fingerprint, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CAST(? AS JSON), ?, ?, NOW(3), NOW(3))
`, userID, accountID, batchID, parsed.SourceType, transaction.OccurredAt, transaction.Category,
			transaction.CounterpartyName, transaction.CounterpartyAccount, transaction.ProductDescription,
			transaction.Direction, transaction.AmountCents, transaction.PaymentMethod, transaction.Status,
			transaction.PlatformOrderNo, transaction.MerchantOrderNo, transaction.Remark, string(rawSnapshot),
			transaction.RawRowHash, transaction.TransactionFingerprint)
		if err != nil {
			return LedgerImport{}, err
		}
		if _, err := result.RowsAffected(); err != nil {
			return LedgerImport{}, err
		}
	}
	var totalImported int
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM ledger_v2_source_transactions
WHERE import_batch_id = ? AND user_id = ?
`, batchID, userID).Scan(&totalImported); err != nil {
		return LedgerImport{}, err
	}
	finalDuplicates := parsed.TransactionCount - totalImported
	if finalDuplicates < 0 {
		finalDuplicates = 0
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE ledger_v2_import_batches
SET status = 'IMPORTED', imported_transaction_count = ?, duplicate_transaction_count = ?, updated_at = NOW(3)
WHERE id = ? AND user_id = ?
`, totalImported, finalDuplicates, batchID, userID); err != nil {
		return LedgerImport{}, err
	}
	if err := tx.Commit(); err != nil {
		return LedgerImport{}, err
	}
	return LedgerImport{ID: batchID, SourceType: parsed.SourceType, Status: "IMPORTED", OriginalFilename: parsed.File.Name,
		StatementStartAt: parsed.Statement.StartAt, StatementEndAt: parsed.Statement.EndAt, ParsedCount: parsed.TransactionCount,
		ImportedCount: totalImported, DuplicateCount: finalDuplicates, ParserVersion: parsed.ParserVersion,
		ImportedAt: time.Now(), Reprocessed: existing.ID != 0}, nil
}

func (s *Store) LedgerImports(ctx context.Context, userID int64) ([]LedgerImport, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, source_type, status, original_filename, COALESCE(start_at, ''), COALESCE(end_at, ''),
       parsed_transaction_count, imported_transaction_count, duplicate_transaction_count,
       parser_version, COALESCE(parse_error, ''), created_at
FROM ledger_v2_import_batches
WHERE user_id = ?
ORDER BY id DESC
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	imports := make([]LedgerImport, 0)
	for rows.Next() {
		var item LedgerImport
		if err := rows.Scan(&item.ID, &item.SourceType, &item.Status, &item.OriginalFilename, &item.StatementStartAt,
			&item.StatementEndAt, &item.ParsedCount, &item.ImportedCount, &item.DuplicateCount,
			&item.ParserVersion, &item.ParseError, &item.ImportedAt); err != nil {
			return nil, err
		}
		imports = append(imports, item)
	}
	return imports, rows.Err()
}

func (s *Store) LedgerTransactions(ctx context.Context, userID int64, limit int) ([]LedgerTransaction, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT t.id, t.source_type, a.platform, t.occurred_at, COALESCE(t.source_category, ''),
       COALESCE(t.counterparty_name, ''), COALESCE(t.product_description, ''),
       t.direction, t.amount_cents, COALESCE(t.payment_method, ''), COALESCE(t.transaction_status, '')
FROM ledger_v2_source_transactions t
JOIN ledger_v2_source_accounts a ON a.id = t.source_account_id AND a.user_id = t.user_id
WHERE t.user_id = ?
ORDER BY t.occurred_at DESC, t.id DESC
LIMIT ?
`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	transactions := make([]LedgerTransaction, 0)
	for rows.Next() {
		var item LedgerTransaction
		if err := rows.Scan(&item.ID, &item.SourceType, &item.Platform, &item.OccurredAt, &item.SourceCategory,
			&item.CounterpartyName, &item.ProductDescription, &item.Direction, &item.AmountCents, &item.PaymentMethod, &item.Status); err != nil {
			return nil, err
		}
		item.NeedsExplanation = item.CounterpartyName == "" && item.ProductDescription == ""
		transactions = append(transactions, item)
	}
	return transactions, rows.Err()
}

func (s *Store) LedgerTransaction(ctx context.Context, userID, transactionID int64) (LedgerTransaction, error) {
	var item LedgerTransaction
	var rawSnapshot string
	err := s.db.QueryRowContext(ctx, `
SELECT t.id, t.source_type, a.platform, t.occurred_at, COALESCE(t.source_category, ''),
       COALESCE(t.counterparty_name, ''), COALESCE(t.product_description, ''),
       t.direction, t.amount_cents, COALESCE(t.payment_method, ''), COALESCE(t.transaction_status, ''), t.raw_snapshot
FROM ledger_v2_source_transactions t
JOIN ledger_v2_source_accounts a ON a.id = t.source_account_id AND a.user_id = t.user_id
WHERE t.user_id = ? AND t.id = ?
`, userID, transactionID).Scan(&item.ID, &item.SourceType, &item.Platform, &item.OccurredAt, &item.SourceCategory,
		&item.CounterpartyName, &item.ProductDescription, &item.Direction, &item.AmountCents, &item.PaymentMethod, &item.Status, &rawSnapshot)
	if err != nil {
		return LedgerTransaction{}, err
	}
	if err := json.Unmarshal([]byte(rawSnapshot), &item.RawSnapshot); err != nil {
		return LedgerTransaction{}, fmt.Errorf("decode source transaction snapshot: %w", err)
	}
	item.NeedsExplanation = item.CounterpartyName == "" && item.ProductDescription == ""
	return item, nil
}

func (s *Store) MaterializeLedger(ctx context.Context, userID int64) (LedgerMaterialization, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return LedgerMaterialization{}, err
	}
	defer tx.Rollback()

	facts, err := loadSourceFacts(ctx, tx, userID, "t.source_type IN ('alipay_csv', 'wechat_xlsx')")
	if err != nil {
		return LedgerMaterialization{}, err
	}
	result := LedgerMaterialization{}
	for _, fact := range facts {
		projection := ledger.ProjectHumanTransaction(fact.SourceFact)
		entryID, err := upsertNormalizedEntry(ctx, tx, userID, fact.ID, projection.EntryKind, fact.OccurredAt, fact.AmountCents, fact.Status, "payment")
		if err != nil {
			return LedgerMaterialization{}, err
		}
		humanID, err := upsertHumanTransaction(ctx, tx, userID, entryID, fact, projection)
		if err != nil {
			return LedgerMaterialization{}, err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT IGNORE INTO ledger_v2_human_transaction_sources (human_transaction_id, normalized_entry_id, user_id, source_role, created_at)
VALUES (?, ?, ?, 'primary', NOW(3))
		`, humanID, entryID, userID); err != nil {
			return LedgerMaterialization{}, err
		}
		result.HumanTransactionCount++
	}

	bankFacts, err := loadSourceFacts(ctx, tx, userID, "t.source_type = 'cmb_bank_pdf_text'")
	if err != nil {
		return LedgerMaterialization{}, err
	}
	for _, fact := range bankFacts {
		if _, err := upsertNormalizedEntry(ctx, tx, userID, fact.ID, "bank_evidence", fact.OccurredAt, fact.AmountCents, fact.Status, "bank"); err != nil {
			return LedgerMaterialization{}, err
		}
		result.BankEvidenceCount++
	}

	rows, err := tx.QueryContext(ctx, `
SELECT bank_entry.id, payment_entry.id
FROM ledger_v2_normalized_entries bank_entry
JOIN ledger_v2_source_transactions bank_source ON bank_source.id = bank_entry.source_transaction_id
JOIN ledger_v2_normalized_entries payment_entry ON payment_entry.user_id = bank_entry.user_id
JOIN ledger_v2_source_transactions payment_source ON payment_source.id = payment_entry.source_transaction_id
WHERE bank_entry.user_id = ?
  AND bank_source.source_type = 'cmb_bank_pdf_text'
  AND payment_source.source_type IN ('alipay_csv', 'wechat_xlsx')
  AND LEFT(bank_source.occurred_at, 10) = LEFT(payment_source.occurred_at, 10)
  AND ABS(bank_source.amount_cents) = ABS(payment_source.amount_cents)
  AND bank_source.direction = payment_source.direction
`, userID)
	if err != nil {
		return LedgerMaterialization{}, err
	}
	var candidates [][2]int64
	for rows.Next() {
		var pair [2]int64
		if err := rows.Scan(&pair[0], &pair[1]); err != nil {
			rows.Close()
			return LedgerMaterialization{}, err
		}
		candidates = append(candidates, pair)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return LedgerMaterialization{}, err
	}
	rows.Close()
	for _, pair := range candidates {
		_, err := tx.ExecContext(ctx, `
INSERT IGNORE INTO ledger_v2_entry_links (
  user_id, left_entry_id, right_entry_id, link_type, match_status, match_method,
  confidence_bps, evidence, candidate_reason, created_at, updated_at
) VALUES (?, ?, ?, 'payment_evidence', 'candidate', 'same_day_same_amount', 6000,
          JSON_OBJECT('rule', 'same_day_same_amount'), '同日同金额：需人工确认是否为同一资金链路', NOW(3), NOW(3))
		`, userID, pair[0], pair[1])
		if err != nil {
			return LedgerMaterialization{}, err
		}
		result.CandidateLinkCount++
	}
	if err := tx.Commit(); err != nil {
		return LedgerMaterialization{}, err
	}
	if err := s.ApplyClassificationRules(ctx, userID); err != nil {
		return LedgerMaterialization{}, err
	}
	return result, nil
}

func (s *Store) HumanTransactions(ctx context.Context, userID int64, filter HumanTransactionFilter) ([]HumanTransaction, error) {
	query := fmt.Sprintf(`
SELECT h.id, h.occurred_at, COALESCE(NULLIF(o.human_title, ''), h.human_title), COALESCE(h.merchant_name, ''),
       COALESCE(h.source_platform, ''), h.transaction_type, h.original_amount_cents, h.refunded_amount_cents,
       h.actual_amount_cents, %s,
       COALESCE(NULLIF(o.category_group, ''), '待确认'), COALESCE(NULLIF(o.category_detail, ''), '信息不足'),
       COALESCE(NULLIF(o.purpose, ''), '未知'), COALESCE(NULLIF(o.necessity, ''), '未知'),
       COALESCE(NULLIF(o.planning, ''), '未知'), COALESCE(NULLIF(o.recurrence, ''), '未知'),
       COALESCE(NULLIF(o.budget_treatment, ''), 'flexible'), o.sinking_fund_id,
       COALESCE(o.confidence, 'unknown'), COALESCE(o.interpretation_source, 'default_projection')
FROM ledger_v2_human_transactions h
LEFT JOIN ledger_v2_transaction_overrides o ON o.human_transaction_id = h.id AND o.user_id = h.user_id
WHERE h.user_id = ?`, transactionNeedsReviewSQL)
	args := []any{userID}
	add := func(clause string, value any) {
		query += " AND " + clause
		args = append(args, value)
	}
	if filter.From != "" {
		add("LEFT(h.occurred_at, 10) >= ?", filter.From)
	}
	if filter.To != "" {
		add("LEFT(h.occurred_at, 10) <= ?", filter.To)
	}
	if filter.SourcePlatform != "" {
		add("h.source_platform = ?", filter.SourcePlatform)
	}
	if filter.TransactionType != "" {
		add("h.transaction_type = ?", filter.TransactionType)
	}
	stringFilters := []struct {
		value  string
		clause string
	}{
		{filter.CategoryGroup, "COALESCE(NULLIF(o.category_group, ''), '待确认') = ?"},
		{filter.CategoryDetail, "COALESCE(NULLIF(o.category_detail, ''), '信息不足') = ?"},
		{filter.Purpose, "COALESCE(NULLIF(o.purpose, ''), '未知') = ?"},
		{filter.Necessity, "COALESCE(NULLIF(o.necessity, ''), '未知') = ?"},
		{filter.Planning, "COALESCE(NULLIF(o.planning, ''), '未知') = ?"},
		{filter.Recurrence, "COALESCE(NULLIF(o.recurrence, ''), '未知') = ?"},
		{filter.BudgetTreatment, "COALESCE(NULLIF(o.budget_treatment, ''), 'flexible') = ?"},
	}
	for _, item := range stringFilters {
		if strings.TrimSpace(item.value) != "" {
			add(item.clause, item.value)
		}
	}
	if filter.NeedsReview != nil {
		add(transactionNeedsReviewSQL+" = ?", *filter.NeedsReview)
	}
	query += " ORDER BY h.occurred_at DESC, h.id DESC LIMIT ?"
	args = append(args, filter.Limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	transactions := make([]HumanTransaction, 0)
	for rows.Next() {
		var item HumanTransaction
		if err := rows.Scan(&item.ID, &item.OccurredAt, &item.HumanTitle, &item.MerchantName, &item.SourcePlatform,
			&item.TransactionType, &item.OriginalAmountCents, &item.RefundedAmountCents, &item.ActualAmountCents,
			&item.NeedsReview, &item.CategoryGroup, &item.CategoryDetail,
			&item.Purpose, &item.Necessity, &item.Planning, &item.Recurrence, &item.BudgetTreatment, &item.SinkingFundID,
			&item.Confidence, &item.InterpretationSource); err != nil {
			return nil, err
		}
		transactions = append(transactions, item)
	}
	return transactions, rows.Err()
}

func (s *Store) HumanTransaction(ctx context.Context, userID, humanID int64) (HumanTransactionDetail, error) {
	var detail HumanTransactionDetail
	err := s.db.QueryRowContext(ctx, fmt.Sprintf(`
SELECT h.id, h.occurred_at, COALESCE(NULLIF(o.human_title, ''), h.human_title), COALESCE(h.merchant_name, ''),
       COALESCE(h.source_platform, ''), h.transaction_type, h.original_amount_cents, h.refunded_amount_cents,
       h.actual_amount_cents, %s,
       COALESCE(NULLIF(o.category_group, ''), '待确认'), COALESCE(NULLIF(o.category_detail, ''), '信息不足'),
       COALESCE(NULLIF(o.purpose, ''), '未知'), COALESCE(NULLIF(o.necessity, ''), '未知'),
       COALESCE(NULLIF(o.planning, ''), '未知'), COALESCE(NULLIF(o.recurrence, ''), '未知'),
       COALESCE(NULLIF(o.budget_treatment, ''), 'flexible'), o.sinking_fund_id,
       COALESCE(o.confidence, 'unknown'), COALESCE(o.interpretation_source, 'default_projection'),
       COALESCE(o.user_note, ''), COALESCE(o.evidence_basis, 'insufficient'),
       COALESCE(o.decision_scope, 'single_transaction'), COALESCE(o.rationale, '')
FROM ledger_v2_human_transactions h
LEFT JOIN ledger_v2_transaction_overrides o ON o.human_transaction_id = h.id AND o.user_id = h.user_id
WHERE h.user_id = ? AND h.id = ?
`, transactionNeedsReviewSQL), userID, humanID).Scan(&detail.ID, &detail.OccurredAt, &detail.HumanTitle, &detail.MerchantName, &detail.SourcePlatform,
		&detail.TransactionType, &detail.OriginalAmountCents, &detail.RefundedAmountCents, &detail.ActualAmountCents,
		&detail.NeedsReview, &detail.CategoryGroup, &detail.CategoryDetail,
		&detail.Purpose, &detail.Necessity, &detail.Planning, &detail.Recurrence, &detail.BudgetTreatment, &detail.SinkingFundID,
		&detail.Confidence, &detail.InterpretationSource, &detail.UserNote, &detail.EvidenceBasis, &detail.DecisionScope, &detail.Rationale)
	if err != nil {
		return HumanTransactionDetail{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT t.id, t.source_type, a.platform, t.occurred_at, COALESCE(t.source_category, ''),
       COALESCE(t.counterparty_name, ''), COALESCE(t.product_description, ''), t.direction,
       t.amount_cents, COALESCE(t.payment_method, ''), COALESCE(t.transaction_status, ''), t.raw_snapshot
FROM ledger_v2_human_transaction_sources hs
JOIN ledger_v2_normalized_entries e ON e.id = hs.normalized_entry_id AND e.user_id = hs.user_id
JOIN ledger_v2_source_transactions t ON t.id = e.source_transaction_id AND t.user_id = e.user_id
JOIN ledger_v2_source_accounts a ON a.id = t.source_account_id AND a.user_id = t.user_id
WHERE hs.user_id = ? AND hs.human_transaction_id = ?
ORDER BY hs.source_role, t.id
`, userID, humanID)
	if err != nil {
		return HumanTransactionDetail{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var source LedgerTransaction
		var raw string
		if err := rows.Scan(&source.ID, &source.SourceType, &source.Platform, &source.OccurredAt, &source.SourceCategory,
			&source.CounterpartyName, &source.ProductDescription, &source.Direction, &source.AmountCents, &source.PaymentMethod, &source.Status, &raw); err != nil {
			return HumanTransactionDetail{}, err
		}
		if err := json.Unmarshal([]byte(raw), &source.RawSnapshot); err != nil {
			return HumanTransactionDetail{}, fmt.Errorf("decode human transaction source snapshot: %w", err)
		}
		detail.Sources = append(detail.Sources, source)
	}
	return detail, rows.Err()
}

func (s *Store) UpdateHumanTransactionOverride(ctx context.Context, userID, humanID int64, override HumanTransactionOverride) error {
	result, err := s.db.ExecContext(ctx, `
INSERT INTO ledger_v2_transaction_overrides (
  human_transaction_id, user_id, human_title, user_note, category_group, category_detail,
  purpose, necessity, planning, recurrence, budget_treatment, sinking_fund_id,
  confidence, evidence_basis, decision_scope, rationale, interpretation_source, classification_rule_id, updated_at
) SELECT id, user_id, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'user_override', NULL, NOW(3)
  FROM ledger_v2_human_transactions
  WHERE id = ? AND user_id = ?
    AND (? IS NULL OR EXISTS (
      SELECT 1 FROM ledger_v2_sinking_funds f WHERE f.id = ? AND f.user_id = ?
    ))
ON DUPLICATE KEY UPDATE human_title = VALUES(human_title), user_note = VALUES(user_note),
  category_group = VALUES(category_group), category_detail = VALUES(category_detail), purpose = VALUES(purpose),
  necessity = VALUES(necessity), planning = VALUES(planning), recurrence = VALUES(recurrence),
  budget_treatment = VALUES(budget_treatment), sinking_fund_id = VALUES(sinking_fund_id),
  confidence = VALUES(confidence), evidence_basis = VALUES(evidence_basis), decision_scope = VALUES(decision_scope),
  rationale = VALUES(rationale), interpretation_source = 'user_override', classification_rule_id = NULL, updated_at = NOW(3)
`, override.HumanTitle, override.UserNote, override.CategoryGroup, override.CategoryDetail, override.Purpose,
		override.Necessity, override.Planning, override.Recurrence, override.BudgetTreatment, override.SinkingFundID,
		override.Confidence, override.EvidenceBasis, override.DecisionScope, override.Rationale,
		humanID, userID, override.SinkingFundID, override.SinkingFundID, userID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return sql.ErrNoRows
	}
	complete := override.Confidence == "confirmed" && override.CategoryGroup != "" && override.CategoryGroup != "待确认" &&
		override.CategoryDetail != "" && override.CategoryDetail != "信息不足" &&
		override.Purpose != "" && override.Purpose != "未知" && override.Necessity != "" && override.Necessity != "未知" &&
		override.Planning != "" && override.Planning != "未知" && override.Recurrence != "" && override.Recurrence != "未知"
	if _, err := s.db.ExecContext(ctx, `UPDATE ledger_v2_human_transactions SET needs_review = ?, updated_at = NOW(3) WHERE id = ? AND user_id = ?`, !complete, humanID, userID); err != nil {
		return err
	}
	return nil
}

func (s *Store) LedgerLinkCandidates(ctx context.Context, userID int64, limit int) ([]LedgerLinkCandidate, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT l.id, bank_source.occurred_at, COALESCE(bank_source.counterparty_name, ''), ABS(bank_source.amount_cents),
       human.id, human.human_title, COALESCE(l.candidate_reason, ''), l.match_status
FROM ledger_v2_entry_links l
JOIN ledger_v2_normalized_entries bank_entry ON bank_entry.id = l.left_entry_id AND bank_entry.user_id = l.user_id
JOIN ledger_v2_source_transactions bank_source ON bank_source.id = bank_entry.source_transaction_id AND bank_source.user_id = bank_entry.user_id
JOIN ledger_v2_normalized_entries payment_entry ON payment_entry.id = l.right_entry_id AND payment_entry.user_id = l.user_id
JOIN ledger_v2_human_transactions human ON human.primary_entry_id = payment_entry.id AND human.user_id = payment_entry.user_id
WHERE l.user_id = ? AND l.link_type = 'payment_evidence' AND l.match_status = 'candidate'
ORDER BY bank_source.occurred_at DESC, l.id DESC
LIMIT ?
`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	candidates := make([]LedgerLinkCandidate, 0)
	for rows.Next() {
		var item LedgerLinkCandidate
		if err := rows.Scan(&item.ID, &item.BankOccurredAt, &item.BankCounterparty, &item.AmountCents, &item.HumanTransactionID, &item.HumanTitle, &item.CandidateReason, &item.MatchStatus); err != nil {
			return nil, err
		}
		candidates = append(candidates, item)
	}
	return candidates, rows.Err()
}

func (s *Store) DecideLedgerLinkCandidate(ctx context.Context, userID, candidateID int64, decision string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var bankEntryID, paymentEntryID int64
	err = tx.QueryRowContext(ctx, `
SELECT left_entry_id, right_entry_id
FROM ledger_v2_entry_links
WHERE id = ? AND user_id = ? AND link_type = 'payment_evidence' AND match_status = 'candidate'
FOR UPDATE
`, candidateID, userID).Scan(&bankEntryID, &paymentEntryID)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE ledger_v2_entry_links SET match_status = ?, updated_at = NOW(3) WHERE id = ? AND user_id = ?`, decision, candidateID, userID); err != nil {
		return err
	}
	if decision == "confirmed" {
		if _, err := tx.ExecContext(ctx, `
INSERT IGNORE INTO ledger_v2_human_transaction_sources (human_transaction_id, normalized_entry_id, user_id, source_role, created_at)
SELECT id, ?, user_id, 'funding_evidence', NOW(3)
FROM ledger_v2_human_transactions
WHERE user_id = ? AND primary_entry_id = ?
`, bankEntryID, userID, paymentEntryID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func loadSourceFacts(ctx context.Context, tx *sql.Tx, userID int64, sourcePredicate string) ([]sourceFactRow, error) {
	query := `
SELECT t.id, a.platform, t.source_type, t.occurred_at, COALESCE(t.source_category, ''),
       COALESCE(t.counterparty_name, ''), COALESCE(t.product_description, ''), t.direction,
       t.amount_cents, COALESCE(t.payment_method, ''), COALESCE(t.transaction_status, '')
FROM ledger_v2_source_transactions t
JOIN ledger_v2_source_accounts a ON a.id = t.source_account_id AND a.user_id = t.user_id
WHERE t.user_id = ? AND ` + sourcePredicate + `
ORDER BY t.id ASC`
	rows, err := tx.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	facts := make([]sourceFactRow, 0)
	for rows.Next() {
		var fact sourceFactRow
		if err := rows.Scan(&fact.ID, &fact.Platform, &fact.SourceType, &fact.OccurredAt, &fact.SourceCategory,
			&fact.CounterpartyName, &fact.ProductDescription, &fact.Direction, &fact.AmountCents, &fact.PaymentMethod, &fact.Status); err != nil {
			return nil, err
		}
		facts = append(facts, fact)
	}
	return facts, rows.Err()
}

func upsertNormalizedEntry(ctx context.Context, tx *sql.Tx, userID, sourceID int64, entryKind, occurredAt string, amountCents int64, status, evidenceLevel string) (int64, error) {
	result, err := tx.ExecContext(ctx, `
INSERT INTO ledger_v2_normalized_entries (user_id, source_transaction_id, entry_kind, occurred_at, amount_cents, status, evidence_level, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, NOW(3), NOW(3))
ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id), entry_kind = VALUES(entry_kind), status = VALUES(status), evidence_level = VALUES(evidence_level), updated_at = NOW(3)
`, userID, sourceID, entryKind, occurredAt, amountCents, status, evidenceLevel)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func upsertHumanTransaction(ctx context.Context, tx *sql.Tx, userID, entryID int64, fact sourceFactRow, projection ledger.HumanProjection) (int64, error) {
	result, err := tx.ExecContext(ctx, `
INSERT INTO ledger_v2_human_transactions (
  user_id, primary_entry_id, human_title, merchant_name, transaction_type, occurred_at, source_platform,
  original_amount_cents, refunded_amount_cents, actual_amount_cents, needs_review, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(3), NOW(3))
ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id), human_title = VALUES(human_title), merchant_name = VALUES(merchant_name),
  transaction_type = VALUES(transaction_type), occurred_at = VALUES(occurred_at), source_platform = VALUES(source_platform),
  original_amount_cents = VALUES(original_amount_cents), refunded_amount_cents = VALUES(refunded_amount_cents),
  actual_amount_cents = VALUES(actual_amount_cents), needs_review = VALUES(needs_review), updated_at = NOW(3)
`, userID, entryID, projection.HumanTitle, projection.MerchantName, projection.TransactionType, fact.OccurredAt, fact.Platform,
		projection.OriginalAmountCents, projection.RefundedAmountCents, projection.ActualAmountCents, projection.NeedsReview)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
