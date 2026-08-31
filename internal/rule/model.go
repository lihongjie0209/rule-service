package rule

import "time"

type Audit struct {
	Version   int64     `db:"version" json:"version"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
	CreatedBy string    `db:"created_by" json:"created_by"`
	UpdatedBy string    `db:"updated_by" json:"updated_by"`
}

type RuleSet struct {
	ID                     string `db:"id" json:"id"`
	TenantID               string `db:"tenant_id" json:"tenant_id"`
	Code                   string `db:"code" json:"code"`
	Name                   string `db:"name" json:"name"`
	Description            string `db:"description" json:"description"`
	Status                 string `db:"status" json:"status"`
	PublishedVersionNumber int64  `db:"published_version_number" json:"published_version_number"`
	Audit
}

type RuleVersion struct {
	ID             string     `db:"id" json:"id"`
	TenantID       string     `db:"tenant_id" json:"tenant_id"`
	RuleSetID      string     `db:"rule_set_id" json:"rule_set_id"`
	VersionNumber  int64      `db:"version_number" json:"version_number"`
	Status         string     `db:"status" json:"status"`
	DefinitionJSON string     `db:"definition_json" json:"definition_json"`
	Checksum       string     `db:"checksum" json:"checksum"`
	IdempotencyKey string     `db:"idempotency_key" json:"-"`
	PublishedAt    *time.Time `db:"published_at" json:"published_at,omitempty"`
	Audit
}

type Page[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

type EvaluationInput struct {
	TenantID      string
	RuleSetID     string
	RuleSetCode   string
	VersionNumber int64
	FactsJSON     string
}

type BatchEvaluationResult struct {
	Index      int
	Evaluation Evaluation
	Version    RuleVersion
	Err        error
}

type OutboxEvent struct {
	ID          string
	Subject     string
	Envelope    []byte
	AvailableAt time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CreatedBy   string
	UpdatedBy   string
}
