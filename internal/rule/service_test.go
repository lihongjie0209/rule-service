package rule

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	platformprincipal "github.com/lihongjie0209/microservice-platform-go/principal"
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
func (r *fakeRepository) GetRuleSet(_ context.Context, tenantID, _, _ string) (RuleSet, error) {
	if r.ruleSet.ID == "" || r.ruleSet.TenantID != tenantID {
		return RuleSet{}, ErrNotFound
	}
	return r.ruleSet, nil
}
func (*fakeRepository) ListRuleSets(context.Context, string, string, string, int, int) ([]RuleSet, int64, error) {
	return nil, 0, nil
}
func (r *fakeRepository) CreateRuleVersion(_ context.Context, _ sqlx.ExtContext, value RuleVersion) (RuleVersion, bool, error) {
	value.VersionNumber = 1
	r.ruleVersion = value
	return value, true, nil
}
func (r *fakeRepository) GetRuleVersion(_ context.Context, tenantID, ruleSetID, _, _ string, _ int64) (RuleVersion, error) {
	panic("unreachable")
}
func (*fakeRepository) ListRuleVersions(context.Context, string, string, string, int, int) ([]RuleVersion, int64, error) {
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

func (r serviceRepository) GetRuleVersion(_ context.Context, tenantID, ruleSetID, _ string, versionNumber int64) (RuleVersion, error) {
	if r.ruleVersion.ID == "" || r.ruleVersion.TenantID != tenantID || r.ruleVersion.RuleSetID != ruleSetID || (versionNumber != 0 && r.ruleVersion.VersionNumber != versionNumber) {
		return RuleVersion{}, ErrNotFound
	}
	return r.ruleVersion, nil
}

func TestServiceRuleLifecycleAndEvaluation(t *testing.T) {
	t.Parallel()

	repository := &fakeRepository{}
	service := &Service{repository: serviceRepository{repository}, transactor: fakeTransaction{}, now: func() time.Time { return time.Date(2026, 8, 31, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60)) }}
	ctx := platformprincipal.WithContext(context.Background(), platformprincipal.Principal{ID: "user-1", Type: platformprincipal.TypeUser, TenantID: "tenant-1"})

	set, err := service.CreateRuleSet(ctx, "tenant-1", "checkout.discount", "Checkout discount", "")
	if err != nil {
		t.Fatalf("create rule set: %v", err)
	}
	definition := `{"rules":[{"name":"vip","condition":"facts.vip == true","result":{"discount":20}}],"default_result":{"discount":0}}`
	version, created, err := service.CreateRuleVersion(ctx, "tenant-1", set.ID, definition, "version-key-1")
	if err != nil || !created {
		t.Fatalf("create rule version: created=%v err=%v", created, err)
	}
	set, version, err = service.PublishRuleVersion(ctx, "tenant-1", set.ID, version.ID, 1, 1)
	if err != nil {
		t.Fatalf("publish rule version: %v", err)
	}
	if len(repository.outboxEvents) != 1 || repository.outboxEvents[0].Subject != "platform.rule.rule-version.published.v1" {
		t.Fatalf("outbox events = %+v", repository.outboxEvents)
	}

	evaluation, evaluatedVersion, err := service.Evaluate(ctx, "tenant-1", set.ID, "", 0, `{"vip":true}`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !evaluation.Matched || evaluation.MatchedRule != "vip" || evaluation.ResultJSON != `{"discount":20}` || evaluatedVersion.VersionNumber != 1 {
		t.Fatalf("evaluation = %+v version=%+v", evaluation, evaluatedVersion)
	}
	batch, err := service.BatchEvaluate(ctx, []EvaluationInput{
		{TenantID: "tenant-1", RuleSetID: set.ID, FactsJSON: `{"vip":true}`},
		{TenantID: "tenant-2", RuleSetID: set.ID, FactsJSON: `{"vip":false}`},
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

	service := &Service{repository: serviceRepository{&fakeRepository{}}, transactor: fakeTransaction{}, now: time.Now}
	ctx := platformprincipal.WithContext(context.Background(), platformprincipal.Principal{ID: "user-1", Type: platformprincipal.TypeUser, TenantID: "tenant-1"})
	_, err := service.CreateRuleSet(ctx, "tenant-2", "checkout.discount", "Checkout", "")
	if err == nil {
		t.Fatal("cross-tenant create succeeded")
	}
	var target interface{ Unwrap() error }
	if !errors.As(err, &target) {
		t.Fatalf("error = %T %v, want application error", err, err)
	}
}
