package rule

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCompileDefinitionAndEvaluate(t *testing.T) {
	t.Parallel()

	raw := `{
      "rules": [
        {"name":"vip","condition":"facts.tier == 'vip' && facts.amount >= 100","result":{"discount":20}},
        {"name":"large","condition":"facts.amount >= 100","result":{"discount":10}}
      ],
      "default_result":{"discount":0}
    }`
	definition, issues := CompileDefinition(raw)
	if len(issues) != 0 {
		t.Fatalf("compile issues: %+v", issues)
	}
	if definition.Checksum() == "" || definition.CanonicalJSON() == "" {
		t.Fatal("compiled definition must expose canonical JSON and checksum")
	}

	tests := []struct {
		name        string
		facts       string
		matched     bool
		matchedRule string
		result      string
	}{
		{name: "first matching rule wins", facts: `{"tier":"vip","amount":120}`, matched: true, matchedRule: "vip", result: `{"discount":20}`},
		{name: "later rule can match", facts: `{"tier":"basic","amount":120}`, matched: true, matchedRule: "large", result: `{"discount":10}`},
		{name: "default result", facts: `{"tier":"basic","amount":10}`, result: `{"discount":0}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := definition.Evaluate(context.Background(), test.facts)
			if err != nil {
				t.Fatalf("evaluate: %v", err)
			}
			if got.Matched != test.matched || got.MatchedRule != test.matchedRule || got.ResultJSON != test.result {
				t.Fatalf("evaluation = %+v, want matched=%v rule=%q result=%s", got, test.matched, test.matchedRule, test.result)
			}
		})
	}
}

func TestCompileDefinitionRejectsUnsafeOrInvalidDefinitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		code string
	}{
		{name: "unknown field", raw: `{"rules":[],"default_result":{},"script":"x"}`, code: "invalid_json"},
		{name: "duplicate names", raw: `{"rules":[{"name":"x","condition":"true","result":{}},{"name":"x","condition":"false","result":{}}],"default_result":{}}`, code: "duplicate"},
		{name: "non boolean condition", raw: `{"rules":[{"name":"x","condition":"facts.amount","result":{}}],"default_result":{}}`, code: "invalid_result_type"},
		{name: "invalid expression", raw: `{"rules":[{"name":"x","condition":"facts.","result":{}}],"default_result":{}}`, code: "invalid_expression"},
		{name: "result must be object", raw: `{"rules":[{"name":"x","condition":"true","result":1}],"default_result":{}}`, code: "invalid_result"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, issues := CompileDefinition(test.raw)
			for _, issue := range issues {
				if issue.Code == test.code {
					return
				}
			}
			t.Fatalf("issues = %+v, want code %q", issues, test.code)
		})
	}
}

func TestCompiledDefinitionEvaluateRejectsInvalidFactsAndCancellation(t *testing.T) {
	t.Parallel()

	definition, issues := CompileDefinition(`{"rules":[{"name":"always","condition":"true","result":{}}],"default_result":{}}`)
	if len(issues) != 0 {
		t.Fatalf("compile issues: %+v", issues)
	}
	if _, err := definition.Evaluate(context.Background(), `[]`); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("invalid facts error = %v", err)
	}
	if _, err := definition.Evaluate(context.Background(), strings.Repeat("x", maxFactsBytes+1)); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("oversized facts error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := definition.Evaluate(ctx, `{}`); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled error = %v, want context canceled", err)
	}
}
