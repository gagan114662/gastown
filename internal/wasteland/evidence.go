package wasteland

import (
	"fmt"
	"net/url"
	"strings"
)

type EvidenceType string

const (
	EvidenceTypeGitHubPR     EvidenceType = "github_pr"
	EvidenceTypeGitHubCommit EvidenceType = "github_commit"
	EvidenceTypeDoltHub      EvidenceType = "dolthub"
	EvidenceTypeLink         EvidenceType = "link"
	EvidenceTypeManual       EvidenceType = "manual"
)

type ValidationStatus string

const (
	ValidationVerified    ValidationStatus = "verified"
	ValidationUnvalidated ValidationStatus = "unvalidated"
)

type EvidenceAssessment struct {
	Type   EvidenceType     `json:"type"`
	Status ValidationStatus `json:"status"`
	URL    string           `json:"url,omitempty"`
}

// AnalyzeEvidence classifies completion evidence and determines whether it can be auto-verified.
func AnalyzeEvidence(evidence string) (EvidenceAssessment, error) {
	evidence = strings.TrimSpace(evidence)
	if evidence == "" {
		return EvidenceAssessment{}, fmt.Errorf("evidence cannot be empty")
	}
	parsed, err := url.Parse(evidence)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return EvidenceAssessment{Type: EvidenceTypeManual, Status: ValidationUnvalidated}, nil
	}

	host := strings.ToLower(parsed.Host)
	path := strings.Trim(parsed.Path, "/")
	switch {
	case strings.Contains(host, "github.com") && strings.Contains(path, "/pull/"):
		return EvidenceAssessment{Type: EvidenceTypeGitHubPR, Status: ValidationVerified, URL: evidence}, nil
	case strings.Contains(host, "github.com") && strings.Contains(path, "/commit/"):
		return EvidenceAssessment{Type: EvidenceTypeGitHubCommit, Status: ValidationVerified, URL: evidence}, nil
	case strings.Contains(host, "dolthub.com"):
		return EvidenceAssessment{Type: EvidenceTypeDoltHub, Status: ValidationVerified, URL: evidence}, nil
	default:
		return EvidenceAssessment{Type: EvidenceTypeLink, Status: ValidationUnvalidated, URL: evidence}, nil
	}
}
