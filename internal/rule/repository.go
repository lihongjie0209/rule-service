package rule

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/jmoiron/sqlx"
)

var (
	ErrNotFound     = errors.New("rule resource not found")
	ErrStaleVersion = errors.New("stale rule resource version")
	ErrConflict     = errors.New("rule resource conflict")
)

type Repository interface {
	CreateRuleSet(context.Context, sqlx.ExtContext, RuleSet) error
	UpdateRuleSet(context.Context, sqlx.ExtContext, RuleSet, int64) error
	GetRuleSet(context.Context, string, string, string, string) (RuleSet, error)
	ListRuleSets(context.Context, string, string, string, string, int, int) ([]RuleSet, int64, error)
	CreateRuleVersion(context.Context, sqlx.ExtContext, RuleVersion, int64) (RuleVersion, bool, error)
	GetRuleVersion(context.Context, string, string, string, string, int64) (RuleVersion, error)
	ListRuleVersions(context.Context, string, string, string, string, int, int) ([]RuleVersion, int64, error)
	PublishRuleVersion(context.Context, sqlx.ExtContext, RuleSet, RuleVersion, int64, int64) error
	AddOutbox(context.Context, sqlx.ExtContext, OutboxEvent) error
}

type SQLRepository struct{ db *sqlx.DB }

func NewRepository(db *sqlx.DB) Repository { return &SQLRepository{db: db} }

const ruleSetColumns = "id,tenant_id,application_id,code,name,description,status,published_version_number,version,created_at,updated_at,created_by,updated_by"
const ruleVersionColumns = "id,tenant_id,application_id,rule_set_id,version_number,status,definition_json,checksum,idempotency_key,published_at,version,created_at,updated_at,created_by,updated_by"

func (r *SQLRepository) CreateRuleSet(ctx context.Context, e sqlx.ExtContext, value RuleSet) error {
	_, err := e.ExecContext(ctx, r.db.Rebind("INSERT INTO rule_sets ("+ruleSetColumns+") VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)"), value.ID, value.TenantID, value.ApplicationID, value.Code, value.Name, value.Description, value.Status, value.PublishedVersionNumber, value.Version, value.CreatedAt, value.UpdatedAt, value.CreatedBy, value.UpdatedBy)
	return err
}

func (r *SQLRepository) UpdateRuleSet(ctx context.Context, e sqlx.ExtContext, value RuleSet, expected int64) error {
	result, err := e.ExecContext(ctx, r.db.Rebind("UPDATE rule_sets SET name=?,description=?,status=?,version=version+1,updated_at=?,updated_by=? WHERE tenant_id=? AND application_id=? AND id=? AND version=?"), value.Name, value.Description, value.Status, value.UpdatedAt, value.UpdatedBy, value.TenantID, value.ApplicationID, value.ID, expected)
	return optimistic(result, err)
}

func (r *SQLRepository) GetRuleSet(ctx context.Context, tenantID, applicationID, id, code string) (RuleSet, error) {
	query, argument := "SELECT "+ruleSetColumns+" FROM rule_sets WHERE tenant_id=? AND application_id=? AND id=?", id
	if strings.TrimSpace(id) == "" {
		query, argument = "SELECT "+ruleSetColumns+" FROM rule_sets WHERE tenant_id=? AND application_id=? AND code=?", code
	}
	var value RuleSet
	err := r.db.GetContext(ctx, &value, r.db.Rebind(query), tenantID, applicationID, argument)
	return value, notFound(err)
}

func (r *SQLRepository) ListRuleSets(ctx context.Context, tenantID, applicationID, status, keyword string, limit, offset int) ([]RuleSet, int64, error) {
	where, args := "tenant_id=? AND application_id=?", []any{tenantID, applicationID}
	if status != "" {
		where += " AND status=?"
		args = append(args, status)
	}
	if keyword != "" {
		where += " AND (LOWER(code) LIKE ? OR LOWER(name) LIKE ?)"
		like := "%" + strings.ToLower(keyword) + "%"
		args = append(args, like, like)
	}
	var total int64
	if err := r.db.GetContext(ctx, &total, r.db.Rebind("SELECT COUNT(*) FROM rule_sets WHERE "+where), args...); err != nil {
		return nil, 0, err
	}
	items := []RuleSet{}
	pageArgs := append(append([]any{}, args...), limit, offset)
	err := r.db.SelectContext(ctx, &items, r.db.Rebind("SELECT "+ruleSetColumns+" FROM rule_sets WHERE "+where+" ORDER BY code LIMIT ? OFFSET ?"), pageArgs...)
	return items, total, err
}

