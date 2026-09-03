package grpctransport

import (
	"context"
	"errors"

	commonv1 "github.com/lihongjie0209/platform-protos/gen/go/platform/common/v1"
	rulev1 "github.com/lihongjie0209/platform-protos/gen/go/platform/rule/v1"
	"github.com/lihongjie0209/rule-service/internal/apperror"
	"github.com/lihongjie0209/rule-service/internal/rule"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ruleServer struct {
	rulev1.UnimplementedRuleServiceServer
	service *rule.Service
}

func (s *ruleServer) CreateRuleSet(ctx context.Context, request *rulev1.CreateRuleSetRequest) (*rulev1.CreateRuleSetResponse, error) {
	value, err := s.service.CreateRuleSet(ctx, request.GetTenantId(), request.GetApplicationId(), request.GetCode(), request.GetName(), request.GetDescription())
	return &rulev1.CreateRuleSetResponse{RuleSet: rule.ToProtoRuleSet(value)}, ruleError(err)
}
func (s *ruleServer) UpdateRuleSet(ctx context.Context, request *rulev1.UpdateRuleSetRequest) (*rulev1.UpdateRuleSetResponse, error) {
	value, err := s.service.UpdateRuleSet(ctx, request.GetTenantId(), request.GetApplicationId(), request.GetId(), request.GetName(), request.GetDescription(), request.GetStatus(), request.GetVersion())
	return &rulev1.UpdateRuleSetResponse{RuleSet: rule.ToProtoRuleSet(value)}, ruleError(err)
}
func (s *ruleServer) GetRuleSet(ctx context.Context, request *rulev1.GetRuleSetRequest) (*rulev1.GetRuleSetResponse, error) {
	value, published, err := s.service.GetRuleSet(ctx, request.GetTenantId(), request.GetApplicationId(), request.GetId(), request.GetCode())
	response := &rulev1.GetRuleSetResponse{RuleSet: rule.ToProtoRuleSet(value)}
	if published != nil {
		response.PublishedVersion = rule.ToProtoRuleVersion(*published)
	}
	return response, ruleError(err)
}
func (s *ruleServer) ListRuleSets(ctx context.Context, request *rulev1.ListRuleSetsRequest) (*rulev1.ListRuleSetsResponse, error) {
	page, size := grpcPage(request.GetPage())
	values, err := s.service.ListRuleSets(ctx, request.GetTenantId(), request.GetApplicationId(), request.GetStatus(), request.GetKeyword(), page, size)
	items := make([]*rulev1.RuleSet, len(values.Items))
	for index := range values.Items {
		items[index] = rule.ToProtoRuleSet(values.Items[index])
	}
	return &rulev1.ListRuleSetsResponse{RuleSets: items, Page: grpcPageResult(values.Total, values.Page, values.PageSize)}, ruleError(err)
}
func (s *ruleServer) CreateRuleVersion(ctx context.Context, request *rulev1.CreateRuleVersionRequest) (*rulev1.CreateRuleVersionResponse, error) {
	value, _, err := s.service.CreateRuleVersion(ctx, request.GetTenantId(), request.GetApplicationId(), request.GetRuleSetId(), request.GetDefinitionJson(), request.GetIdempotencyKey())
	return &rulev1.CreateRuleVersionResponse{RuleVersion: rule.ToProtoRuleVersion(value)}, ruleError(err)
}
func (s *ruleServer) ValidateRuleVersion(ctx context.Context, request *rulev1.ValidateRuleVersionRequest) (*rulev1.ValidateRuleVersionResponse, error) {
	checksum, issues, err := s.service.ValidateRuleVersion(ctx, request.GetTenantId(), request.GetApplicationId(), request.GetDefinitionJson())
	values := make([]*rulev1.ValidationIssue, len(issues))
	for index := range issues {
		values[index] = &rulev1.ValidationIssue{Path: issues[index].Path, Code: issues[index].Code, Message: issues[index].Message}
	}
	return &rulev1.ValidateRuleVersionResponse{Valid: len(values) == 0, Issues: values, Checksum: checksum}, ruleError(err)
}
func (s *ruleServer) PublishRuleVersion(ctx context.Context, request *rulev1.PublishRuleVersionRequest) (*rulev1.PublishRuleVersionResponse, error) {
	ruleSet, version, err := s.service.PublishRuleVersion(ctx, request.GetTenantId(), request.GetApplicationId(), request.GetRuleSetId(), request.GetRuleVersionId(), request.GetRuleSetVersion(), request.GetRuleVersionVersion())
	return &rulev1.PublishRuleVersionResponse{RuleSet: rule.ToProtoRuleSet(ruleSet), RuleVersion: rule.ToProtoRuleVersion(version)}, ruleError(err)
}
func (s *ruleServer) GetRuleVersion(ctx context.Context, request *rulev1.GetRuleVersionRequest) (*rulev1.GetRuleVersionResponse, error) {
	value, err := s.service.GetRuleVersion(ctx, request.GetTenantId(), request.GetApplicationId(), request.GetRuleSetId(), request.GetId())
	return &rulev1.GetRuleVersionResponse{RuleVersion: rule.ToProtoRuleVersion(value)}, ruleError(err)
}
func (s *ruleServer) ListRuleVersions(ctx context.Context, request *rulev1.ListRuleVersionsRequest) (*rulev1.ListRuleVersionsResponse, error) {
	page, size := grpcPage(request.GetPage())
	values, err := s.service.ListRuleVersions(ctx, request.GetTenantId(), request.GetApplicationId(), request.GetRuleSetId(), request.GetStatus(), page, size)
	items := make([]*rulev1.RuleVersion, len(values.Items))
	for index := range values.Items {
		items[index] = rule.ToProtoRuleVersion(values.Items[index])
	}
	return &rulev1.ListRuleVersionsResponse{RuleVersions: items, Page: grpcPageResult(values.Total, values.Page, values.PageSize)}, ruleError(err)
}
func (s *ruleServer) Evaluate(ctx context.Context, request *rulev1.EvaluateRequest) (*rulev1.EvaluateResponse, error) {
	evaluation, version, err := s.service.Evaluate(ctx, request.GetTenantId(), request.GetApplicationId(), request.GetRuleSetId(), request.GetRuleSetCode(), request.GetVersionNumber(), request.GetFactsJson())
	return &rulev1.EvaluateResponse{Matched: evaluation.Matched, MatchedRule: evaluation.MatchedRule, ResultJson: evaluation.ResultJSON, EvaluatedVersionNumber: version.VersionNumber, Checksum: version.Checksum}, ruleError(err)
}
func (s *ruleServer) BatchEvaluate(ctx context.Context, request *rulev1.BatchEvaluateRequest) (*rulev1.BatchEvaluateResponse, error) {
	inputs := make([]rule.EvaluationInput, len(request.GetRequests()))
	for index, item := range request.GetRequests() {
		inputs[index] = rule.EvaluationInput{TenantID: item.GetTenantId(), ApplicationID: item.GetApplicationId(), RuleSetID: item.GetRuleSetId(), RuleSetCode: item.GetRuleSetCode(), VersionNumber: item.GetVersionNumber(), FactsJSON: item.GetFactsJson()}
	}
	evaluations, err := s.service.BatchEvaluate(ctx, inputs)
	if err != nil {
		return nil, ruleError(err)
	}
	results := make([]*rulev1.BatchEvaluateResult, len(evaluations))
	for index, evaluation := range evaluations {
		result := &rulev1.BatchEvaluateResult{Index: int32(evaluation.Index)}
		if evaluation.Err != nil {
			mapped := ruleError(evaluation.Err)
			result.ErrorCode, result.ErrorMessage = status.Code(mapped).String(), status.Convert(mapped).Message()
		} else {
			result.Response = &rulev1.EvaluateResponse{Matched: evaluation.Evaluation.Matched, MatchedRule: evaluation.Evaluation.MatchedRule, ResultJson: evaluation.Evaluation.ResultJSON, EvaluatedVersionNumber: evaluation.Version.VersionNumber, Checksum: evaluation.Version.Checksum}
		}
		results[index] = result
	}
	return &rulev1.BatchEvaluateResponse{Results: results}, nil
}

func grpcPage(value *commonv1.PageRequest) (int, int) {
	if value == nil {
		return 0, 0
	}
	return int(value.GetPage()), int(value.GetPageSize())
}
func grpcPageResult(total int64, page, size int) *commonv1.PageResult {
	return &commonv1.PageResult{Total: uint64(total), Page: uint32(page), PageSize: uint32(size)}
}
func ruleError(err error) error {
	if err == nil {
		return nil
	}
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		return status.Error(codes.Internal, "internal server error")
	}
	mapping := map[int]codes.Code{apperror.CodeInvalidArgument: codes.InvalidArgument, apperror.CodeNotFound: codes.NotFound, apperror.CodeUnauthorized: codes.Unauthenticated, apperror.CodeForbidden: codes.PermissionDenied, apperror.CodeConflict: codes.Aborted, apperror.CodeRequestInProgress: codes.Aborted, apperror.CodeDependencyUnavailable: codes.Unavailable, apperror.CodeRequestTimeout: codes.DeadlineExceeded, apperror.CodeTooManyRequests: codes.ResourceExhausted}
	code, ok := mapping[appErr.Code]
	if !ok {
		code = codes.Internal
	}
	return status.Error(code, appErr.Message)
}
