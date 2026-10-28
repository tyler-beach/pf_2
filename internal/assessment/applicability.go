package assessment

import (
	"fmt"
	"path/filepath"
	"strings"
)

// AppliesTo declares when a requirement can produce signal for an artifact.
// Empty fields mean "derive conservative defaults from the verifier".
// Explicit metadata on the catalog / binding wins over defaults.
type AppliesTo struct {
	SubmissionKinds []string `json:"submission_kinds,omitempty"`
	PathGlobs       []string `json:"path_globs,omitempty"`
	Analyzers       []string `json:"analyzers,omitempty"`
}

// ApplicabilityReasonPrefix is recorded on NOT_APPLICABLE rows produced by the filter.
const ApplicabilityReasonPrefix = "Not applicable by applicability filter:"

// EffectiveAppliesTo returns catalog applies_to merged with verifier-derived defaults.
// Conservative: prefer false N/A over false FAIL only when the rule cannot fire.
func EffectiveAppliesTo(r Requirement) AppliesTo {
	at := AppliesTo{
		SubmissionKinds: append([]string(nil), r.AppliesTo.SubmissionKinds...),
		PathGlobs:       append([]string(nil), r.AppliesTo.PathGlobs...),
		Analyzers:       append([]string(nil), r.AppliesTo.Analyzers...),
	}
	if len(at.SubmissionKinds) == 0 {
		switch r.Verifier.Type {
		case "manual-review":
			// Manual obligations may appear on prose or code surfaces; IssueSurface
			// still flips undecided manuals to NOT_APPLICABLE on sandboxes.
			at.SubmissionKinds = []string{string(SubmissionKindTechSpec), string(SubmissionKindCodeSnippet)}
		default:
			at.SubmissionKinds = []string{string(SubmissionKindCodeSnippet)}
		}
	}
	if len(at.PathGlobs) == 0 {
		at.PathGlobs = derivePathGlobs(r.Verifier)
	}
	if len(at.Analyzers) == 0 && r.Verifier.Type == "analyzer-rule" {
		if name := strings.TrimSpace(r.Verifier.Analyzer); name != "" {
			at.Analyzers = []string{name}
		}
	}
	return at
}

func derivePathGlobs(v Verifier) []string {
	var globs []string
	if fp := strings.TrimSpace(v.FilePath); fp != "" {
		globs = append(globs, filepath.ToSlash(fp))
	}
	for _, prefix := range v.SourcePathPrefixes {
		p := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(prefix))))
		p = strings.TrimPrefix(p, "./")
		if p == "" || p == "." {
			continue
		}
		p = strings.TrimSuffix(p, "/")
		globs = append(globs, p+"/**")
	}
	for _, ext := range v.SourceExtensions {
		ext = strings.ToLower(strings.TrimSpace(ext))
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		globs = append(globs, "**/*"+ext)
	}
	return globs
}

// CheckApplicability returns false when the requirement cannot apply to this
// assessment surface. AssessAll short-circuits to applicable.
//
// Grill: never upgrade missing evidence to N/A. Submission-mode go-import/json
// verifiers still emit EVIDENCE_MISSING when scope files are absent. Path/extension
// filtering is for analyzer-rule (and assess --scope intersection) where the rule
// provably cannot fire on the supplied artifact.
func CheckApplicability(r Requirement, files []SourceFile, options AssessmentOptions) (applicable bool, reason string) {
	if options.AssessAll {
		return true, ""
	}
	at := EffectiveAppliesTo(r)

	if kind := options.SubmissionKind; kind != SubmissionKindUnspecified {
		if !kindAllowed(kind, at.SubmissionKinds) {
			return false, fmt.Sprintf(
				"%s submission_kind %q is outside applies_to.submission_kinds %v",
				ApplicabilityReasonPrefix, kind, at.SubmissionKinds,
			)
		}
	}

	if len(options.ScopePrefixes) > 0 && r.Verifier.Type != "manual-review" {
		if !requirementIntersectsScope(r, options.ScopePrefixes) {
			return false, fmt.Sprintf(
				"%s requirement path scope does not intersect assess --scope %v",
				ApplicabilityReasonPrefix, options.ScopePrefixes,
			)
		}
	}

	if r.Verifier.Type == "analyzer-rule" && len(files) > 0 {
		if !analyzerRuleMatchesFiles(r, files, options.ScopePrefixes) {
			return false, fmt.Sprintf(
				"%s no submitted/scoped paths match analyzer rule extensions/prefixes for %s",
				ApplicabilityReasonPrefix, r.ID,
			)
		}
	}

	if len(at.Analyzers) > 0 {
		available := availableAnalyzers(options)
		if len(available) > 0 && !analyzerOverlap(at.Analyzers, available) {
			return false, fmt.Sprintf(
				"%s required analyzer(s) %v not among available %v",
				ApplicabilityReasonPrefix, at.Analyzers, available,
			)
		}
	}

	return true, ""
}

