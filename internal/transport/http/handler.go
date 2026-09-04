package httptransport

import (
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lihongjie0209/rule-service/internal/apperror"
	"github.com/lihongjie0209/rule-service/internal/buildinfo"
	"github.com/lihongjie0209/rule-service/internal/health"
	"github.com/lihongjie0209/rule-service/internal/rule"
)

type Handler struct {
	logger *slog.Logger
	health *health.Service
	rules  *rule.Service
}

func NewHandler(healthService *health.Service, ruleService *rule.Service, logger *slog.Logger) *Handler {
	return &Handler{health: healthService, rules: ruleService, logger: logger}
}

type MeResponseBody struct {
	Subject string `json:"subject"`
}

// Login godoc
// @Summary Issue a JWT access token
// @Tags authentication
// @Accept json
// @Produce json
// @Param request body LoginRequest true "Client credentials"
// @Success 200 {object} Response{body=LoginResponseBody}
// @Failure 400 {object} Response "Code 10001: invalid request"
// @Failure 401 {object} Response "Code 20001: invalid credentials"
// @Failure 429 {object} Response "Code 10029: rate limited"

// Live godoc
// @Summary Check process liveness
// @Tags operations
// @Produce json
// @Success 200 {object} Response{body=health.Status}
// @Router /live [post]
func (h *Handler) Live(c *gin.Context) { OK(c, h.health.Live()) }

// Ready godoc
// @Summary Check database and Redis readiness
// @Tags operations
// @Produce json
// @Success 200 {object} Response{body=health.Status}
// @Failure 503 {object} Response{body=health.Status} "Code 50003: dependency unavailable"
// @Router /ready [post]
func (h *Handler) Ready(c *gin.Context) {
	status, ready := h.health.Ready(c.Request.Context())
	if !ready {
		c.JSON(503, Response{Code: apperror.CodeDependencyUnavailable, Message: "service is not ready", Body: status, RequestID: requestID(c)})
		return
	}
	OK(c, status)
}

// Me godoc
// @Summary Return the authenticated subject
// @Tags authentication
// @Produce json
// @Security Bearer
// @Success 200 {object} Response{body=MeResponseBody}
// @Failure 401 {object} Response "Code 20001: unauthorized"
// @Router /api/v1/me [post]
func (h *Handler) Me(c *gin.Context) {
	subject, _ := c.Get("subject")
	OK(c, gin.H{"subject": subject})
}

// Version godoc
// @Summary Return build and runtime version information
// @Tags operations
// @Produce json
// @Success 200 {object} Response{body=buildinfo.Info}
// @Router /api/v1/version [post]
func (h *Handler) Version(c *gin.Context) { OK(c, buildinfo.Current()) }

