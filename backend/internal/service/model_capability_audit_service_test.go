package service

import "testing"

func TestDeriveAuditVerdictDoesNotInferIdentityFromCapability(t *testing.T) {
	row := ModelCapabilityAuditRow{
		RequestedModel: "public-alias",
		MappedModel:    "upstream-model",
		Protocols: map[string]ModelCapabilityAuditEvidence{
			ModelCapabilityProtocolResponsesTools: {
				Status:             "verified",
				Capability:         "responses_tools",
				ToolCallFound:      true,
				ToolArgumentsValid: true,
			},
		},
	}

	deriveAuditVerdict(&row)

	if row.CapabilityStatus != "verified" {
		t.Fatalf("capability status = %q, want verified", row.CapabilityStatus)
	}
	if row.IdentityStatus != "evidence_only" {
		t.Fatalf("identity status = %q, want evidence_only without response model", row.IdentityStatus)
	}
}

func TestDeriveAuditVerdictRequiresMappedModelResponseForConsistency(t *testing.T) {
	tests := []struct {
		name           string
		responseModel  string
		wantIdentity   string
		wantConfidence string
	}{
		{name: "matching mapped model", responseModel: "upstream-model", wantIdentity: "consistent", wantConfidence: "high"},
		{name: "requested alias only", responseModel: "public-alias", wantIdentity: "mismatch_suspected", wantConfidence: "high"},
		{name: "different model", responseModel: "other-model", wantIdentity: "mismatch_suspected", wantConfidence: "high"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := ModelCapabilityAuditRow{
				RequestedModel: "public-alias",
				MappedModel:    "upstream-model",
				Protocols: map[string]ModelCapabilityAuditEvidence{
					ModelCapabilityProtocolResponses: {
						Status:        "verified",
						ResponseModel: tt.responseModel,
					},
				},
			}

			deriveAuditVerdict(&row)

			if row.IdentityStatus != tt.wantIdentity {
				t.Fatalf("identity status = %q, want %q", row.IdentityStatus, tt.wantIdentity)
			}
			if row.Confidence != tt.wantConfidence {
				t.Fatalf("confidence = %q, want %q", row.Confidence, tt.wantConfidence)
			}
		})
	}
}

func TestDeriveAuditVerdictPassiveMismatchRemainsSuspected(t *testing.T) {
	row := ModelCapabilityAuditRow{
		RequestedModel:    "public-alias",
		MappedModel:       "upstream-model",
		PassiveRequests:   3,
		PassiveMismatches: 1,
		PassiveResponseModels: []string{
			"upstream-model",
		},
	}

	deriveAuditVerdict(&row)

	if row.IdentityStatus != "mismatch_suspected" {
		t.Fatalf("identity status = %q, want mismatch_suspected", row.IdentityStatus)
	}
}
