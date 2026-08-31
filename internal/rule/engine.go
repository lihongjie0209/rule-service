package rule

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
)

const (
	maxDefinitionBytes = 256 << 10
	maxFactsBytes      = 256 << 10
	maxRules           = 100
	maxExpressionBytes = 4096
	maxEvaluationCost  = 100_000
)

var ErrInvalidDefinition = errors.New("invalid rule definition")

type Definition struct {
	Rules         []DefinitionRule `json:"rules"`
	DefaultResult json.RawMessage  `json:"default_result"`
}

type DefinitionRule struct {
	Name      string          `json:"name"`
	Condition string          `json:"condition"`
	Result    json.RawMessage `json:"result"`
}

type ValidationIssue struct {
	Path    string
	Code    string
	Message string
}

type Evaluation struct {
	Matched     bool
	MatchedRule string
	ResultJSON  string
}

type compiledRule struct {
	name    string
	program cel.Program
	result  string
}

type CompiledDefinition struct {
	rules         []compiledRule
	defaultResult string
	canonicalJSON string
	checksum      string
}

func CompileDefinition(raw string) (*CompiledDefinition, []ValidationIssue) {
	if len(raw) == 0 || len(raw) > maxDefinitionBytes {
		return nil, []ValidationIssue{{Path: "$", Code: "invalid_size", Message: "definition must be between 1 byte and 256 KiB"}}
	}

	var definition Definition
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&definition); err != nil {
		return nil, []ValidationIssue{{Path: "$", Code: "invalid_json", Message: err.Error()}}
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, []ValidationIssue{{Path: "$", Code: "multiple_values", Message: "definition must contain one JSON value"}}
	}

	issues := validateShape(definition)
	if len(issues) > 0 {
		return nil, issues
	}

	environment, err := cel.NewEnv(cel.Variable("facts", cel.DynType))
	if err != nil {
		return nil, []ValidationIssue{{Path: "$", Code: "engine_error", Message: "initialize CEL environment"}}
	}
	compiled := &CompiledDefinition{rules: make([]compiledRule, 0, len(definition.Rules))}
	for index, item := range definition.Rules {
		ast, compileIssues := environment.Compile(item.Condition)
		if compileIssues != nil && compileIssues.Err() != nil {
			issues = append(issues, ValidationIssue{Path: fmt.Sprintf("$.rules[%d].condition", index), Code: "invalid_expression", Message: compileIssues.Err().Error()})
			continue
		}
		if ast.OutputType() != cel.BoolType {
			issues = append(issues, ValidationIssue{Path: fmt.Sprintf("$.rules[%d].condition", index), Code: "invalid_result_type", Message: "condition must return bool"})
			continue
		}
		program, programErr := environment.Program(ast, cel.CostLimit(maxEvaluationCost), cel.InterruptCheckFrequency(100))
		if programErr != nil {
			issues = append(issues, ValidationIssue{Path: fmt.Sprintf("$.rules[%d].condition", index), Code: "invalid_program", Message: programErr.Error()})
			continue
		}
		compiled.rules = append(compiled.rules, compiledRule{name: item.Name, program: program, result: canonicalJSON(item.Result)})
	}
	if len(issues) > 0 {
		return nil, issues
	}

	canonical, err := json.Marshal(definition)
	if err != nil {
		return nil, []ValidationIssue{{Path: "$", Code: "invalid_json", Message: err.Error()}}
	}
	compiled.canonicalJSON = string(canonical)
	compiled.defaultResult = canonicalJSON(definition.DefaultResult)
	sum := sha256.Sum256(canonical)
	compiled.checksum = hex.EncodeToString(sum[:])
	return compiled, nil
}

func validateShape(definition Definition) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	if len(definition.Rules) == 0 || len(definition.Rules) > maxRules {
		issues = append(issues, ValidationIssue{Path: "$.rules", Code: "invalid_count", Message: "rules must contain between 1 and 100 items"})
	}
	seen := make(map[string]struct{}, len(definition.Rules))
	for index, item := range definition.Rules {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			issues = append(issues, ValidationIssue{Path: fmt.Sprintf("$.rules[%d].name", index), Code: "required", Message: "name is required"})
		} else if _, exists := seen[name]; exists {
			issues = append(issues, ValidationIssue{Path: fmt.Sprintf("$.rules[%d].name", index), Code: "duplicate", Message: "rule names must be unique"})
		}
		seen[name] = struct{}{}
		if strings.TrimSpace(item.Condition) == "" || len(item.Condition) > maxExpressionBytes {
			issues = append(issues, ValidationIssue{Path: fmt.Sprintf("$.rules[%d].condition", index), Code: "invalid_size", Message: "condition must be between 1 and 4096 bytes"})
		}
		if !validJSONObject(item.Result) {
			issues = append(issues, ValidationIssue{Path: fmt.Sprintf("$.rules[%d].result", index), Code: "invalid_result", Message: "result must be a JSON object"})
		}
	}
	if !validJSONObject(definition.DefaultResult) {
		issues = append(issues, ValidationIssue{Path: "$.default_result", Code: "invalid_result", Message: "default_result must be a JSON object"})
	}
	return issues
}

func validJSONObject(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var value map[string]any
	return json.Unmarshal(raw, &value) == nil && value != nil
}

func canonicalJSON(raw json.RawMessage) string {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "{}"
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(canonical)
}

func (definition *CompiledDefinition) CanonicalJSON() string { return definition.canonicalJSON }
func (definition *CompiledDefinition) Checksum() string      { return definition.checksum }

func (definition *CompiledDefinition) Evaluate(ctx context.Context, factsJSON string) (Evaluation, error) {
	if err := ctx.Err(); err != nil {
		return Evaluation{}, err
	}
	if len(factsJSON) == 0 || len(factsJSON) > maxFactsBytes {
		return Evaluation{}, fmt.Errorf("%w: facts must be between 1 byte and 256 KiB", ErrInvalidDefinition)
	}
	var facts map[string]any
	decoder := json.NewDecoder(strings.NewReader(factsJSON))
	if err := decoder.Decode(&facts); err != nil || facts == nil {
		return Evaluation{}, fmt.Errorf("%w: facts must be a JSON object", ErrInvalidDefinition)
	}
	for _, item := range definition.rules {
		value, _, err := item.program.ContextEval(ctx, map[string]any{"facts": facts})
		if err != nil {
			return Evaluation{}, fmt.Errorf("evaluate rule %q: %w", item.name, err)
		}
		matched, ok := value.(types.Bool)
		if !ok {
			return Evaluation{}, fmt.Errorf("evaluate rule %q: condition returned %T", item.name, value)
		}
		if bool(matched) {
			return Evaluation{Matched: true, MatchedRule: item.name, ResultJSON: item.result}, nil
		}
	}
	return Evaluation{ResultJSON: definition.defaultResult}, nil
}