type RuleSetView struct {
	ID                     string    `json:"id"`
	TenantID               string    `json:"tenant_id"`
	ApplicationID          string    `json:"application_id"`
	Code                   string    `json:"code"`
	Name                   string    `json:"name"`
	Description            string    `json:"description"`
	Status                 string    `json:"status"`
	PublishedVersionNumber int64     `json:"published_version_number"`
	Version                int64     `json:"version"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
	CreatedBy              string    `json:"created_by"`
	UpdatedBy              string    `json:"updated_by"`
}
type RuleVersionView struct {
	ID            string          `json:"id"`
	TenantID      string          `json:"tenant_id"`
	ApplicationID string          `json:"application_id"`
	RuleSetID     string          `json:"rule_set_id"`
	VersionNumber int64           `json:"version_number"`
	Status        string          `json:"status"`
	Definition    json.RawMessage `json:"definition" swaggertype:"object"`
	Checksum      string          `json:"checksum"`
	PublishedAt   *time.Time      `json:"published_at,omitempty"`
	Version       int64           `json:"version"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	CreatedBy     string          `json:"created_by"`
	UpdatedBy     string          `json:"updated_by"`
}
type PageRequest struct {
	TenantID      string `json:"tenant_id"`
	ApplicationID string `json:"application_id"`
	Status        string `json:"status"`
	Keyword       string `json:"keyword"`
	Page          int    `json:"page"`
	PageSize      int    `json:"page_size"`
}
type CreateRuleSetRequest struct {
	TenantID      string `json:"tenant_id"`
	ApplicationID string `json:"application_id"`
	Code          string `json:"code"`
	Name          string `json:"name"`
	Description   string `json:"description"`
}
type UpdateRuleSetRequest struct {
	TenantID      string `json:"tenant_id"`
	ApplicationID string `json:"application_id"`
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Status        string `json:"status"`
	Version       int64  `json:"version"`
}
type GetRuleSetRequest struct {
	TenantID      string `json:"tenant_id"`
	ApplicationID string `json:"application_id"`
	ID            string `json:"id"`
	Code          string `json:"code"`
}
type CreateRuleVersionRequest struct {
	TenantID       string          `json:"tenant_id"`
	ApplicationID  string          `json:"application_id"`
	RuleSetID      string          `json:"rule_set_id"`
	Definition     json.RawMessage `json:"definition" swaggertype:"object"`
	IdempotencyKey string          `json:"idempotency_key"`
	RuleSetVersion int64           `json:"rule_set_version"`
}
type ValidateRuleVersionRequest struct {
	TenantID      string          `json:"tenant_id"`
	ApplicationID string          `json:"application_id"`
	Definition    json.RawMessage `json:"definition" swaggertype:"object"`
}
type PublishRuleVersionRequest struct {
	TenantID           string `json:"tenant_id"`
	ApplicationID      string `json:"application_id"`
	RuleSetID          string `json:"rule_set_id"`
	RuleVersionID      string `json:"rule_version_id"`
	RuleSetVersion     int64  `json:"rule_set_version"`
	RuleVersionVersion int64  `json:"rule_version_version"`
}
type ListRuleVersionsRequest struct {
	TenantID      string `json:"tenant_id"`
	ApplicationID string `json:"application_id"`
	RuleSetID     string `json:"rule_set_id"`
	Status        string `json:"status"`
	Page          int    `json:"page"`
	PageSize      int    `json:"page_size"`
}
type GetRuleVersionRequest struct {
	TenantID      string `json:"tenant_id"`
	ApplicationID string `json:"application_id"`
	RuleSetID     string `json:"rule_set_id"`
	ID            string `json:"id"`
}
type EvaluateRequest struct {
	TenantID      string          `json:"tenant_id"`
	ApplicationID string          `json:"application_id"`
	RuleSetID     string          `json:"rule_set_id"`
	RuleSetCode   string          `json:"rule_set_code"`
	VersionNumber int64           `json:"version_number"`
	Facts         json.RawMessage `json:"facts" swaggertype:"object"`
}
type BatchEvaluateRequest struct {
	Requests []EvaluateRequest `json:"requests"`
}
type EvaluateResponse struct {
	Matched                bool            `json:"matched"`
	MatchedRule            string          `json:"matched_rule"`
	Result                 json.RawMessage `json:"result" swaggertype:"object"`
	EvaluatedVersionNumber int64           `json:"evaluated_version_number"`
	Checksum               string          `json:"checksum"`
}
type BatchEvaluateResult struct {
	Index        int               `json:"index"`
	Response     *EvaluateResponse `json:"response,omitempty"`
	ErrorCode    int               `json:"error_code,omitempty"`
	ErrorMessage string            `json:"error_message,omitempty"`
}
type BatchEvaluateResponse struct {
	Results []BatchEvaluateResult `json:"results"`
}
type RuleSetDetailBody struct {
	RuleSet          RuleSetView      `json:"rule_set"`
	PublishedVersion *RuleVersionView `json:"published_version,omitempty"`
}
type RuleSetPageBody struct {
	Items    []RuleSetView `json:"items"`
	Total    int64         `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}
type CreateRuleVersionBody struct {
	RuleVersion RuleVersionView `json:"rule_version"`
	Duplicate   bool            `json:"duplicate"`
}
type ValidationIssueBody struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
type ValidateRuleVersionBody struct {
	Valid    bool                  `json:"valid"`
	Issues   []ValidationIssueBody `json:"issues"`
	Checksum string                `json:"checksum"`
}
type PublishRuleVersionBody struct {
	RuleSet     RuleSetView     `json:"rule_set"`
	RuleVersion RuleVersionView `json:"rule_version"`
}
type RuleVersionPageBody struct {
	Items    []RuleVersionView `json:"items"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

func bind(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		Fail(c, slog.Default(), apperror.Invalid("invalid JSON request", err))
		return false
	}
	return true
}
func ruleSetView(v rule.RuleSet) RuleSetView {
	return RuleSetView{ID: v.ID, TenantID: v.TenantID, ApplicationID: v.ApplicationID, Code: v.Code, Name: v.Name, Description: v.Description, Status: v.Status, PublishedVersionNumber: v.PublishedVersionNumber, Version: v.Version, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, CreatedBy: v.CreatedBy, UpdatedBy: v.UpdatedBy}
}
func ruleVersionView(v rule.RuleVersion) RuleVersionView {
	return RuleVersionView{ID: v.ID, TenantID: v.TenantID, ApplicationID: v.ApplicationID, RuleSetID: v.RuleSetID, VersionNumber: v.VersionNumber, Status: v.Status, Definition: json.RawMessage(v.DefinitionJSON), Checksum: v.Checksum, PublishedAt: v.PublishedAt, Version: v.Version, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, CreatedBy: v.CreatedBy, UpdatedBy: v.UpdatedBy}
}

