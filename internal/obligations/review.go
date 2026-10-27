package obligations

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tyler-beach/pf_2/internal/assessment"
)

const RegistrySchemaVersion = "1"

const (
	DecisionPending   = "pending_review"
	DecisionApproved = "approved"
	DecisionRejected = "rejected"
	DecisionSuperseded = "superseded"

	EnforcementBlocking = "blocking"
	EnforcementAdvisory = "advisory"
	EnforcementReviewRequired = "review_required"
)

// CandidateSnapshot preserves the exact interpretation that was reviewed.
// It is intentionally separate from the enforceable requirements catalog.
type CandidateSnapshot struct {
	ID              string `json:"id"`
	ADRID           string `json:"adr_id"`
	ADRTitle        string `json:"adr_title"`
	ADRStatus       string `json:"adr_status,omitempty"`
	SectionHeading  string `json:"section_heading"`
	Statement       string `json:"statement"`
	Modality        string `json:"modality"`
	Kind            string `json:"kind"`
	PatternHint     string `json:"pattern_hint"`
	SourcePath      string `json:"source_path"`
	SourceSHA256    string `json:"source_sha256"`
	SourceLineStart int    `json:"source_line_start"`
	SourceLineEnd   int    `json:"source_line_end"`
	ReviewHints     []string `json:"review_hints,omitempty"`
}

// ReviewRecord is an append-only, revisioned human decision for a candidate.
// A record never changes the active requirements catalog by itself.
type ReviewRecord struct {
	CandidateID     string            `json:"candidate_id"`
	Revision        int               `json:"revision"`
	Decision        string            `json:"decision"`
	Candidate       CandidateSnapshot `json:"candidate"`
	RequirementID   string            `json:"requirement_id,omitempty"`
	PatternID       string            `json:"pattern_id,omitempty"`
	EnforcementMode string            `json:"enforcement_mode"`
	Reviewer        string            `json:"reviewer"`
	ReviewedAt      string            `json:"reviewed_at"`
	ReviewReference string            `json:"review_reference"`
	Rationale       string            `json:"rationale"`
}

// Registry retains all historical decisions, including the source snapshot
// and digest that the reviewer actually saw.
type Registry struct {
	SchemaVersion string         `json:"schema_version"`
	Records       []ReviewRecord `json:"records"`
}

type ReviewProposal struct {
	CandidateID     string `json:"candidate_id"`
	Decision        string `json:"decision"`
	RequirementID   string `json:"requirement_id,omitempty"`
	PatternID       string `json:"pattern_id,omitempty"`
	EnforcementMode string `json:"enforcement_mode"`
	Reviewer        string `json:"reviewer"`
	ReviewedAt      string `json:"reviewed_at"`
	ReviewReference string `json:"review_reference"`
	Rationale       string `json:"rationale"`
}

// BindingCoverage reports candidate-review state separately from code evidence
// coverage. A registry can have zero blockers while every candidate is pending.
type BindingCoverage struct {
	SchemaVersion        string `json:"schema_version"`
	CandidateIndexSHA256 string `json:"candidate_index_sha256"`
	CandidateCount       int    `json:"candidate_count"`
	PendingReviewCount   int    `json:"pending_review_count"`
	ApprovedCount        int    `json:"approved_count"`
	RejectedCount        int    `json:"rejected_count"`
	SupersededCount      int    `json:"superseded_count"`
	StaleCount           int    `json:"stale_count"`
	OrphanedCount        int    `json:"orphaned_count"`
	BlockingBindingCount int    `json:"blocking_binding_count"`
	AdvisoryBindingCount int    `json:"advisory_binding_count"`
	ReviewRequiredCount  int    `json:"review_required_count"`
	UnboundCount         int    `json:"unbound_count"`
}

// NewRegistry returns an empty versioned audit log. Candidates without a record
// are implicitly pending review and are counted by ValidateRegistry.
func NewRegistry() Registry {
	return Registry{SchemaVersion: RegistrySchemaVersion, Records: []ReviewRecord{}}
}

