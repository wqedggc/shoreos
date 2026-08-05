USE shoreos;

CREATE TABLE IF NOT EXISTS ledger_v2_budget_settings (
  user_id BIGINT UNSIGNED NOT NULL,
  flexible_budget_monthly_cents BIGINT NOT NULL DEFAULT 0,
  started_on DATE NOT NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (user_id),
  CONSTRAINT fk_ledger_v2_budget_settings_user
    FOREIGN KEY (user_id) REFERENCES shoreos_users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ledger_v2_fixed_expenses (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  name VARCHAR(128) NOT NULL,
  category_group VARCHAR(64) NULL,
  category_detail VARCHAR(64) NULL,
  monthly_amount_cents BIGINT NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'active',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  KEY idx_ledger_v2_fixed_expenses_user_status (user_id, status),
  CONSTRAINT fk_ledger_v2_fixed_expenses_user
    FOREIGN KEY (user_id) REFERENCES shoreos_users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ledger_v2_sinking_funds (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  name VARCHAR(128) NOT NULL,
  monthly_contribution_cents BIGINT NOT NULL,
  opening_balance_cents BIGINT NOT NULL DEFAULT 0,
  started_on DATE NOT NULL,
  accrual_ended_on DATE NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'active',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  KEY idx_ledger_v2_sinking_funds_user_status (user_id, status),
  CONSTRAINT fk_ledger_v2_sinking_funds_user
    FOREIGN KEY (user_id) REFERENCES shoreos_users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ledger_v2_classification_rules (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  match_field VARCHAR(32) NOT NULL,
  match_value VARCHAR(255) NOT NULL,
  category_group VARCHAR(64) NULL,
  category_detail VARCHAR(64) NULL,
  purpose VARCHAR(64) NULL,
  necessity VARCHAR(32) NULL,
  planning VARCHAR(32) NULL,
  recurrence VARCHAR(32) NULL,
  budget_treatment VARCHAR(32) NULL,
  sinking_fund_id BIGINT UNSIGNED NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  hit_count INT NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_ledger_v2_rules_user_match (user_id, match_field, match_value),
  KEY idx_ledger_v2_rules_user_enabled (user_id, enabled),
  CONSTRAINT fk_ledger_v2_rules_user
    FOREIGN KEY (user_id) REFERENCES shoreos_users (id) ON DELETE CASCADE,
  CONSTRAINT fk_ledger_v2_rules_sinking_fund
    FOREIGN KEY (sinking_fund_id) REFERENCES ledger_v2_sinking_funds (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE ledger_v2_transaction_overrides
  ADD COLUMN budget_treatment VARCHAR(32) NULL AFTER recurrence,
  ADD COLUMN sinking_fund_id BIGINT UNSIGNED NULL AFTER budget_treatment,
  ADD COLUMN confidence VARCHAR(16) NOT NULL DEFAULT 'unknown' AFTER sinking_fund_id,
  ADD COLUMN evidence_basis VARCHAR(32) NOT NULL DEFAULT 'insufficient' AFTER confidence,
  ADD COLUMN decision_scope VARCHAR(32) NOT NULL DEFAULT 'single_transaction' AFTER evidence_basis,
  ADD COLUMN rationale VARCHAR(1000) NULL AFTER decision_scope,
  ADD COLUMN interpretation_source VARCHAR(32) NOT NULL DEFAULT 'user_override' AFTER rationale,
  ADD COLUMN classification_rule_id BIGINT UNSIGNED NULL AFTER interpretation_source,
  ADD KEY idx_ledger_v2_transaction_overrides_user_budget (user_id, budget_treatment),
  ADD KEY idx_ledger_v2_transaction_overrides_sinking_fund (sinking_fund_id),
  ADD KEY idx_ledger_v2_transaction_overrides_rule (classification_rule_id),
  ADD CONSTRAINT fk_ledger_v2_transaction_overrides_sinking_fund
    FOREIGN KEY (sinking_fund_id) REFERENCES ledger_v2_sinking_funds (id) ON DELETE SET NULL,
  ADD CONSTRAINT fk_ledger_v2_transaction_overrides_rule
    FOREIGN KEY (classification_rule_id) REFERENCES ledger_v2_classification_rules (id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS ledger_v2_rule_applications (
  rule_id BIGINT UNSIGNED NOT NULL,
  human_transaction_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  applied_at DATETIME(3) NOT NULL,
  PRIMARY KEY (rule_id, human_transaction_id),
  KEY idx_ledger_v2_rule_applications_user_transaction (user_id, human_transaction_id),
  CONSTRAINT fk_ledger_v2_rule_applications_rule
    FOREIGN KEY (rule_id) REFERENCES ledger_v2_classification_rules (id) ON DELETE CASCADE,
  CONSTRAINT fk_ledger_v2_rule_applications_human
    FOREIGN KEY (human_transaction_id) REFERENCES ledger_v2_human_transactions (id) ON DELETE CASCADE,
  CONSTRAINT fk_ledger_v2_rule_applications_user
    FOREIGN KEY (user_id) REFERENCES shoreos_users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
