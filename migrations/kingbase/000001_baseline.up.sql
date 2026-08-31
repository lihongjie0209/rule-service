CREATE TABLE rule_sets (
 id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, code TEXT NOT NULL, name TEXT NOT NULL,
 description TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, published_version_number BIGINT NOT NULL DEFAULT 0,
 version BIGINT NOT NULL DEFAULT 1, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL,
 created_by TEXT NOT NULL, updated_by TEXT NOT NULL, UNIQUE(tenant_id,code)
);
CREATE INDEX idx_rule_sets_tenant_status_code ON rule_sets(tenant_id,status,code);
CREATE TABLE rule_versions (
 id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, rule_set_id TEXT NOT NULL REFERENCES rule_sets(id),
 version_number BIGINT NOT NULL, status TEXT NOT NULL, definition_json JSONB NOT NULL, checksum TEXT NOT NULL,
 idempotency_key TEXT NOT NULL, published_at TIMESTAMPTZ NULL,
 version BIGINT NOT NULL DEFAULT 1, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL,
 created_by TEXT NOT NULL, updated_by TEXT NOT NULL,
 UNIQUE(tenant_id,rule_set_id,version_number), UNIQUE(tenant_id,rule_set_id,idempotency_key)
);
CREATE INDEX idx_rule_versions_set_status_number ON rule_versions(tenant_id,rule_set_id,status,version_number DESC);
CREATE TABLE rule_outbox_events (
 id TEXT PRIMARY KEY, subject TEXT NOT NULL, envelope BYTEA NOT NULL, attempts INTEGER NOT NULL DEFAULT 0,
 available_at TIMESTAMPTZ NOT NULL, published_at TIMESTAMPTZ NULL, last_error TEXT NOT NULL DEFAULT '',
 version BIGINT NOT NULL DEFAULT 1, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL,
 created_by TEXT NOT NULL, updated_by TEXT NOT NULL
);
CREATE INDEX idx_rule_outbox_pending ON rule_outbox_events(published_at,available_at);
