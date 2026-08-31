package rule

import (
	rulev1 "github.com/lihongjie0209/platform-protos/gen/go/platform/rule/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func ToProtoRuleSet(value RuleSet) *rulev1.RuleSet {
	return &rulev1.RuleSet{Id: value.ID, TenantId: value.TenantID, Code: value.Code, Name: value.Name, Description: value.Description, Status: value.Status, PublishedVersionNumber: value.PublishedVersionNumber, Version: value.Version, CreatedAt: timestamppb.New(value.CreatedAt), UpdatedAt: timestamppb.New(value.UpdatedAt), CreatedBy: value.CreatedBy, UpdatedBy: value.UpdatedBy}
}

func ToProtoRuleVersion(value RuleVersion) *rulev1.RuleVersion {
	result := &rulev1.RuleVersion{Id: value.ID, TenantId: value.TenantID, RuleSetId: value.RuleSetID, VersionNumber: value.VersionNumber, Status: value.Status, DefinitionJson: value.DefinitionJSON, Checksum: value.Checksum, Version: value.Version, CreatedAt: timestamppb.New(value.CreatedAt), UpdatedAt: timestamppb.New(value.UpdatedAt), CreatedBy: value.CreatedBy, UpdatedBy: value.UpdatedBy}
	if value.PublishedAt != nil {
		result.PublishedAt = timestamppb.New(*value.PublishedAt)
	}
	return result
}
