ALTER TABLE rule_sets
  ADD COLUMN application_id VARCHAR(36) NOT NULL AFTER tenant_id,
  DROP INDEX uk_rule_sets_tenant_code,
  DROP INDEX idx_rule_sets_tenant_status_code,
  ADD CONSTRAINT chk_rule_sets_application_id_nonempty CHECK (application_id <> ''),
  ADD UNIQUE KEY uk_rule_sets_tenant_application_code(tenant_id,application_id,code),
  ADD KEY idx_rule_sets_tenant_application_status_code(tenant_id,application_id,status,code);

ALTER TABLE rule_versions
  ADD COLUMN application_id VARCHAR(36) NOT NULL AFTER tenant_id,
  DROP INDEX uk_rule_versions_number,
  DROP INDEX uk_rule_versions_idempotency,
  DROP INDEX idx_rule_versions_set_status_number,
  ADD CONSTRAINT chk_rule_versions_application_id_nonempty CHECK (application_id <> ''),
  ADD UNIQUE KEY uk_rule_versions_application_number(tenant_id,application_id,rule_set_id,version_number),
  ADD UNIQUE KEY uk_rule_versions_application_idempotency(tenant_id,application_id,rule_set_id,idempotency_key),
  ADD KEY idx_rule_versions_application_status_number(tenant_id,application_id,rule_set_id,status,version_number);
