ALTER TABLE rule_versions
  DROP CHECK chk_rule_versions_application_id_nonempty,
  DROP INDEX uk_rule_versions_application_number,
  DROP INDEX uk_rule_versions_application_idempotency,
  DROP INDEX idx_rule_versions_application_status_number,
  DROP COLUMN application_id,
  ADD UNIQUE KEY uk_rule_versions_number(tenant_id,rule_set_id,version_number),
  ADD UNIQUE KEY uk_rule_versions_idempotency(tenant_id,rule_set_id,idempotency_key),
  ADD KEY idx_rule_versions_set_status_number(tenant_id,rule_set_id,status,version_number);

ALTER TABLE rule_sets
  DROP CHECK chk_rule_sets_application_id_nonempty,
  DROP INDEX uk_rule_sets_tenant_application_code,
  DROP INDEX idx_rule_sets_tenant_application_status_code,
  DROP COLUMN application_id,
  ADD UNIQUE KEY uk_rule_sets_tenant_code(tenant_id,code),
  ADD KEY idx_rule_sets_tenant_status_code(tenant_id,status,code);
