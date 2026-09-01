DROP INDEX idx_rule_versions_tenant_application_set_status_number;
ALTER TABLE rule_versions DROP CONSTRAINT rule_versions_tenant_application_set_idempotency_key;
ALTER TABLE rule_versions DROP CONSTRAINT rule_versions_tenant_application_set_number_key;
ALTER TABLE rule_versions ADD CONSTRAINT rule_versions_tenant_id_rule_set_id_version_number_key UNIQUE (tenant_id, rule_set_id, version_number);
ALTER TABLE rule_versions ADD CONSTRAINT rule_versions_tenant_id_rule_set_id_idempotency_key_key UNIQUE (tenant_id, rule_set_id, idempotency_key);

DROP INDEX idx_rule_sets_tenant_application_status_code;
ALTER TABLE rule_sets DROP CONSTRAINT rule_sets_tenant_application_code_key;
ALTER TABLE rule_sets ADD CONSTRAINT rule_sets_tenant_id_code_key UNIQUE (tenant_id, code);

ALTER TABLE rule_versions DROP COLUMN application_id;
ALTER TABLE rule_sets DROP COLUMN application_id;
CREATE INDEX idx_rule_sets_tenant_status_code ON rule_sets(tenant_id, status, code);
CREATE INDEX idx_rule_versions_set_status_number ON rule_versions(tenant_id, rule_set_id, status, version_number DESC);
