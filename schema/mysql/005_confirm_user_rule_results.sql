USE shoreos;

-- User-authored rules are explicit decisions, not machine guesses. Existing
-- rule projections created before this migration must therefore be confirmed.
UPDATE ledger_v2_transaction_overrides
SET confidence = 'confirmed', updated_at = NOW(3)
WHERE interpretation_source = 'user_rule'
  AND confidence <> 'confirmed';

UPDATE ledger_v2_human_transactions h
JOIN ledger_v2_transaction_overrides o
  ON o.human_transaction_id = h.id AND o.user_id = h.user_id
SET h.needs_review = CASE
  WHEN COALESCE(NULLIF(o.category_group, ''), '待确认') = '待确认'
    OR COALESCE(NULLIF(o.category_detail, ''), '信息不足') IN ('信息不足', '需要人工判断')
    OR COALESCE(NULLIF(o.purpose, ''), '未知') = '未知'
    OR COALESCE(NULLIF(o.necessity, ''), '未知') = '未知'
    OR COALESCE(NULLIF(o.planning, ''), '未知') = '未知'
    OR COALESCE(NULLIF(o.recurrence, ''), '未知') = '未知'
  THEN 1 ELSE 0 END,
  h.updated_at = NOW(3)
WHERE o.interpretation_source = 'user_rule';
