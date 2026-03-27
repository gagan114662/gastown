package wasteland

import "testing"

func TestAnalyzeEvidence(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		wantTy EvidenceType
		wantSt ValidationStatus
	}{
		{"github pr", "https://github.com/openai/gastown/pull/1", EvidenceTypeGitHubPR, ValidationVerified},
		{"github commit", "https://github.com/openai/gastown/commit/abcdef", EvidenceTypeGitHubCommit, ValidationVerified},
		{"dolthub", "https://www.dolthub.com/repositories/hop/wl-commons/pulls/1", EvidenceTypeDoltHub, ValidationVerified},
		{"generic link", "https://example.com/runbook", EvidenceTypeLink, ValidationUnvalidated},
		{"manual", "commit abc123", EvidenceTypeManual, ValidationUnvalidated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AnalyzeEvidence(tt.input)
			if err != nil {
				t.Fatalf("AnalyzeEvidence() error: %v", err)
			}
			if got.Type != tt.wantTy || got.Status != tt.wantSt {
				t.Fatalf("AnalyzeEvidence() = %+v, want type=%s status=%s", got, tt.wantTy, tt.wantSt)
			}
		})
	}
}
