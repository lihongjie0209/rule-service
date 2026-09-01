package rule

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lihongjie0209/microservice-platform-go/appaccess"
	platformevents "github.com/lihongjie0209/microservice-platform-go/eventbus"
	platformprincipal "github.com/lihongjie0209/microservice-platform-go/principal"
	rulev1 "github.com/lihongjie0209/platform-protos/gen/go/platform/rule/v1"
	"github.com/lihongjie0209/rule-service/internal/apperror"
	"github.com/lihongjie0209/rule-service/internal/database"
	"google.golang.org/protobuf/proto"
)

var ruleCodePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{1,127}$`)

type transactionRunner interface {
	Within(context.Context, *sql.TxOptions, func(*sqlx.Tx) error) error
}

type Service struct {
	repository   Repository
	transactor   transactionRunner
	applications appaccess.Verifier
	now          func() time.Time
	compiled     sync.Map
}

func NewService(repository Repository, transactor *database.Transactor, applications appaccess.Verifier) (*Service, error) {
	if repository == nil || transactor == nil || applications == nil {
		return nil, errors.New("rule repository, transactor, and application verifier are required")
	}
	return &Service{repository: repository, transactor: transactor, applications: applications, now: time.Now}, nil
}

func (s *Service) CreateRuleSet(ctx context.Context, tenantID, applicationID, code, name, description string) (RuleSet, error) {
	actorID, err := authorize(ctx, tenantID)
	if err != nil {
		return RuleSet{}, err
	}
	tenantID, applicationID, code, name = strings.TrimSpace(tenantID), strings.TrimSpace(applicationID), strings.ToLower(strings.TrimSpace(code)), strings.TrimSpace(name)
	if tenantID == "" || applicationID == "" || !ruleCodePattern.MatchString(code) || name == "" {
		return RuleSet{}, apperror.Invalid("tenant_id, application_id, valid code, and name are required", nil)
	}
	if err := s.verifyApplication(ctx, tenantID, applicationID); err != nil {
		return RuleSet{}, err
	}
	now := s.now()
	value := RuleSet{ID: uuid.NewString(), TenantID: tenantID, ApplicationID: applicationID, Code: code, Name: name, Description: strings.TrimSpace(description), Status: "draft", Audit: newAudit(actorID, now)}
	err = s.transactor.Within(ctx, nil, func(tx *sqlx.Tx) error { return s.repository.CreateRuleSet(ctx, tx, value) })
	return value, translate(err)
}

func (s *Service) UpdateRuleSet(ctx context.Context, tenantID, applicationID, id, name, description, status string, expected int64) (RuleSet, error) {
	actorID, err := authorize(ctx, tenantID)
	if err != nil {
		return RuleSet{}, err
	}
	status, name = strings.ToLower(strings.TrimSpace(status)), strings.TrimSpace(name)
	if id == "" || name == "" || expected < 1 || !map[string]bool{"draft": true, "active": true, "disabled": true}[status] {
		return RuleSet{}, apperror.Invalid("id, name, valid status, and positive version are required", nil)
	}
	applicationID = strings.TrimSpace(applicationID)
	if applicationID == "" {
		return RuleSet{}, apperror.Invalid("application_id is required", nil)
	}
	if err := s.verifyApplication(ctx, tenantID, applicationID); err != nil {
		return RuleSet{}, err
	}
	value, err := s.repository.GetRuleSet(ctx, strings.TrimSpace(tenantID), applicationID, strings.TrimSpace(id), "")
	if err != nil {
		return RuleSet{}, translate(err)
	}
	value.Name, value.Description, value.Status = name, strings.TrimSpace(description), status
	value.Version, value.UpdatedAt, value.UpdatedBy = expected+1, s.now(), actorID
	err = s.transactor.Within(ctx, nil, func(tx *sqlx.Tx) error { return s.repository.UpdateRuleSet(ctx, tx, value, expected) })
	return value, translate(err)
}

func (s *Service) GetRuleSet(ctx context.Context, tenantID, applicationID, id, code string) (RuleSet, *RuleVersion, error) {
	if _, err := authorize(ctx, tenantID); err != nil {
		return RuleSet{}, nil, err
	}
	if err := s.verifyApplication(ctx, tenantID, applicationID); err != nil {
		return RuleSet{}, nil, err
	}
	value, err := s.repository.GetRuleSet(ctx, strings.TrimSpace(tenantID), strings.TrimSpace(applicationID), strings.TrimSpace(id), strings.ToLower(strings.TrimSpace(code)))
	if err != nil {
		return RuleSet{}, nil, translate(err)
	}
	if value.PublishedVersionNumber == 0 {
		return value, nil, nil
	}
	published, err := s.repository.GetRuleVersion(ctx, value.TenantID, value.ApplicationID, value.ID, "", value.PublishedVersionNumber)
	return value, &published, translate(err)
}

func (s *Service) ListRuleSets(ctx context.Context, tenantID, applicationID, status, keyword string, page, pageSize int) (Page[RuleSet], error) {
	if _, err := authorize(ctx, tenantID); err != nil {
		return Page[RuleSet]{}, err
	}
	if err := s.verifyApplication(ctx, tenantID, applicationID); err != nil {
		return Page[RuleSet]{}, err
	}
	page, pageSize = normalizePage(page, pageSize)
	items, total, err := s.repository.ListRuleSets(ctx, strings.TrimSpace(tenantID), strings.TrimSpace(applicationID), strings.ToLower(strings.TrimSpace(status)), strings.TrimSpace(keyword), pageSize, (page-1)*pageSize)
	return Page[RuleSet]{Items: items, Total: total, Page: page, PageSize: pageSize}, translate(err)
}

func (s *Service) CreateRuleVersion(ctx context.Context, tenantID, applicationID, ruleSetID, definitionJSON, idempotencyKey string) (RuleVersion, bool, error) {
	actorID, err := authorize(ctx, tenantID)
	if err != nil {
		return RuleVersion{}, false, err
	}
	if err := s.verifyApplication(ctx, tenantID, applicationID); err != nil {
		return RuleVersion{}, false, err
	}
	if strings.TrimSpace(ruleSetID) == "" || strings.TrimSpace(idempotencyKey) == "" || len(idempotencyKey) > 191 {
		return RuleVersion{}, false, apperror.Invalid("rule_set_id and idempotency_key are required", nil)
	}
	compiled, issues := CompileDefinition(definitionJSON)
	if len(issues) > 0 {
		return RuleVersion{}, false, apperror.Invalid("invalid rule definition", fmt.Errorf("%s: %s", issues[0].Path, issues[0].Message))
	}
	now := s.now()
	value := RuleVersion{ID: uuid.NewString(), TenantID: strings.TrimSpace(tenantID), ApplicationID: strings.TrimSpace(applicationID), RuleSetID: strings.TrimSpace(ruleSetID), Status: "draft", DefinitionJSON: compiled.CanonicalJSON(), Checksum: compiled.Checksum(), IdempotencyKey: strings.TrimSpace(idempotencyKey), Audit: newAudit(actorID, now)}
	created := false
	err = s.transactor.Within(ctx, nil, func(tx *sqlx.Tx) error {
		var repositoryErr error
		value, created, repositoryErr = s.repository.CreateRuleVersion(ctx, tx, value)
		return repositoryErr
	})
	if err == nil {
		s.compiled.Store(value.Checksum, compiled)
	}
	return value, created, translate(err)
}

func (s *Service) ValidateRuleVersion(ctx context.Context, tenantID, applicationID, definitionJSON string) (string, []ValidationIssue, error) {
	if _, err := authorize(ctx, tenantID); err != nil {
		return "", nil, err
	}
	if err := s.verifyApplication(ctx, tenantID, applicationID); err != nil {
		return "", nil, err
	}
	compiled, issues := CompileDefinition(definitionJSON)
	if len(issues) > 0 {
		return "", issues, nil
	}
	return compiled.Checksum(), nil, nil
}

func (s *Service) PublishRuleVersion(ctx context.Context, tenantID, applicationID, ruleSetID, versionID string, ruleSetExpected, versionExpected int64) (RuleSet, RuleVersion, error) {
	actorID, err := authorize(ctx, tenantID)
	if err != nil {
		return RuleSet{}, RuleVersion{}, err
	}
	if err := s.verifyApplication(ctx, tenantID, applicationID); err != nil {
		return RuleSet{}, RuleVersion{}, err
	}
	if ruleSetExpected < 1 || versionExpected < 1 {
		return RuleSet{}, RuleVersion{}, apperror.Invalid("positive rule set and rule version versions are required", nil)
	}
	ruleSet, err := s.repository.GetRuleSet(ctx, tenantID, applicationID, ruleSetID, "")
	if err != nil {
		return RuleSet{}, RuleVersion{}, translate(err)
	}
	version, err := s.repository.GetRuleVersion(ctx, tenantID, applicationID, ruleSetID, versionID, 0)
	if err != nil {
		return RuleSet{}, RuleVersion{}, translate(err)
	}
	if version.Status != "draft" {
		return RuleSet{}, RuleVersion{}, apperror.Conflict("only draft rule versions can be published", nil)
	}
	compiled, issues := CompileDefinition(version.DefinitionJSON)
	if len(issues) > 0 || compiled.Checksum() != version.Checksum {
		return RuleSet{}, RuleVersion{}, apperror.Conflict("stored rule definition failed validation", nil)
	}
	now := s.now()
	version.Status, version.PublishedAt, version.Version, version.UpdatedAt, version.UpdatedBy = "published", &now, versionExpected+1, now, actorID
	ruleSet.Status, ruleSet.PublishedVersionNumber, ruleSet.Version, ruleSet.UpdatedAt, ruleSet.UpdatedBy = "active", version.VersionNumber, ruleSetExpected+1, now, actorID
	err = s.transactor.Within(ctx, nil, func(tx *sqlx.Tx) error {
		if err := s.repository.PublishRuleVersion(ctx, tx, ruleSet, version, ruleSetExpected, versionExpected); err != nil {
			return err
		}
		payload := &rulev1.RuleVersionPublishedEvent{RuleSet: ToProtoRuleSet(ruleSet), RuleVersion: ToProtoRuleVersion(version)}
		return s.addEvent(ctx, tx, ruleSet, version, actorID, now, payload)
	})
	if err == nil {
		s.compiled.Store(version.Checksum, compiled)
	}
	return ruleSet, version, translate(err)
}

func (s *Service) ListRuleVersions(ctx context.Context, tenantID, applicationID, ruleSetID, status string, page, pageSize int) (Page[RuleVersion], error) {
	if _, err := authorize(ctx, tenantID); err != nil {
		return Page[RuleVersion]{}, err
	}
	if err := s.verifyApplication(ctx, tenantID, applicationID); err != nil {
		return Page[RuleVersion]{}, err
	}
	page, pageSize = normalizePage(page, pageSize)
	items, total, err := s.repository.ListRuleVersions(ctx, strings.TrimSpace(tenantID), strings.TrimSpace(applicationID), strings.TrimSpace(ruleSetID), strings.ToLower(strings.TrimSpace(status)), pageSize, (page-1)*pageSize)
	return Page[RuleVersion]{Items: items, Total: total, Page: page, PageSize: pageSize}, translate(err)
}

func (s *Service) Evaluate(ctx context.Context, tenantID, applicationID, ruleSetID, ruleSetCode string, versionNumber int64, factsJSON string) (Evaluation, RuleVersion, error) {
	if _, err := authorize(ctx, tenantID); err != nil {
		return Evaluation{}, RuleVersion{}, err
	}
	if err := s.verifyApplication(ctx, tenantID, applicationID); err != nil {
		return Evaluation{}, RuleVersion{}, err
	}
	ruleSet, err := s.repository.GetRuleSet(ctx, strings.TrimSpace(tenantID), strings.TrimSpace(applicationID), strings.TrimSpace(ruleSetID), strings.ToLower(strings.TrimSpace(ruleSetCode)))
	if err != nil {
		return Evaluation{}, RuleVersion{}, translate(err)
	}
	if ruleSet.Status != "active" {
		return Evaluation{}, RuleVersion{}, apperror.Conflict("rule set is not active", nil)
	}
	if versionNumber == 0 {
		versionNumber = ruleSet.PublishedVersionNumber
	}
	version, err := s.repository.GetRuleVersion(ctx, ruleSet.TenantID, ruleSet.ApplicationID, ruleSet.ID, "", versionNumber)
	if err != nil {
		return Evaluation{}, RuleVersion{}, translate(err)
	}
	if version.Status != "published" {
		return Evaluation{}, RuleVersion{}, apperror.Conflict("rule version is not published", nil)
	}
	compiled, err := s.compiledDefinition(version)
	if err != nil {
		return Evaluation{}, RuleVersion{}, apperror.Internal(err)
	}
	evaluation, err := compiled.Evaluate(ctx, factsJSON)
	if err != nil {
		if errors.Is(err, ErrInvalidDefinition) {
			return Evaluation{}, RuleVersion{}, apperror.Invalid("invalid facts", err)
		}
		return Evaluation{}, RuleVersion{}, apperror.Internal(err)
	}
	return evaluation, version, nil
}

func (s *Service) BatchEvaluate(ctx context.Context, inputs []EvaluationInput) ([]BatchEvaluationResult, error) {
	if len(inputs) == 0 || len(inputs) > 100 {
		return nil, apperror.Invalid("requests must contain between 1 and 100 items", nil)
	}
	results := make([]BatchEvaluationResult, len(inputs))
	for index, input := range inputs {
		evaluation, version, err := s.Evaluate(ctx, input.TenantID, input.ApplicationID, input.RuleSetID, input.RuleSetCode, input.VersionNumber, input.FactsJSON)
		results[index] = BatchEvaluationResult{Index: index, Evaluation: evaluation, Version: version, Err: err}
	}
	return results, nil
}

func (s *Service) compiledDefinition(version RuleVersion) (*CompiledDefinition, error) {
	if cached, ok := s.compiled.Load(version.Checksum); ok {
		return cached.(*CompiledDefinition), nil
	}
	compiled, issues := CompileDefinition(version.DefinitionJSON)
	if len(issues) > 0 || compiled.Checksum() != version.Checksum {
		return nil, fmt.Errorf("published definition checksum or validation mismatch")
	}
	actual, _ := s.compiled.LoadOrStore(version.Checksum, compiled)
	return actual.(*CompiledDefinition), nil
}

func (s *Service) addEvent(ctx context.Context, tx *sqlx.Tx, ruleSet RuleSet, version RuleVersion, actorID string, at time.Time, payload proto.Message) error {
	envelope, err := platformevents.NewEnvelope(platformevents.Metadata{EventID: uuid.NewString(), EventType: "platform.rule.v1.RuleVersionPublished", AggregateID: ruleSet.ID, AggregateType: "rule_set", TenantID: ruleSet.TenantID, ApplicationID: ruleSet.ApplicationID, SchemaVersion: 1, ActorID: actorID, OccurredAt: at}, payload)
	if err != nil {
		return err
	}
	encoded, err := proto.Marshal(envelope)
	if err != nil {
		return err
	}
	return s.repository.AddOutbox(ctx, tx, OutboxEvent{ID: envelope.GetEventId(), Subject: "platform.rule.rule-version.published.v1", Envelope: encoded, AvailableAt: at, CreatedAt: at, UpdatedAt: at, CreatedBy: actorID, UpdatedBy: actorID})
}

func (s *Service) verifyApplication(ctx context.Context, tenantID, applicationID string) error {
	if strings.TrimSpace(applicationID) == "" {
		return apperror.Invalid("application_id is required", nil)
	}
	err := s.applications.Verify(ctx, strings.TrimSpace(tenantID), strings.TrimSpace(applicationID))
	if errors.Is(err, appaccess.ErrNotGranted) {
		return apperror.New(apperror.CodeForbidden, "application access denied", 403, err)
	}
	if err != nil {
		return apperror.Unavailable("application authorization is unavailable", err)
	}
	return nil
}

func authorize(ctx context.Context, tenantID string) (string, error) {
	principal, ok := platformprincipal.FromContext(ctx)
	if !ok {
		return "", apperror.Unauthorized("authenticated actor is required")
	}
	if principal.Type != platformprincipal.TypeServiceAccount && principal.Type != platformprincipal.TypeSystem && (principal.TenantID == "" || principal.TenantID != strings.TrimSpace(tenantID)) {
		return "", apperror.New(apperror.CodeForbidden, "tenant access denied", 403, nil)
	}
	return principal.ID, nil
}

func newAudit(actorID string, now time.Time) Audit {
	return Audit{Version: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: actorID, UpdatedBy: actorID}
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize
}

func translate(err error) error {
	if err == nil {
		return nil
	}
	var applicationError *apperror.Error
	if errors.As(err, &applicationError) {
		return err
	}
	switch {
	case errors.Is(err, ErrNotFound):
		return apperror.NotFound("rule resource not found")
	case errors.Is(err, ErrStaleVersion), errors.Is(err, ErrConflict):
		return apperror.Conflict("rule resource conflict", err)
	default:
		return apperror.Internal(err)
	}
}
