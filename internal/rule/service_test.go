package rule

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/lihongjie0209/microservice-platform-go/appaccess"
	platformprincipal "github.com/lihongjie0209/microservice-platform-go/principal"
	commonv1 "github.com/lihongjie0209/platform-protos/gen/go/platform/common/v1"
	"github.com/lihongjie0209/rule-service/internal/apperror"
	"google.golang.org/protobuf/proto"
)

type fakeTransaction struct{}

func (fakeTransaction) Within(_ context.Context, _ *sql.TxOptions, fn func(*sqlx.Tx) error) error {
	return fn(nil)
}

type fakeRepository struct {
	ruleSet      RuleSet
	ruleVersion  RuleVersion
	outboxEvents []OutboxEvent
}

func (r *fakeRepository) CreateRuleSet(_ context.Context, _ sqlx.ExtContext, value RuleSet) error {
	r.ruleSet = value
	return nil
}
func (r *fakeRepository) UpdateRuleSet(_ context.Context, _ sqlx.ExtContext, value RuleSet, _ int64) error {
	r.ruleSet = value
	return nil
}
func (r *fakeRepository) GetRuleSet(_ context.Context, tenantID, applicationID, _, _ string) (RuleSet, error) {
	if r.ruleSet.ID == "" || r.ruleSet.TenantID != tenantID || r.ruleSet.ApplicationID != applicationID {
		return RuleSet{}, ErrNotFound
	}
	return r.ruleSet, nil
}
func (*fakeRepository) ListRuleSets(context.Context, string, string, string, string, int, int) ([]RuleSet, int64, error) {
	return nil, 0, nil
}
func (r *fakeRepository) CreateRuleVersion(_ context.Context, _ sqlx.ExtContext, value RuleVersion) (RuleVersion, bool, error) {
	value.VersionNumber = 1
	r.ruleVersion = value
	return value, true, nil
}
func (r *fakeRepository) GetRuleVersion(_ context.Context, tenantID, applicationID, ruleSetID, _, _ string, _ int64) (RuleVersion, error) {
	panic("unreachable")
}
func (*fakeRepository) ListRuleVersions(context.Context, string, string, string, string, int, int) ([]RuleVersion, int64, error) {
	return nil, 0, nil
}
func (r *fakeRepository) PublishRuleVersion(_ context.Context, _ sqlx.ExtContext, set RuleSet, version RuleVersion, _, _ int64) error {
	r.ruleSet, r.ruleVersion = set, version
	return nil
}
func (r *fakeRepository) AddOutbox(_ context.Context, _ sqlx.ExtContext, event OutboxEvent) error {
	r.outboxEvents = append(r.outboxEvents, event)
	return nil
}

// getRuleVersionRepository fixes the deliberately explicit selector signature
// while keeping tests independent from a database implementation.
type serviceRepository struct{ *fakeRepository }

func (r serviceRepository) GetRuleVersion(_ context.Context, tenantID, applicationID, ruleSetID, _ string, versionNumber int64) (RuleVersion, error) {
	if r.ruleVersion.ID == "" || r.ruleVersion.TenantID != tenantID || r.ruleVersion.ApplicationID != applicationID || r.ruleVersion.RuleSetID != ruleSetID || (versionNumber != 0 && r.ruleVersion.VersionNumber != versionNumber) {
		return RuleVersion{}, ErrNotFound
	}
	return r.ruleVersion, nil
}

type fakeApplicationVerifier struct{ err error }

func (v fakeApplicationVerifier) Verify(context.Context, string, string) error { return v.err }

func TestServiceRuleLifecycleAndEvaluation(t *testing.T) {
	t.Parallel()

	repository := &fakeRepository{}
	service := &Service{repository: serviceRepository{repository}, transactor: fakeTransaction{}, applications: fakeApplicationVerifier{}, now: func() time.Time { return time.Date(2026, 8, 31, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60)) }}
	ctx := platformprincipal.WithContext(context.Background(), platformprincipal.Principal{ID: "user-1", Type: platformprincipal.TypeUser, TenantID: "tenant-1"})

	set, err := service.CreateRuleSet(ctx, "tenant-1", "app-1", "checkout.discount", "Checkout discount", "")
	if err != nil {
		t.Fatalf("create rule set: %v", err)
	}
	definition := `{"rules":[{"name":"vip","condition":"facts.vip == true","result":{"discount":20}}],"default_result":{"discount":0}}`
	version, created, err := service.CreateRuleVersion(ctx, "tenant-1", "app-1", set.ID, definition, "version-key-1")
	if err != nil || !created {
		t.Fatalf("create rule version: created=%v err=%v", created, err)
	}
	currentVersion, err := service.GetRuleVersion(ctx, "tenant-1", "app-1", set.ID, version.ID)
	if err != nil || currentVersion.ID != version.ID || currentVersion.Version != version.Version {
		t.Fatalf("get rule version: value=%+v err=%v", currentVersion, err)
	}
	set, version, err = service.PublishRuleVersion(ctx, "tenant-1", "app-1", set.ID, version.ID, 1, 1)
	if err != nil {
		t.Fatalf("publish rule version: %v", err)
	}
	if len(repository.outboxEvents) != 1 || repository.outboxEvents[0].Subject != "platform.rule.rule-version.published.v1" {
		t.Fatalf("outbox events = %+v", repository.outboxEvents)
	}
	envelope := &commonv1.EventEnvelope{}
	if err := proto.Unmarshal(repository.outboxEvents[0].Envelope, envelope); err != nil || envelope.GetApplicationId() != "app-1" {
		t.Fatalf("published event application scope = %q, err = %v", envelope.GetApplicationId(), err)
	}

	evaluation, evaluatedVersion, err := service.Evaluate(ctx, "tenant-1", "app-1", set.ID, "", 0, `{"vip":true}`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !evaluation.Matched || evaluation.MatchedRule != "vip" || evaluation.ResultJSON != `{"discount":20}` || evaluatedVersion.VersionNumber != 1 {
		t.Fatalf("evaluation = %+v version=%+v", evaluation, evaluatedVersion)
	}
	batch, err := service.BatchEvaluate(ctx, []EvaluationInput{
		{TenantID: "tenant-1", ApplicationID: "app-1", RuleSetID: set.ID, FactsJSON: `{"vip":true}`},
		{TenantID: "tenant-2", ApplicationID: "app-1", RuleSetID: set.ID, FactsJSON: `{"vip":false}`},
	})
	if err != nil || len(batch) != 2 || batch[0].Err != nil || !batch[0].Evaluation.Matched || batch[1].Err == nil {
		t.Fatalf("batch = %+v, err = %v", batch, err)
	}
}