// SnapshotCandidate copies the review-relevant source and interpretation fields.
func SnapshotCandidate(c Candidate) CandidateSnapshot {
	return CandidateSnapshot{
		ID: c.ID, ADRID: c.ADRID, ADRTitle: c.ADRTitle, ADRStatus: c.ADRStatus,
		SectionHeading: c.SectionHeading, Statement: c.Statement, Modality: c.Modality,
		Kind: c.Kind, PatternHint: c.PatternHint, SourcePath: c.SourcePath,
		SourceSHA256: c.SourceSHA256, SourceLineStart: c.SourceLineStart, SourceLineEnd: c.SourceLineEnd,
		ReviewHints: append([]string(nil), c.ReviewHints...),
	}
}

// RecordReview appends a new revision. If a candidate disappeared from the
// latest index, the only permitted new decision for it is superseded; this
// preserves an auditable reason for retaining historical candidate records.
func RecordReview(index Index, catalog assessment.Catalog, registry Registry, proposal ReviewProposal) (Registry, BindingCoverage, error) {
	if registry.SchemaVersion == "" {
		registry = NewRegistry()
	}
	if registry.Records == nil {
		registry.Records = []ReviewRecord{}
	}
	if err := validateRegistryStructure(registry); err != nil {
		return registry, BindingCoverage{}, fmt.Errorf("validate existing review registry: %w", err)
	}
	if err := validateCandidateIndex(index); err != nil {
		return registry, BindingCoverage{}, err
	}
	if err := validateCatalogForReview(catalog); err != nil {
		return registry, BindingCoverage{}, err
	}
	proposal.CandidateID = strings.TrimSpace(proposal.CandidateID)
	if proposal.CandidateID == "" {
		return registry, BindingCoverage{}, fmt.Errorf("candidate ID is required")
	}
	candidate, found, err := findCandidate(index, proposal.CandidateID)
	if err != nil {
		return registry, BindingCoverage{}, err
	}
	latest := latestRecords(registry)
	var snapshot CandidateSnapshot
	if found {
		if proposal.Decision == DecisionSuperseded {
			return registry, BindingCoverage{}, fmt.Errorf("candidate %s is still present in the current index; supersede it only after it is removed or replaced", proposal.CandidateID)
		}
		snapshot = SnapshotCandidate(candidate)
	} else {
		if proposal.Decision != DecisionSuperseded {
			return registry, BindingCoverage{}, fmt.Errorf("candidate %s is not in the current index; only a superseded decision can close its review history", proposal.CandidateID)
		}
		prior, ok := latest[proposal.CandidateID]
		if !ok {
			return registry, BindingCoverage{}, fmt.Errorf("cannot supersede candidate %s without review history", proposal.CandidateID)
		}
		snapshot = prior.Candidate
	}
	if proposal.EnforcementMode == "" {
		proposal.EnforcementMode = EnforcementReviewRequired
	}
	if proposal.Decision == DecisionApproved && proposal.PatternID == "" && found {
		proposal.PatternID = candidate.PatternHint
	}
	record := ReviewRecord{
		CandidateID: proposal.CandidateID,
		Revision: 1,
		Decision: strings.TrimSpace(proposal.Decision),
		Candidate: snapshot,
		RequirementID: strings.TrimSpace(proposal.RequirementID),
		PatternID: strings.TrimSpace(proposal.PatternID),
		EnforcementMode: strings.TrimSpace(proposal.EnforcementMode),
		Reviewer: strings.TrimSpace(proposal.Reviewer),
		ReviewedAt: strings.TrimSpace(proposal.ReviewedAt),
		ReviewReference: strings.TrimSpace(proposal.ReviewReference),
		Rationale: strings.TrimSpace(proposal.Rationale),
	}
	if previous, ok := latest[record.CandidateID]; ok {
		record.Revision = previous.Revision + 1
	}
	if err := validateReviewRecord(record); err != nil {
		return registry, BindingCoverage{}, err
	}
	if found && record.Decision == DecisionApproved {
		if err := validateApprovedBinding(record, candidate, catalog); err != nil {
			return registry, BindingCoverage{}, err
		}
	}
	next := Registry{SchemaVersion: RegistrySchemaVersion, Records: append(append([]ReviewRecord{}, registry.Records...), record)}
	coverage, err := ValidateRegistry(index, catalog, next)
	if err != nil {
		return registry, BindingCoverage{}, err
	}
	return next, coverage, nil
}

