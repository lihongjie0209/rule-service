package grpctransport

import (
	"testing"
	"time"

	platformauthz "github.com/lihongjie0209/microservice-platform-go/authz"
	platformprincipal "github.com/lihongjie0209/microservice-platform-go/principal"
	rulev1 "github.com/lihongjie0209/platform-protos/gen/go/platform/rule/v1"
	"github.com/lihongjie0209/rule-service/internal/auth"
	"github.com/lihongjie0209/rule-service/internal/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestRuleGRPCRequirementCoversEveryBusinessMethod(t *testing.T) {
	t.Parallel()
	resolve := ruleGRPCRequirement(true)
	methods := []string{rulev1.RuleService_CreateRuleSet_FullMethodName, rulev1.RuleService_UpdateRuleSet_FullMethodName, rulev1.RuleService_GetRuleSet_FullMethodName, rulev1.RuleService_ListRuleSets_FullMethodName, rulev1.RuleService_CreateRuleVersion_FullMethodName, rulev1.RuleService_ValidateRuleVersion_FullMethodName, rulev1.RuleService_PublishRuleVersion_FullMethodName, rulev1.RuleService_ListRuleVersions_FullMethodName, rulev1.RuleService_Evaluate_FullMethodName, rulev1.RuleService_BatchEvaluate_FullMethodName}
	for _, method := range methods {
		requirement, ok := resolve(method)
		if !ok || requirement.Resource == "" || requirement.Action == "" || requirement.Scope != platformauthz.ScopePrincipal {
			t.Fatalf("method %q requirement = %+v, %v", method, requirement, ok)
		}
	}
	if _, ok := ruleGRPCRequirement(false)(rulev1.RuleService_Evaluate_FullMethodName); ok {
		t.Fatal("disabled authorization must not enforce")
	}
}

func TestAuthenticateGRPC_PSKWildcard(t *testing.T) {
	t.Parallel()
	const key = "01234567890123456789012345678901"
	authService := auth.New(config.Config{JWT: config.JWT{Issuer: "test", Secret: key, TTL: time.Hour}})
	cfg := config.Auth{
		SkipGRPCMethods: []string{"/platform.rule.v1.RuleService/*"},
		PSK:             config.PSK{Enabled: true, Key: key, GRPCMethods: []string{"/platform.rule.v1.RuleService/*"}},
	}
	for _, test := range []struct {
		name   string
		header string
		code   codes.Code
	}{
		{name: "valid", header: "PSK " + key, code: codes.OK},
		{name: "PSK precedes skip", code: codes.Unauthenticated},
		{name: "bearer rejected", header: "Bearer " + key, code: codes.Unauthenticated},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs("authorization", test.header))
			authenticated, err := authenticateGRPC(ctx, "/platform.rule.v1.RuleService/Evaluate", authService, cfg)
			if got := status.Code(err); got != test.code {
				t.Fatalf("status code = %s, want %s", got, test.code)
			}
			if test.code == codes.OK {
				value, ok := platformprincipal.FromContext(authenticated)
				if !ok || value.ID != "rule-service:psk" || value.Type != platformprincipal.TypeServiceAccount {
					t.Fatalf("principal = %#v, %v", value, ok)
				}
			}
		})
	}
}

func TestAuthenticateGRPC_JWTInjectsPrincipal(t *testing.T) {
	t.Parallel()
	const key = "01234567890123456789012345678901"
	service := auth.New(config.Config{JWT: config.JWT{Issuer: "test", Secret: key, TTL: time.Hour}, Auth: config.Auth{ClientID: "client", ClientSecret: "secret"}})
	token, err := service.Issue("user-1")
	if err != nil {
		t.Fatal(err)
	}
	ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs("authorization", "Bearer "+token))
	ctx, err = authenticateGRPC(ctx, "/platform.rule.v1.RuleService/Evaluate", service, config.Auth{})
	if err != nil {
		t.Fatal(err)
	}
	value, ok := platformprincipal.FromContext(ctx)
	if !ok || value.ID != "user-1" || value.Type != platformprincipal.TypeUser {
		t.Fatalf("principal = %#v, %v", value, ok)
	}
}
