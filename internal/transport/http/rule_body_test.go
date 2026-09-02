package httptransport

import (
	"encoding/json"
	"testing"
)

func TestRuleSetDetailOmitsMissingPublishedVersion(t *testing.T) {
	t.Parallel()
	encoded, err := json.Marshal(RuleSetDetailBody{RuleSet: RuleSetView{ID: "set-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"rule_set":{"id":"set-1","tenant_id":"","application_id":"","code":"","name":"","description":"","status":"","published_version_number":0,"version":0,"created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z","created_by":"","updated_by":""}}` {
		t.Fatalf("response unexpectedly represents a missing published version: %s", encoded)
	}
}

func TestValidationIssuesUseStableJSONFields(t *testing.T) {
	t.Parallel()
	encoded, err := json.Marshal(ValidateRuleVersionBody{Issues: []ValidationIssueBody{{Path: "rules[0]", Code: "invalid", Message: "invalid condition"}}})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	issues, ok := payload["issues"].([]any)
	if !ok || len(issues) != 1 {
		t.Fatalf("issues = %#v", payload["issues"])
	}
	issue := issues[0].(map[string]any)
	if issue["path"] != "rules[0]" || issue["code"] != "invalid" || issue["message"] != "invalid condition" {
		t.Fatalf("issue = %#v", issue)
	}
}