// ValidateRegistry checks history integrity and active bindings. Stale approvals
// and orphaned records are reported in coverage rather than treated as bindings;
// strict validation rejects them until they are re-reviewed or superseded.
func ValidateRegistry(index Index, catalog assessment.Catalog, registry Registry) (BindingCoverage, error) {
	coverage := BindingCoverage{
		SchemaVersion: "1",
		CandidateIndexSHA256: index.SourceIndexSHA256,
		CandidateCount: len(index.Candidates),
	}
	if err := validateCandidateIndex(index); err != nil {
		return coverage, err
	}
	if err := validateCatalogForReview(catalog); err != nil {
		return coverage, err
	}
	if err := validateRegistryStructure(registry); err != nil {
		return coverage, err
	}
	candidates := make(map[string]Candidate, len(index.Candidates))
	for _, candidate := range index.Candidates {
		candidates[candidate.ID] = candidate
	}
	latest := latestRecords(registry)
	seen := make(map[string]bool, len(latest))
	for id, candidate := range candidates {
		record, ok := latest[id]
		if !ok {
			coverage.PendingReviewCount++
			coverage.UnboundCount++
			continue
		}
		seen[id] = true
		if record.Decision == DecisionSuperseded || !snapshotsMatchCurrent(record.Candidate, candidate) {
			coverage.StaleCount++
			coverage.PendingReviewCount++
			coverage.UnboundCount++
			continue
		}
		switch record.Decision {
		case DecisionPending:
			coverage.PendingReviewCount++
			coverage.UnboundCount++
		case DecisionRejected:
			coverage.RejectedCount++
			coverage.UnboundCount++
		case DecisionApproved:
			if err := validateApprovedBinding(record, candidate, catalog); err != nil {
				return coverage, fmt.Errorf("active review record %s revision %d: %w", id, record.Revision, err)
			}
			coverage.ApprovedCount++
			if record.RequirementID == "" {
				coverage.UnboundCount++
				coverage.ReviewRequiredCount++
				continue
			}
			switch record.EnforcementMode {
			case EnforcementBlocking:
				coverage.BlockingBindingCount++
			case EnforcementAdvisory:
				coverage.AdvisoryBindingCount++
			case EnforcementReviewRequired:
				coverage.ReviewRequiredCount++
			}
		}
	}
	for id, record := range latest {
		if seen[id] {
			continue
		}
		if _, exists := candidates[id]; exists {
			continue
		}
		if record.Decision == DecisionSuperseded {
			coverage.SupersededCount++
		} else {
			coverage.OrphanedCount++
		}
	}
	return coverage, nil
}

func ValidateActiveRegistry(index Index, catalog assessment.Catalog, registry Registry) (BindingCoverage, error) {
	coverage, err := ValidateRegistry(index, catalog, registry)
	if err != nil {
		return coverage, err
	}
	if coverage.StaleCount != 0 || coverage.OrphanedCount != 0 {
		return coverage, fmt.Errorf("review registry has %d stale candidate decision(s) and %d orphaned candidate history record(s)", coverage.StaleCount, coverage.OrphanedCount)
	}
	return coverage, nil
}

func validateCandidateIndex(index Index) error {
	if index.SchemaVersion != "1" {
		return fmt.Errorf("unsupported candidate index schema_version %q", index.SchemaVersion)
	}
	if !validSHA256(index.SourceIndexSHA256) {
		return fmt.Errorf("candidate index has invalid source_index_sha256")
	}
	if strings.TrimSpace(index.SourceRoot) == "" {
		return fmt.Errorf("candidate index source_root is required")
	}
	if index.CandidateCount != len(index.Candidates) {
		return fmt.Errorf("candidate index candidate_count=%d but contains %d candidates", index.CandidateCount, len(index.Candidates))
	}
	seen := make(map[string]bool, len(index.Candidates))
	for _, candidate := range index.Candidates {
		if seen[candidate.ID] {
			return fmt.Errorf("duplicate candidate ID %q", candidate.ID)
		}
		seen[candidate.ID] = true
		if err := validateCandidateSnapshot(SnapshotCandidate(candidate)); err != nil {
			return fmt.Errorf("candidate %s: %w", candidate.ID, err)
		}
		if candidate.ApprovalStatus != DecisionPending || candidate.EnforcementMode != EnforcementReviewRequired || candidate.Executable {
			return fmt.Errorf("candidate %s was made enforceable or altered from review-only defaults", candidate.ID)
		}
	}
	return nil
}

