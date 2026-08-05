USE shoreos;

CREATE TABLE IF NOT EXISTS ledger_v2_import_batches (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  source_type VARCHAR(64) NOT NULL,
  parser_version VARCHAR(32) NOT NULL,
  original_filename VARCHAR(255) NOT NULL,
  source_file_hash CHAR(64) NOT NULL,
  storage_key VARCHAR(512) NOT NULL,
  status VARCHAR(32) NOT NULL,
  start_at VARCHAR(64) NULL,
  end_at VARCHAR(64) NULL,
  parsed_transaction_count INT NOT NULL DEFAULT 0,
  imported_transaction_count INT NOT NULL DEFAULT 0,
  duplicate_transaction_count INT NOT NULL DEFAULT 0,
  parse_error VARCHAR(512) NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_ledger_v2_import_batches_user_file (user_id, source_file_hash),
  KEY idx_ledger_v2_import_batches_user_created (user_id, created_at),
  KEY idx_ledger_v2_import_batches_user_status (user_id, status),
  CONSTRAINT fk_ledger_v2_import_batches_user
    FOREIGN KEY (user_id) REFERENCES shoreos_users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ledger_v2_source_accounts (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  platform VARCHAR(64) NOT NULL,
  account_key CHAR(64) NOT NULL,
  account_label VARCHAR(128) NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_ledger_v2_source_accounts_user_platform_key (user_id, platform, account_key),
  KEY idx_ledger_v2_source_accounts_user_platform (user_id, platform),
  CONSTRAINT fk_ledger_v2_source_accounts_user
    FOREIGN KEY (user_id) REFERENCES shoreos_users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ledger_v2_source_transactions (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  source_account_id BIGINT UNSIGNED NOT NULL,
  import_batch_id BIGINT UNSIGNED NOT NULL,
  source_type VARCHAR(64) NOT NULL,
  occurred_at VARCHAR(64) NOT NULL,
  source_category VARCHAR(128) NULL,
  counterparty_name VARCHAR(255) NULL,
  counterparty_account VARCHAR(255) NULL,
  product_description VARCHAR(512) NULL,
  direction VARCHAR(16) NOT NULL,
  amount_cents BIGINT NOT NULL,
  payment_method VARCHAR(128) NULL,
  transaction_status VARCHAR(128) NULL,
  platform_order_no VARCHAR(128) NULL,
  merchant_order_no VARCHAR(128) NULL,
  remark VARCHAR(512) NULL,
  raw_snapshot JSON NOT NULL,
  raw_row_hash CHAR(64) NOT NULL,
  transaction_fingerprint CHAR(64) NOT NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_ledger_v2_source_transactions_user_fingerprint (user_id, transaction_fingerprint),
  KEY idx_ledger_v2_source_transactions_user_time (user_id, occurred_at),
  KEY idx_ledger_v2_source_transactions_batch (import_batch_id),
  KEY idx_ledger_v2_source_transactions_orders (user_id, platform_order_no, merchant_order_no),
  CONSTRAINT fk_ledger_v2_source_transactions_user
    FOREIGN KEY (user_id) REFERENCES shoreos_users (id) ON DELETE CASCADE,
  CONSTRAINT fk_ledger_v2_source_transactions_account
    FOREIGN KEY (source_account_id) REFERENCES ledger_v2_source_accounts (id) ON DELETE CASCADE,
  CONSTRAINT fk_ledger_v2_source_transactions_batch
    FOREIGN KEY (import_batch_id) REFERENCES ledger_v2_import_batches (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ledger_v2_normalized_entries (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  source_transaction_id BIGINT UNSIGNED NOT NULL,
  entry_kind VARCHAR(32) NOT NULL,
  occurred_at VARCHAR(64) NOT NULL,
  amount_cents BIGINT NOT NULL,
  status VARCHAR(64) NOT NULL,
  evidence_level VARCHAR(32) NOT NULL DEFAULT 'source',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_ledger_v2_normalized_entries_source (user_id, source_transaction_id),
  KEY idx_ledger_v2_normalized_entries_user_time (user_id, occurred_at),
  CONSTRAINT fk_ledger_v2_normalized_entries_user
    FOREIGN KEY (user_id) REFERENCES shoreos_users (id) ON DELETE CASCADE,
  CONSTRAINT fk_ledger_v2_normalized_entries_source
    FOREIGN KEY (source_transaction_id) REFERENCES ledger_v2_source_transactions (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ledger_v2_entry_links (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  left_entry_id BIGINT UNSIGNED NOT NULL,
  right_entry_id BIGINT UNSIGNED NOT NULL,
  link_type VARCHAR(32) NOT NULL,
  match_status VARCHAR(32) NOT NULL,
  match_method VARCHAR(64) NOT NULL,
  confidence_bps INT NOT NULL DEFAULT 0,
  evidence JSON NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_ledger_v2_entry_links_pair_type (user_id, left_entry_id, right_entry_id, link_type),
  KEY idx_ledger_v2_entry_links_right (user_id, right_entry_id),
  CONSTRAINT fk_ledger_v2_entry_links_user
    FOREIGN KEY (user_id) REFERENCES shoreos_users (id) ON DELETE CASCADE,
  CONSTRAINT fk_ledger_v2_entry_links_left
    FOREIGN KEY (left_entry_id) REFERENCES ledger_v2_normalized_entries (id) ON DELETE CASCADE,
  CONSTRAINT fk_ledger_v2_entry_links_right
    FOREIGN KEY (right_entry_id) REFERENCES ledger_v2_normalized_entries (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ledger_v2_human_transactions (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  primary_entry_id BIGINT UNSIGNED NOT NULL,
  transaction_type VARCHAR(32) NOT NULL,
  original_amount_cents BIGINT NOT NULL,
  refunded_amount_cents BIGINT NOT NULL DEFAULT 0,
  actual_amount_cents BIGINT NOT NULL,
  needs_review TINYINT(1) NOT NULL DEFAULT 1,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_ledger_v2_human_transactions_primary (user_id, primary_entry_id),
  KEY idx_ledger_v2_human_transactions_user_review (user_id, needs_review),
  CONSTRAINT fk_ledger_v2_human_transactions_user
    FOREIGN KEY (user_id) REFERENCES shoreos_users (id) ON DELETE CASCADE,
  CONSTRAINT fk_ledger_v2_human_transactions_primary
    FOREIGN KEY (primary_entry_id) REFERENCES ledger_v2_normalized_entries (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ledger_v2_human_transaction_sources (
  human_transaction_id BIGINT UNSIGNED NOT NULL,
  normalized_entry_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  source_role VARCHAR(32) NOT NULL,
  created_at DATETIME(3) NOT NULL,
  PRIMARY KEY (human_transaction_id, normalized_entry_id),
  KEY idx_ledger_v2_human_transaction_sources_entry (user_id, normalized_entry_id),
  CONSTRAINT fk_ledger_v2_human_transaction_sources_human
    FOREIGN KEY (human_transaction_id) REFERENCES ledger_v2_human_transactions (id) ON DELETE CASCADE,
  CONSTRAINT fk_ledger_v2_human_transaction_sources_entry
    FOREIGN KEY (normalized_entry_id) REFERENCES ledger_v2_normalized_entries (id) ON DELETE CASCADE,
  CONSTRAINT fk_ledger_v2_human_transaction_sources_user
    FOREIGN KEY (user_id) REFERENCES shoreos_users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ledger_v2_transaction_overrides (
  human_transaction_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  human_title VARCHAR(255) NULL,
  user_note VARCHAR(1000) NULL,
  category_group VARCHAR(64) NULL,
  category_detail VARCHAR(64) NULL,
  purpose VARCHAR(64) NULL,
  necessity VARCHAR(32) NULL,
  planning VARCHAR(32) NULL,
  recurrence VARCHAR(32) NULL,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (human_transaction_id),
  KEY idx_ledger_v2_transaction_overrides_user_category (user_id, category_group, category_detail),
  CONSTRAINT fk_ledger_v2_transaction_overrides_human
    FOREIGN KEY (human_transaction_id) REFERENCES ledger_v2_human_transactions (id) ON DELETE CASCADE,
  CONSTRAINT fk_ledger_v2_transaction_overrides_user
    FOREIGN KEY (user_id) REFERENCES shoreos_users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
