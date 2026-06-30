CREATE DATABASE IF NOT EXISTS shoreos
  DEFAULT CHARACTER SET utf8mb4
  DEFAULT COLLATE utf8mb4_unicode_ci;

USE shoreos;

CREATE TABLE IF NOT EXISTS shoreos_users (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  username VARCHAR(64) NOT NULL,
  password_hash CHAR(64) NOT NULL,
  display_name VARCHAR(128) NOT NULL,
  avatar VARCHAR(32) NOT NULL DEFAULT '🌸',
  status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_shoreos_users_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS shoreos_sessions (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  token_hash CHAR(64) NOT NULL,
  expires_at DATETIME(3) NOT NULL,
  created_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_shoreos_sessions_token_hash (token_hash),
  KEY idx_shoreos_sessions_user_expires (user_id, expires_at),
  CONSTRAINT fk_shoreos_sessions_user
    FOREIGN KEY (user_id) REFERENCES shoreos_users (id)
    ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS fire_scenarios (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  profile_uid VARCHAR(64) NOT NULL,
  name VARCHAR(128) NOT NULL,
  avatar VARCHAR(32) NOT NULL DEFAULT '🌸',
  scenario_json LONGTEXT NOT NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_fire_scenarios_user_profile (user_id, profile_uid),
  KEY idx_fire_scenarios_user_updated (user_id, updated_at),
  CONSTRAINT fk_fire_scenarios_user
    FOREIGN KEY (user_id) REFERENCES shoreos_users (id)
    ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS fire_asset_snapshots (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  scenario_id BIGINT UNSIGNED NULL,
  as_of_date DATE NOT NULL,
  total_asset_cents BIGINT NOT NULL DEFAULT 0,
  total_liability_cents BIGINT NOT NULL DEFAULT 0,
  investable_net_worth_cents BIGINT NOT NULL DEFAULT 0,
  source_type VARCHAR(64) NOT NULL DEFAULT 'manual',
  source_snapshot LONGTEXT NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  KEY idx_fire_asset_snapshots_user_date (user_id, as_of_date),
  CONSTRAINT fk_fire_asset_snapshots_user
    FOREIGN KEY (user_id) REFERENCES shoreos_users (id)
    ON DELETE CASCADE,
  CONSTRAINT fk_fire_asset_snapshots_scenario
    FOREIGN KEY (scenario_id) REFERENCES fire_scenarios (id)
    ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS fire_asset_snapshot_items (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  snapshot_id BIGINT UNSIGNED NOT NULL,
  asset_class VARCHAR(64) NOT NULL,
  display_name VARCHAR(128) NOT NULL,
  currency CHAR(3) NOT NULL DEFAULT 'CNY',
  asset_value_cents BIGINT NOT NULL DEFAULT 0,
  liability_cents BIGINT NOT NULL DEFAULT 0,
  note VARCHAR(512) NULL,
  created_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  KEY idx_fire_asset_snapshot_items_snapshot (snapshot_id),
  CONSTRAINT fk_fire_asset_snapshot_items_snapshot
    FOREIGN KEY (snapshot_id) REFERENCES fire_asset_snapshots (id)
    ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS fire_projection_runs (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  scenario_id BIGINT UNSIGNED NULL,
  snapshot_id BIGINT UNSIGNED NULL,
  run_mode VARCHAR(64) NOT NULL DEFAULT 'monthly',
  input_snapshot LONGTEXT NOT NULL,
  created_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  KEY idx_fire_projection_runs_user_created (user_id, created_at),
  CONSTRAINT fk_fire_projection_runs_user
    FOREIGN KEY (user_id) REFERENCES shoreos_users (id)
    ON DELETE CASCADE,
  CONSTRAINT fk_fire_projection_runs_scenario
    FOREIGN KEY (scenario_id) REFERENCES fire_scenarios (id)
    ON DELETE SET NULL,
  CONSTRAINT fk_fire_projection_runs_snapshot
    FOREIGN KEY (snapshot_id) REFERENCES fire_asset_snapshots (id)
    ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS fire_projection_points (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  run_id BIGINT UNSIGNED NOT NULL,
  point_index INT NOT NULL,
  point_date DATE NOT NULL,
  projected_asset_cents BIGINT NOT NULL,
  projected_fire_number_cents BIGINT NOT NULL,
  coverage_ratio_bps INT NOT NULL,
  payload LONGTEXT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_fire_projection_points_run_index (run_id, point_index),
  CONSTRAINT fk_fire_projection_points_run
    FOREIGN KEY (run_id) REFERENCES fire_projection_runs (id)
    ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