func validateCatalogForReview(catalog assessment.Catalog) error {
	if err := assessment.ValidateCatalog(catalog); err != nil {
		return fmt.Errorf("validate requirements catalog: %w", err)
	}
	return nil
}

func findCandidate(index Index, id string) (Candidate, bool, error) {
	var found Candidate
	count := 0
	for _, candidate := range index.Candidates {
		if candidate.ID == id {
			found = candidate
			count++
		}
	}
	if count > 1 {
		return Candidate{}, false, fmt.Errorf("candidate ID %s is not unique in the candidate index", id)
	}
	return found, count == 1, nil
}

func latestRecords(registry Registry) map[string]ReviewRecord {
	latest := make(map[string]ReviewRecord)
	for _, record := range registry.Records {
		latest[record.CandidateID] = record
	}
	return latest
}

func validateRegistryStructure(registry Registry) error {
	if registry.SchemaVersion != RegistrySchemaVersion {
		return fmt.Errorf("unsupported registry schema_version %q (expected %s)", registry.SchemaVersion, RegistrySchemaVersion)
	}
	expectedRevision := make(map[string]int)
	for i, record := range registry.Records {
		if err := validateReviewRecord(record); err != nil {
			return fmt.Errorf("record %d: %w", i+1, err)
		}
		expectedRevision[record.CandidateID]++
		if record.Revision != expectedRevision[record.CandidateID] {
			return fmt.Errorf("candidate %s has revision %d at record %d; expected contiguous revision %d", record.CandidateID, record.Revision, i+1, expectedRevision[record.CandidateID])
		}
	}
	return nil
}

func validateReviewRecord(record ReviewRecord) error {
	if record.CandidateID == "" || record.Candidate.ID != record.CandidateID {
		return fmt.Errorf("candidate_id must match the recorded candidate snapshot ID")
	}
	if record.Revision < 1 {
		return fmt.Errorf("revision must be >= 1")
	}
	if err := validateCandidateSnapshot(record.Candidate); err != nil {
		return fmt.Errorf("candidate snapshot: %w", err)
	}
	if record.Decision != DecisionPending && record.Decision != DecisionApproved && record.Decision != DecisionRejected && record.Decision != DecisionSuperseded {
		return fmt.Errorf("invalid decision %q", record.Decision)
	}
	if record.EnforcementMode != EnforcementBlocking && record.EnforcementMode != EnforcementAdvisory && record.EnforcementMode != EnforcementReviewRequired {
		return fmt.Errorf("invalid enforcement_mode %q", record.EnforcementMode)
	}
	if record.EnforcementMode != EnforcementReviewRequired && record.Decision != DecisionApproved {
		return fmt.Errorf("only an approved decision may select a blocking or advisory enforcement mode")
	}
	if strings.TrimSpace(record.Reviewer) == "" {
		return fmt.Errorf("reviewer is required for each recorded decision")
	}
	if strings.TrimSpace(record.Rationale) == "" {
		return fmt.Errorf("rationale is required for each recorded decision")
	}
	if len(record.Reviewer) > 256 || len(record.Rationale) > 4000 {
		return fmt.Errorf("reviewer or rationale exceeds the supported length")
	}
	parsedTime, err := time.Parse(time.RFC3339, record.ReviewedAt)
	if err != nil || parsedTime.IsZero() {
		return fmt.Errorf("reviewed_at must be an RFC3339 timestamp")
	}
	ref, err := url.ParseRequestURI(record.ReviewReference)
	if err != nil || ref.Scheme != "https" || ref.Hostname() == "" || ref.User != nil {
		return fmt.Errorf("review_reference must be an HTTPS URL without embedded credentials")
	}
	switch record.Decision {
	case DecisionPending, DecisionRejected, DecisionSuperseded:
		if record.RequirementID != "" || record.PatternID != "" {
			return fmt.Errorf("%s decisions must not claim a requirement or pattern binding", record.Decision)
		}
		if record.EnforcementMode != EnforcementReviewRequired {
			return fmt.Errorf("%s decisions require enforcement_mode %q", record.Decision, EnforcementReviewRequired)
		}
	case DecisionApproved:
		if record.PatternID == "" {
			return fmt.Errorf("approved decisions require pattern_id")
		}
		if !validPattern(record.PatternID) {
			return fmt.Errorf("unsupported pattern_id %q", record.PatternID)
		}
		if record.PatternID == "manual_review" {
			if record.RequirementID != "" || record.EnforcementMode != EnforcementReviewRequired {
				return fmt.Errorf("manual_review candidates cannot bind a requirement or become blocking/advisory")
			}
		} else if record.RequirementID == "" {
			return fmt.Errorf("approved machine-checkable decisions require requirement_id")
		}
	}
	return nil
}

