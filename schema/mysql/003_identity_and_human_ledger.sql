USE shoreos;

ALTER TABLE shoreos_users
  MODIFY COLUMN password_hash VARCHAR(255) NOT NULL;

CREATE TABLE IF NOT EXISTS shoreos_user_identities (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  provider VARCHAR(32) NOT NULL,
  provider_subject VARCHAR(128) NOT NULL,
  provider_union_id VARCHAR(128) NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_shoreos_user_identities_provider_subject (provider, provider_subject),
  UNIQUE KEY uk_shoreos_user_identities_user_provider (user_id, provider),
  CONSTRAINT fk_shoreos_user_identities_user
    FOREIGN KEY (user_id) REFERENCES shoreos_users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS shoreos_identity_binding_codes (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  provider VARCHAR(32) NOT NULL,
  code_hash CHAR(64) NOT NULL,
  expires_at DATETIME(3) NOT NULL,
  consumed_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_shoreos_identity_binding_codes_hash (code_hash),
  KEY idx_shoreos_identity_binding_codes_user_expiry (user_id, expires_at),
  CONSTRAINT fk_shoreos_identity_binding_codes_user
    FOREIGN KEY (user_id) REFERENCES shoreos_users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE ledger_v2_human_transactions
  ADD COLUMN human_title VARCHAR(255) NULL AFTER primary_entry_id,
  ADD COLUMN merchant_name VARCHAR(255) NULL AFTER human_title,
  ADD COLUMN occurred_at VARCHAR(64) NULL AFTER transaction_type,
  ADD COLUMN source_platform VARCHAR(64) NULL AFTER occurred_at,
  ADD KEY idx_ledger_v2_human_transactions_user_time (user_id, occurred_at);

ALTER TABLE ledger_v2_entry_links
  ADD COLUMN candidate_reason VARCHAR(512) NULL AFTER evidence;