// CreateRuleSet godoc
// @Summary Create a rule set
// @Tags rules
// @Accept json
// @Produce json
// @Security Bearer
// @Security PSK
// @Param request body CreateRuleSetRequest true "Rule set"
// @Success 200 {object} Response{body=RuleSetView}
// @Router /api/v1/rule-sets/create [post]
func (h *Handler) CreateRuleSet(c *gin.Context) {
	var r CreateRuleSetRequest
	if !bind(c, &r) {
		return
	}
	v, e := h.rules.CreateRuleSet(c.Request.Context(), r.TenantID, r.ApplicationID, r.Code, r.Name, r.Description)
	if e != nil {
		Fail(c, h.logger, e)
		return
	}
	OK(c, ruleSetView(v))
}

// UpdateRuleSet godoc
// @Summary Update a rule set with optimistic locking
// @Tags rules
// @Accept json
// @Produce json
// @Security Bearer
// @Security PSK
// @Param request body UpdateRuleSetRequest true "Rule set update"
// @Success 200 {object} Response{body=RuleSetView}
// @Router /api/v1/rule-sets/update [post]
func (h *Handler) UpdateRuleSet(c *gin.Context) {
	var r UpdateRuleSetRequest
	if !bind(c, &r) {
		return
	}
	v, e := h.rules.UpdateRuleSet(c.Request.Context(), r.TenantID, r.ApplicationID, r.ID, r.Name, r.Description, r.Status, r.Version)
	if e != nil {
		Fail(c, h.logger, e)
		return
	}
	OK(c, ruleSetView(v))
}

// GetRuleSet godoc
// @Summary Get a rule set and its published version
// @Tags rules
// @Accept json
// @Produce json
// @Security Bearer
// @Security PSK
// @Param request body GetRuleSetRequest true "Rule set selector"
// @Success 200 {object} Response{body=RuleSetDetailBody}
// @Router /api/v1/rule-sets/get [post]
func (h *Handler) GetRuleSet(c *gin.Context) {
	var r GetRuleSetRequest
	if !bind(c, &r) {
		return
	}
	v, p, e := h.rules.GetRuleSet(c.Request.Context(), r.TenantID, r.ApplicationID, r.ID, r.Code)
	if e != nil {
		Fail(c, h.logger, e)
		return
	}
	body := RuleSetDetailBody{RuleSet: ruleSetView(v)}
	if p != nil {
		published := ruleVersionView(*p)
		body.PublishedVersion = &published
	}
	OK(c, body)
}