func TestBatchEvaluateRejectsInvalidSize(t *testing.T) {
	t.Parallel()
	service := &Service{}
	if _, err := service.BatchEvaluate(t.Context(), nil); err == nil {
		t.Fatal("empty batch accepted")
	}
	if _, err := service.BatchEvaluate(t.Context(), make([]EvaluationInput, 101)); err == nil {
		t.Fatal("oversized batch accepted")
	}
}

func TestServiceRejectsCrossTenantAccess(t *testing.T) {
	t.Parallel()

	service := &Service{repository: serviceRepository{&fakeRepository{}}, transactor: fakeTransaction{}, applications: fakeApplicationVerifier{}, now: time.Now}
	ctx := platformprincipal.WithContext(context.Background(), platformprincipal.Principal{ID: "user-1", Type: platformprincipal.TypeUser, TenantID: "tenant-1"})
	_, err := service.CreateRuleSet(ctx, "tenant-2", "app-1", "checkout.discount", "Checkout", "")
	if err == nil {
		t.Fatal("cross-tenant create succeeded")
	}
	var target interface{ Unwrap() error }
	if !errors.As(err, &target) {
		t.Fatalf("error = %T %v, want application error", err, err)
	}
}

func TestServiceRejectsUnavailableApplicationVerifier(t *testing.T) {
	t.Parallel()
	service := &Service{repository: serviceRepository{&fakeRepository{}}, transactor: fakeTransaction{}, applications: fakeApplicationVerifier{err: appaccess.ErrUnavailable}, now: time.Now}
	ctx := platformprincipal.WithContext(t.Context(), platformprincipal.Principal{ID: "user-1", Type: platformprincipal.TypeUser, TenantID: "tenant-1"})
	_, err := service.CreateRuleSet(ctx, "tenant-1", "app-1", "checkout.discount", "Checkout", "")
	var appErr interface{ Unwrap() error }
	if !errors.As(err, &appErr) || !errors.Is(err, appaccess.ErrUnavailable) {
		t.Fatalf("CreateRuleSet() error = %v", err)
	}
}

func TestServiceRejectsApplicationWithoutTenantGrant(t *testing.T) {
	t.Parallel()
	service := &Service{repository: serviceRepository{&fakeRepository{}}, transactor: fakeTransaction{}, applications: fakeApplicationVerifier{err: appaccess.ErrNotGranted}, now: time.Now}
	ctx := platformprincipal.WithContext(t.Context(), platformprincipal.Principal{ID: "user-1", Type: platformprincipal.TypeUser, TenantID: "tenant-1"})
	_, err := service.CreateRuleSet(ctx, "tenant-1", "app-1", "checkout.discount", "Checkout", "")
	var appErr *apperror.Error
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeForbidden {
		t.Fatalf("CreateRuleSet() error = %#v", err)
	}
}

func TestSQLRepositoryGetRuleSetScopesByTenantAndApplication(t *testing.T) {
	t.Parallel()
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	db := sqlx.NewDb(database, "sqlmock")
	repository := &SQLRepository{db: db}
	query := "SELECT " + ruleSetColumns + " FROM rule_sets WHERE tenant_id=? AND application_id=? AND id=?"
	rows := sqlmock.NewRows(strings.Split(ruleSetColumns, ",")).AddRow("set-1", "tenant-1", "app-1", "checkout.discount", "Checkout", "", "draft", int64(0), int64(1), time.Now(), time.Now(), "actor-1", "actor-1")
	mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs("tenant-1", "app-1", "set-1").WillReturnRows(rows)
	value, err := repository.GetRuleSet(t.Context(), "tenant-1", "app-1", "set-1", "")
	if err != nil || value.ApplicationID != "app-1" {
		t.Fatalf("GetRuleSet() = %+v, %v", value, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