func validateCandidateSnapshot(candidate CandidateSnapshot) error {
	if candidate.ID == "" || candidate.ADRID == "" || candidate.ADRTitle == "" || candidate.SectionHeading == "" || candidate.Statement == "" {
		return fmt.Errorf("ID, ADR ID/title, section heading, and statement are required")
	}
	if !validSHA256(candidate.SourceSHA256) {
		return fmt.Errorf("source_sha256 must be a 64-character hexadecimal SHA-256")
	}
	if candidate.SourceLineStart < 1 || candidate.SourceLineEnd < candidate.SourceLineStart {
		return fmt.Errorf("source line range is invalid")
	}
	clean, err := cleanRelativePath(candidate.SourcePath)
	if err != nil || clean != candidate.SourcePath {
		return fmt.Errorf("source_path must be a clean repository-relative path")
	}
	if candidate.ID != candidateID(Candidate{
		ADRID: candidate.ADRID, SourcePath: candidate.SourcePath,
		SectionHeading: candidate.SectionHeading, SourceLineStart: candidate.SourceLineStart,
		Statement: candidate.Statement,
	}) {
		return fmt.Errorf("candidate ID does not match its stable source identity")
	}
	if !validPattern(candidate.PatternHint) {
		return fmt.Errorf("unsupported candidate pattern_hint %q", candidate.PatternHint)
	}
	allowedHints := map[string]bool{"condition_or_exception": true, "compound_or_conjoined": true, "conditional_scope": true, "subjective_or_underspecified": true}
	seenHints := make(map[string]bool, len(candidate.ReviewHints))
	for _, hint := range candidate.ReviewHints {
		if !allowedHints[hint] || seenHints[hint] { return fmt.Errorf("unsupported or duplicate review hint %q", hint) }
		seenHints[hint] = true
	}
	return nil
}

func snapshotsMatchCurrent(snapshot CandidateSnapshot, candidate Candidate) bool {
	current := SnapshotCandidate(candidate)
	// Candidate IDs deliberately exclude the full-document hash so unchanged
	// statements can receive a new revision after an ADR is edited elsewhere.
	return snapshot.ID == current.ID &&
		snapshot.ADRID == current.ADRID &&
		snapshot.ADRTitle == current.ADRTitle &&
		snapshot.ADRStatus == current.ADRStatus &&
		snapshot.SectionHeading == current.SectionHeading &&
		snapshot.Statement == current.Statement &&
		snapshot.Modality == current.Modality &&
		snapshot.Kind == current.Kind &&
		snapshot.PatternHint == current.PatternHint &&
		snapshot.SourcePath == current.SourcePath &&
		snapshot.SourceSHA256 == current.SourceSHA256 &&
		snapshot.SourceLineStart == current.SourceLineStart &&
		snapshot.SourceLineEnd == current.SourceLineEnd &&
		strings.Join(snapshot.ReviewHints, "\x00") == strings.Join(current.ReviewHints, "\x00")
}

func validPattern(pattern string) bool {
	switch pattern {
	case "configuration.bounds_or_required_values", "security.source_pattern", "architecture.dependency_or_boundary", "manual_review":
		return true
	default:
		return false
	}
}