func kindAllowed(kind SubmissionKind, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	want := string(kind)
	for _, a := range allowed {
		if strings.EqualFold(strings.TrimSpace(a), want) {
			return true
		}
	}
	return false
}

func requirementIntersectsScope(r Requirement, scope []string) bool {
	v := r.Verifier
	if fp := strings.TrimSpace(v.FilePath); fp != "" {
		return pathMatches(fp, scope)
	}
	prefixes := v.SourcePathPrefixes
	if len(prefixes) == 0 {
		// No declared path scope → intersects any assess scope.
		return true
	}
	for _, prefix := range prefixes {
		p := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(prefix))))
		p = strings.TrimPrefix(p, "./")
		if p == "" || p == "." {
			return true
		}
		// Either the requirement prefix covers a scope entry, or a scope entry covers the prefix.
		if pathMatches(p, scope) {
			return true
		}
		for _, s := range scope {
			if pathMatches(s, []string{p}) {
				return true
			}
		}
	}
	return false
}

func analyzerRuleMatchesFiles(r Requirement, files []SourceFile, scope []string) bool {
	v := r.Verifier
	extSet := map[string]bool{}
	for _, ext := range v.SourceExtensions {
		ext = strings.ToLower(strings.TrimSpace(ext))
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		extSet[ext] = true
	}
	prefixes := v.SourcePathPrefixes
	for _, f := range files {
		p := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(f.Path))))
		p = strings.TrimPrefix(p, "./")
		if p == "" || p == "." {
			continue
		}
		if len(scope) > 0 && !pathMatches(p, scope) {
			continue
		}
		if len(extSet) > 0 && !extSet[strings.ToLower(filepath.Ext(p))] {
			continue
		}
		if len(prefixes) == 0 || pathMatchesAnyOrRoot(p, prefixes) {
			return true
		}
	}
	return false
}

func pathMatchesAnyOrRoot(path string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return true
	}
	for _, prefix := range prefixes {
		p := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(prefix))))
		p = strings.TrimPrefix(p, "./")
		if p == "" || p == "." {
			return true
		}
	}
	return pathMatches(path, prefixes)
}

func availableAnalyzers(options AssessmentOptions) []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	for _, scan := range options.AnalyzerResults {
		for _, info := range scan.Analyzers {
			add(info.Name)
		}
	}
	for _, bundle := range options.EvidenceBundles {
		for _, info := range bundle.Producers {
			add(info.Name)
		}
	}
	return out
}

func analyzerOverlap(required, available []string) bool {
	avail := map[string]bool{}
	for _, a := range available {
		avail[strings.ToLower(strings.TrimSpace(a))] = true
	}
	for _, r := range required {
		r = strings.ToLower(strings.TrimSpace(r))
		if r == "" {
			continue
		}
		if avail[r] {
			return true
		}
		for a := range avail {
			if strings.Contains(a, r) || strings.Contains(r, a) {
				return true
			}
		}
	}
	return false
}
