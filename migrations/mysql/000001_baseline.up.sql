CREATE TABLE rule_sets (
 id VARCHAR(191) PRIMARY KEY, tenant_id VARCHAR(191) NOT NULL, code VARCHAR(191) NOT NULL, name TEXT NOT NULL,
 description TEXT NOT NULL, status VARCHAR(32) NOT NULL, published_version_number BIGINT NOT NULL DEFAULT 0,
 version BIGINT NOT NULL DEFAULT 1, created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
 created_by TEXT NOT NULL, updated_by TEXT NOT NULL,
 UNIQUE KEY uk_rule_sets_tenant_code(tenant_id,code), KEY idx_rule_sets_tenant_status_code(tenant_id,status,code)
);
CREATE TABLE rule_versions (
 id VARCHAR(191) PRIMARY KEY, tenant_id VARCHAR(191) NOT NULL, rule_set_id VARCHAR(191) NOT NULL,
 version_number BIGINT NOT NULL, status VARCHAR(32) NOT NULL, definition_json JSON NOT NULL,
 checksum VARCHAR(64) NOT NULL, idempotency_key VARCHAR(191) NOT NULL, published_at DATETIME(6) NULL,
 version BIGINT NOT NULL DEFAULT 1, created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
 created_by TEXT NOT NULL, updated_by TEXT NOT NULL,
 UNIQUE KEY uk_rule_versions_number(tenant_id,rule_set_id,version_number),
 UNIQUE KEY uk_rule_versions_idempotency(tenant_id,rule_set_id,idempotency_key),
 KEY idx_rule_versions_set_status_number(tenant_id,rule_set_id,status,version_number)
);
CREATE TABLE rule_outbox_events (
 id VARCHAR(191) PRIMARY KEY, subject VARCHAR(255) NOT NULL, envelope LONGBLOB NOT NULL,
 attempts INT NOT NULL DEFAULT 0, available_at DATETIME(6) NOT NULL, published_at DATETIME(6) NULL,
 last_error TEXT NOT NULL, version BIGINT NOT NULL DEFAULT 1, created_at DATETIME(6) NOT NULL,
 updated_at DATETIME(6) NOT NULL, created_by TEXT NOT NULL, updated_by TEXT NOT NULL,
 KEY idx_rule_outbox_pending(published_at,available_at)
);