func validateApprovedBinding(record ReviewRecord, candidate Candidate, catalog assessment.Catalog) error {
	if record.CandidateID != candidate.ID || record.Candidate.SourceSHA256 != candidate.SourceSHA256 {
		return fmt.Errorf("approved binding is not tied to the current candidate source SHA-256")
	}
	if record.PatternID == "manual_review" {
		if record.RequirementID != "" || record.EnforcementMode != EnforcementReviewRequired {
			return fmt.Errorf("manual_review candidates cannot block CI or bind a verifier")
		}
		return nil
	}
	if record.EnforcementMode == EnforcementBlocking && strings.ToLower(strings.TrimSpace(candidate.ADRStatus)) != "accepted" {
		return fmt.Errorf("only a candidate from an accepted ADR may have blocking enforcement")
	}
	var matched *assessment.Requirement
	var owningADR string
	for ai := range catalog.ADRs {
		for ri := range catalog.ADRs[ai].Requirements {
			if catalog.ADRs[ai].Requirements[ri].ID == record.RequirementID {
				matched = &catalog.ADRs[ai].Requirements[ri]
				owningADR = catalog.ADRs[ai].ID
				if catalog.ADRs[ai].Status != "accepted" {
					return fmt.Errorf("requirement %s is not in an accepted ADR", record.RequirementID)
				}
				break
			}
		}
		if matched != nil { break }
	}
	if matched == nil {
		return fmt.Errorf("requirement %s does not exist in the active requirements catalog", record.RequirementID)
	}
	if owningADR != candidate.ADRID {
		return fmt.Errorf("candidate ADR %s cannot be bound to requirement %s owned by ADR %s", candidate.ADRID, record.RequirementID, owningADR)
	}
	switch record.PatternID {
	case "configuration.bounds_or_required_values":
		if matched.Verifier.Type != "json-required-keys" && matched.Verifier.Type != "json-number-range" {
			return fmt.Errorf("configuration pattern requires json-required-keys or json-number-range verifier")
		}
	case "security.source_pattern":
		if matched.Verifier.Type != "analyzer-rule" {
			return fmt.Errorf("security.source_pattern requires an analyzer-rule verifier")
		}
	case "architecture.dependency_or_boundary":
		if matched.Verifier.Type != "go-import-boundary" && matched.Verifier.Type != "analyzer-rule" {
			return fmt.Errorf("architecture pattern requires go-import-boundary or analyzer-rule verifier")
		}
	default:
		return fmt.Errorf("unsupported approved pattern_id %q", record.PatternID)
	}
	return nil
}

// SaveRegistry writes the audit log atomically to avoid leaving a truncated
// registry if the process is interrupted during an update.
func SaveRegistry(path string, registry Registry) error {
	if err := validateRegistryStructure(registry); err != nil {
		return fmt.Errorf("refusing to save invalid review registry: %w", err)
	}
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil { return fmt.Errorf("encode review registry: %w", err) }
	absolute, err := filepath.Abs(path)
	if err != nil { return fmt.Errorf("resolve registry output path: %w", err) }
	dir := filepath.Dir(absolute)
	if err := os.MkdirAll(dir, 0755); err != nil { return fmt.Errorf("create registry output directory: %w", err) }
	temp, err := os.CreateTemp(dir, "."+filepath.Base(absolute)+".tmp-*")
	if err != nil { return fmt.Errorf("create temporary registry: %w", err) }
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0600); err != nil { temp.Close(); return fmt.Errorf("restrict registry permissions: %w", err) }
	if _, err := temp.Write(data); err != nil { temp.Close(); return fmt.Errorf("write temporary registry: %w", err) }
	if err := temp.Sync(); err != nil { temp.Close(); return fmt.Errorf("sync temporary registry: %w", err) }
	if err := temp.Close(); err != nil { return fmt.Errorf("close temporary registry: %w", err) }
	if err := os.Rename(tempName, absolute); err != nil { return fmt.Errorf("atomically replace registry: %w", err) }
	return nil
}

// LoadRegistry loads a versioned review log. A missing registry is not treated as
// valid by this reader; callers that intend to initialize one should use NewRegistry.
func LoadRegistry(path string) (Registry, error) {
	var registry Registry
	data, err := os.ReadFile(path)
	if err != nil { return registry, fmt.Errorf("read review registry: %w", err) }
	if err := json.Unmarshal(data, &registry); err != nil { return registry, fmt.Errorf("decode review registry: %w", err) }
	if registry.Records == nil { registry.Records = []ReviewRecord{} }
	if err := validateRegistryStructure(registry); err != nil { return registry, err }
	return registry, nil
}