// ListRuleSets godoc
// @Summary List rule sets
// @Tags rules
// @Accept json
// @Produce json
// @Security Bearer
// @Security PSK
// @Param request body PageRequest true "Search and pagination"
// @Success 200 {object} Response{body=RuleSetPageBody}
// @Router /api/v1/rule-sets/list [post]
func (h *Handler) ListRuleSets(c *gin.Context) {
	var r PageRequest
	if !bind(c, &r) {
		return
	}
	v, e := h.rules.ListRuleSets(c.Request.Context(), r.TenantID, r.ApplicationID, r.Status, r.Keyword, r.Page, r.PageSize)
	if e != nil {
		Fail(c, h.logger, e)
		return
	}
	items := make([]RuleSetView, len(v.Items))
	for i := range v.Items {
		items[i] = ruleSetView(v.Items[i])
	}
	OK(c, RuleSetPageBody{Items: items, Total: v.Total, Page: v.Page, PageSize: v.PageSize})
}

// CreateRuleVersion godoc
// @Summary Create an immutable draft rule version
// @Tags rule versions
// @Accept json
// @Produce json
// @Security Bearer
// @Security PSK
// @Param request body CreateRuleVersionRequest true "Rule definition"
// @Success 200 {object} Response{body=CreateRuleVersionBody}
// @Router /api/v1/rule-versions/create [post]
func (h *Handler) CreateRuleVersion(c *gin.Context) {
	var r CreateRuleVersionRequest
	if !bind(c, &r) {
		return
	}
	v, d, e := h.rules.CreateRuleVersion(c.Request.Context(), r.TenantID, r.ApplicationID, r.RuleSetID, string(r.Definition), r.IdempotencyKey, r.RuleSetVersion)
	if e != nil {
		Fail(c, h.logger, e)
		return
	}
	OK(c, CreateRuleVersionBody{RuleVersion: ruleVersionView(v), Duplicate: !d})
}

// ValidateRuleVersion godoc
// @Summary Validate a rule definition without persisting it
// @Tags rule versions
// @Accept json
// @Produce json
// @Security Bearer
// @Security PSK
// @Param request body ValidateRuleVersionRequest true "Rule definition"
// @Success 200 {object} Response{body=ValidateRuleVersionBody}
// @Router /api/v1/rule-versions/validate [post]
func (h *Handler) ValidateRuleVersion(c *gin.Context) {
	var r ValidateRuleVersionRequest
	if !bind(c, &r) {
		return
	}
	checksum, issues, e := h.rules.ValidateRuleVersion(c.Request.Context(), r.TenantID, r.ApplicationID, string(r.Definition))
	if e != nil {
		Fail(c, h.logger, e)
		return
	}
	issueBodies := make([]ValidationIssueBody, len(issues))
	for i, issue := range issues {
		issueBodies[i] = ValidationIssueBody{Path: issue.Path, Code: issue.Code, Message: issue.Message}
	}
	OK(c, ValidateRuleVersionBody{Valid: len(issues) == 0, Issues: issueBodies, Checksum: checksum})
}

// PublishRuleVersion godoc
// @Summary Publish a rule version
// @Tags rule versions
// @Accept json
// @Produce json
// @Security Bearer
// @Security PSK
// @Param request body PublishRuleVersionRequest true "Publish versions"
// @Success 200 {object} Response{body=PublishRuleVersionBody}
// @Router /api/v1/rule-versions/publish [post]
func (h *Handler) PublishRuleVersion(c *gin.Context) {
	var r PublishRuleVersionRequest
	if !bind(c, &r) {
		return
	}
	set, v, e := h.rules.PublishRuleVersion(c.Request.Context(), r.TenantID, r.ApplicationID, r.RuleSetID, r.RuleVersionID, r.RuleSetVersion, r.RuleVersionVersion)
	if e != nil {
		Fail(c, h.logger, e)
		return
	}
	OK(c, PublishRuleVersionBody{RuleSet: ruleSetView(set), RuleVersion: ruleVersionView(v)})
}

// GetRuleVersion godoc
// @Summary Get a rule version
// @Tags rule versions
// @Accept json
// @Produce json
// @Security Bearer
// @Security PSK
// @Param request body GetRuleVersionRequest true "Rule version selector"
// @Success 200 {object} Response{body=RuleVersionView}
// @Router /api/v1/rule-versions/get [post]
func (h *Handler) GetRuleVersion(c *gin.Context) {
	var r GetRuleVersionRequest
	if !bind(c, &r) {
		return
	}
	v, e := h.rules.GetRuleVersion(c.Request.Context(), r.TenantID, r.ApplicationID, r.RuleSetID, r.ID)
	if e != nil {
		Fail(c, h.logger, e)
		return
	}
	OK(c, ruleVersionView(v))
}