func (r *SQLRepository) CreateRuleVersion(ctx context.Context, e sqlx.ExtContext, value RuleVersion, expectedRuleSetVersion int64) (RuleVersion, bool, error) {
	// Serialize version allocation and idempotency lookup on the owning rule set.
	// This prevents two concurrent requests from both missing the key and then
	// racing on either the version number or unique idempotency constraint.
	var ruleSet struct {
		Version int64  `db:"version"`
		Status  string `db:"status"`
	}
	if err := sqlx.GetContext(ctx, e, &ruleSet, r.db.Rebind("SELECT version,status FROM rule_sets WHERE tenant_id=? AND application_id=? AND id=? FOR UPDATE"), value.TenantID, value.ApplicationID, value.RuleSetID); err != nil {
		return RuleVersion{}, false, notFound(err)
	}
	var existing RuleVersion
	err := sqlx.GetContext(ctx, e, &existing, r.db.Rebind("SELECT "+ruleVersionColumns+" FROM rule_versions WHERE tenant_id=? AND application_id=? AND rule_set_id=? AND idempotency_key=?"), value.TenantID, value.ApplicationID, value.RuleSetID, value.IdempotencyKey)
	if err == nil {
		if existing.Checksum != value.Checksum {
			return RuleVersion{}, false, ErrConflict
		}
		return existing, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return RuleVersion{}, false, err
	}
	if ruleSet.Version != expectedRuleSetVersion || ruleSet.Status == "disabled" {
		return RuleVersion{}, false, ErrStaleVersion
	}
	if err := sqlx.GetContext(ctx, e, &value.VersionNumber, r.db.Rebind("SELECT COALESCE(MAX(version_number),0)+1 FROM rule_versions WHERE tenant_id=? AND application_id=? AND rule_set_id=?"), value.TenantID, value.ApplicationID, value.RuleSetID); err != nil {
		return RuleVersion{}, false, err
	}
	_, err = e.ExecContext(ctx, r.db.Rebind("INSERT INTO rule_versions ("+ruleVersionColumns+") VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)"), value.ID, value.TenantID, value.ApplicationID, value.RuleSetID, value.VersionNumber, value.Status, value.DefinitionJSON, value.Checksum, value.IdempotencyKey, value.PublishedAt, value.Version, value.CreatedAt, value.UpdatedAt, value.CreatedBy, value.UpdatedBy)
	return value, true, err
}

func (r *SQLRepository) GetRuleVersion(ctx context.Context, tenantID, applicationID, ruleSetID, id string, versionNumber int64) (RuleVersion, error) {
	query, argument := "SELECT "+ruleVersionColumns+" FROM rule_versions WHERE tenant_id=? AND application_id=? AND rule_set_id=? AND id=?", any(id)
	if strings.TrimSpace(id) == "" {
		query, argument = "SELECT "+ruleVersionColumns+" FROM rule_versions WHERE tenant_id=? AND application_id=? AND rule_set_id=? AND version_number=?", versionNumber
	}
	var value RuleVersion
	err := r.db.GetContext(ctx, &value, r.db.Rebind(query), tenantID, applicationID, ruleSetID, argument)
	return value, notFound(err)
}

func (r *SQLRepository) ListRuleVersions(ctx context.Context, tenantID, applicationID, ruleSetID, status string, limit, offset int) ([]RuleVersion, int64, error) {
	where, args := "tenant_id=? AND application_id=? AND rule_set_id=?", []any{tenantID, applicationID, ruleSetID}
	if status != "" {
		where += " AND status=?"
		args = append(args, status)
	}
	var total int64
	if err := r.db.GetContext(ctx, &total, r.db.Rebind("SELECT COUNT(*) FROM rule_versions WHERE "+where), args...); err != nil {
		return nil, 0, err
	}
	items := []RuleVersion{}
	pageArgs := append(append([]any{}, args...), limit, offset)
	err := r.db.SelectContext(ctx, &items, r.db.Rebind("SELECT "+ruleVersionColumns+" FROM rule_versions WHERE "+where+" ORDER BY version_number DESC LIMIT ? OFFSET ?"), pageArgs...)
	return items, total, err
}

func (r *SQLRepository) PublishRuleVersion(ctx context.Context, e sqlx.ExtContext, ruleSet RuleSet, version RuleVersion, expectedRuleSetVersion, expectedVersion int64) error {
	result, err := e.ExecContext(ctx, r.db.Rebind("UPDATE rule_versions SET status='published',published_at=?,version=version+1,updated_at=?,updated_by=? WHERE tenant_id=? AND application_id=? AND rule_set_id=? AND id=? AND status='draft' AND version=?"), version.PublishedAt, version.UpdatedAt, version.UpdatedBy, version.TenantID, version.ApplicationID, version.RuleSetID, version.ID, expectedVersion)
	if err := optimistic(result, err); err != nil {
		return err
	}
	result, err = e.ExecContext(ctx, r.db.Rebind("UPDATE rule_sets SET status='active',published_version_number=?,version=version+1,updated_at=?,updated_by=? WHERE tenant_id=? AND application_id=? AND id=? AND version=?"), version.VersionNumber, ruleSet.UpdatedAt, ruleSet.UpdatedBy, ruleSet.TenantID, ruleSet.ApplicationID, ruleSet.ID, expectedRuleSetVersion)
	return optimistic(result, err)
}

func (r *SQLRepository) AddOutbox(ctx context.Context, e sqlx.ExtContext, value OutboxEvent) error {
	_, err := e.ExecContext(ctx, r.db.Rebind("INSERT INTO rule_outbox_events (id,subject,envelope,attempts,available_at,published_at,last_error,version,created_at,updated_at,created_by,updated_by) VALUES (?,?,?,0,?,NULL,'',1,?,?,?,?)"), value.ID, value.Subject, value.Envelope, value.AvailableAt, value.CreatedAt, value.UpdatedAt, value.CreatedBy, value.UpdatedBy)
	return err
}

func optimistic(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err == nil && rows == 0 {
		return ErrStaleVersion
	}
	return err
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