// ListRuleVersions godoc
// @Summary List rule versions
// @Tags rule versions
// @Accept json
// @Produce json
// @Security Bearer
// @Security PSK
// @Param request body ListRuleVersionsRequest true "Rule version search"
// @Success 200 {object} Response{body=RuleVersionPageBody}
// @Router /api/v1/rule-versions/list [post]
func (h *Handler) ListRuleVersions(c *gin.Context) {
	var r ListRuleVersionsRequest
	if !bind(c, &r) {
		return
	}
	v, e := h.rules.ListRuleVersions(c.Request.Context(), r.TenantID, r.ApplicationID, r.RuleSetID, r.Status, r.Page, r.PageSize)
	if e != nil {
		Fail(c, h.logger, e)
		return
	}
	items := make([]RuleVersionView, len(v.Items))
	for i := range v.Items {
		items[i] = ruleVersionView(v.Items[i])
	}
	OK(c, RuleVersionPageBody{Items: items, Total: v.Total, Page: v.Page, PageSize: v.PageSize})
}

// Evaluate godoc
// @Summary Evaluate a published rule version
// @Tags rule evaluation
// @Accept json
// @Produce json
// @Security Bearer
// @Security PSK
// @Param request body EvaluateRequest true "Evaluation facts"
// @Success 200 {object} Response{body=EvaluateResponse}
// @Router /api/v1/rules/evaluate [post]
func (h *Handler) Evaluate(c *gin.Context) {
	var r EvaluateRequest
	if !bind(c, &r) {
		return
	}
	v, version, e := h.rules.Evaluate(c.Request.Context(), r.TenantID, r.ApplicationID, r.RuleSetID, r.RuleSetCode, r.VersionNumber, string(r.Facts))
	if e != nil {
		Fail(c, h.logger, e)
		return
	}
	OK(c, evaluationResponse(v, version))
}

// BatchEvaluate godoc
// @Summary Evaluate up to 100 rule requests
// @Tags rule evaluation
// @Accept json
// @Produce json
// @Security Bearer
// @Security PSK
// @Param request body BatchEvaluateRequest true "Evaluation batch"
// @Success 200 {object} Response{body=BatchEvaluateResponse}
// @Failure 400 {object} Response "Code 10001: batch size must be between 1 and 100"
// @Router /api/v1/rules/evaluate-batch [post]
func (h *Handler) BatchEvaluate(c *gin.Context) {
	var request BatchEvaluateRequest
	if !bind(c, &request) {
		return
	}
	inputs := make([]rule.EvaluationInput, len(request.Requests))
	for index, item := range request.Requests {
		inputs[index] = rule.EvaluationInput{TenantID: item.TenantID, ApplicationID: item.ApplicationID, RuleSetID: item.RuleSetID, RuleSetCode: item.RuleSetCode, VersionNumber: item.VersionNumber, FactsJSON: string(item.Facts)}
	}
	results, err := h.rules.BatchEvaluate(c.Request.Context(), inputs)
	if err != nil {
		Fail(c, h.logger, err)
		return
	}
	response := BatchEvaluateResponse{Results: make([]BatchEvaluateResult, len(results))}
	for index, result := range results {
		item := BatchEvaluateResult{Index: result.Index}
		if result.Err == nil {
			value := evaluationResponse(result.Evaluation, result.Version)
			item.Response = &value
		} else {
			var appErr *apperror.Error
			if errors.As(result.Err, &appErr) {
				item.ErrorCode, item.ErrorMessage = appErr.Code, appErr.Message
			} else {
				item.ErrorCode, item.ErrorMessage = apperror.CodeInternal, "internal server error"
			}
		}
		response.Results[index] = item
	}
	OK(c, response)
}

func evaluationResponse(value rule.Evaluation, version rule.RuleVersion) EvaluateResponse {
	return EvaluateResponse{Matched: value.Matched, MatchedRule: value.MatchedRule, Result: json.RawMessage(value.ResultJSON), EvaluatedVersionNumber: version.VersionNumber, Checksum: version.Checksum}
}
