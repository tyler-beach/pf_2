package analyzer

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const AnalyzerPlanSchemaVersion = "1"

const (
	ManifestStatusComplete = "COMPLETE"
	ManifestStatusPartial  = "PARTIAL"
	ManifestStatusError    = "ERROR"

	ManifestTaskComplete = "COMPLETE"
	ManifestTaskPartial  = "PARTIAL"
	ManifestTaskError    = "ERROR"
)

// AnalyzerSpec identifies every material input that may affect a scan result.
// Digests refer to the executable, the configuration bytes, and the full rule
// set (including transitive rule files). Resource limits are part of the key
// so a result produced under a looser/stricter execution envelope is not reused.
type AnalyzerSpec struct {
	Name                string   `json:"name"`
	Version             string   `json:"version"`
	BinarySHA256        string   `json:"binary_sha256"`
	ConfigurationPath   string   `json:"configuration_path"`
	ConfigurationSHA256 string   `json:"configuration_sha256"`
	RuleSetSHA256       string   `json:"rule_set_sha256"`
	PathPrefixes        []string `json:"path_prefixes"`
	SourceExtensions    []string `json:"source_extensions,omitempty"`
	TimeoutSeconds      int      `json:"timeout_seconds"`
	MaxMemoryMB         int      `json:"max_memory_mb"`
	MaxFileBytes        int64    `json:"max_file_bytes"`
}

type AnalyzerTask struct {
	ID       string       `json:"id"`
	CacheKey string       `json:"cache_key"`
	Spec     AnalyzerSpec `json:"spec"`
}

type Plan struct {
	SchemaVersion  string         `json:"schema_version"`
	SnapshotSHA256 string         `json:"snapshot_sha256"`
	PlanSHA256     string         `json:"plan_sha256"`
	Tasks          []AnalyzerTask `json:"tasks"`
}

type planDigestPayload struct {
	SchemaVersion  string         `json:"schema_version"`
	SnapshotSHA256 string         `json:"snapshot_sha256"`
	Tasks          []AnalyzerTask `json:"tasks"`
}

type cacheKeyPayload struct {
	SchemaVersion  string       `json:"schema_version"`
	SnapshotSHA256 string       `json:"snapshot_sha256"`
	Spec           AnalyzerSpec `json:"spec"`
}

// BuildPlan canonicalizes analyzer specifications, computes cache keys for the
// exact repository snapshot and all execution inputs, and sorts tasks by ID.
// It rejects duplicate tasks rather than spending analyzer capacity twice.
func BuildPlan(snapshotSHA256 string, specs []AnalyzerSpec) (Plan, error) {
	snapshotSHA256 = strings.ToLower(strings.TrimSpace(snapshotSHA256))
	if !validPlanSHA256(snapshotSHA256) {
		return Plan{}, fmt.Errorf("snapshot_sha256 must be a 64-character hexadecimal SHA-256")
	}
	if len(specs) == 0 {
		return Plan{}, fmt.Errorf("an analyzer plan must contain at least one task")
	}
	plan := Plan{
		SchemaVersion: AnalyzerPlanSchemaVersion,
		SnapshotSHA256: snapshotSHA256,
		Tasks: make([]AnalyzerTask, 0, len(specs)),
	}
	seen := make(map[string]bool, len(specs))
	for i, input := range specs {
		spec, err := normalizeAnalyzerSpec(input)
		if err != nil {
			return Plan{}, fmt.Errorf("analyzer specification %d: %w", i+1, err)
		}
		material := cacheKeyPayload{SchemaVersion: AnalyzerPlanSchemaVersion, SnapshotSHA256: snapshotSHA256, Spec: spec}
		encoded, err := json.Marshal(material)
		if err != nil { return Plan{}, fmt.Errorf("encode analyzer cache-key material: %w", err) }
		key := sha256Hex(encoded)
		if seen[key] { return Plan{}, fmt.Errorf("duplicate analyzer task %q (%s)", spec.Name, key) }
		seen[key] = true
		plan.Tasks = append(plan.Tasks, AnalyzerTask{ID: key, CacheKey: key, Spec: spec})
	}
	sort.Slice(plan.Tasks, func(i, j int) bool { return plan.Tasks[i].ID < plan.Tasks[j].ID })
	digestPayload := planDigestPayload{SchemaVersion: plan.SchemaVersion, SnapshotSHA256: plan.SnapshotSHA256, Tasks: plan.Tasks}
	encoded, err := json.Marshal(digestPayload)
	if err != nil { return Plan{}, fmt.Errorf("encode analyzer plan: %w", err) }
	plan.PlanSHA256 = sha256Hex(encoded)
	return plan, nil
}

func normalizeAnalyzerSpec(input AnalyzerSpec) (AnalyzerSpec, error) {
	spec := input
	spec.Name = strings.ToLower(strings.TrimSpace(spec.Name))
	spec.Version = strings.TrimSpace(spec.Version)
	spec.BinarySHA256 = strings.ToLower(strings.TrimSpace(spec.BinarySHA256))
	spec.ConfigurationSHA256 = strings.ToLower(strings.TrimSpace(spec.ConfigurationSHA256))
	spec.RuleSetSHA256 = strings.ToLower(strings.TrimSpace(spec.RuleSetSHA256))
	if spec.Name == "" || spec.Version == "" {
		return AnalyzerSpec{}, fmt.Errorf("name and pinned version are required")
	}
	for _, r := range spec.Name {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || strings.ContainsRune("._-", r)) {
			return AnalyzerSpec{}, fmt.Errorf("analyzer name %q contains unsupported characters", spec.Name)
		}
	}
	for label, digest := range map[string]string{
		"binary_sha256": spec.BinarySHA256,
		"configuration_sha256": spec.ConfigurationSHA256,
		"rule_set_sha256": spec.RuleSetSHA256,
	} {
		if !validPlanSHA256(digest) { return AnalyzerSpec{}, fmt.Errorf("%s must be a 64-character hexadecimal SHA-256", label) }
	}
	configPath, err := cleanPlanRelativePath(spec.ConfigurationPath, false)
	if err != nil { return AnalyzerSpec{}, fmt.Errorf("configuration_path: %w", err) }
	spec.ConfigurationPath = configPath
	if len(spec.PathPrefixes) == 0 {
		return AnalyzerSpec{}, fmt.Errorf("path_prefixes must declare the analyzed repository scope")
	}
	prefixes := make([]string, 0, len(spec.PathPrefixes))
	seenPrefixes := make(map[string]bool, len(spec.PathPrefixes))
	for _, prefix := range spec.PathPrefixes {
		clean, err := cleanPlanRelativePath(prefix, true)
		if err != nil { return AnalyzerSpec{}, fmt.Errorf("path prefix: %w", err) }
		if seenPrefixes[clean] { return AnalyzerSpec{}, fmt.Errorf("duplicate path prefix %q", clean) }
		seenPrefixes[clean] = true
		prefixes = append(prefixes, clean)
	}
	sort.Strings(prefixes)
	spec.PathPrefixes = prefixes
	extensions := make([]string, 0, len(spec.SourceExtensions))
	seenExtensions := make(map[string]bool, len(spec.SourceExtensions))
	for _, ext := range spec.SourceExtensions {
		ext = strings.ToLower(strings.TrimSpace(ext))
		valid := len(ext) >= 2 && len(ext) <= 12 && ext[0] == '.'
		if valid {
			for _, r := range ext[1:] {
				if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
					valid = false
					break
				}
			}
		}
		if !valid { return AnalyzerSpec{}, fmt.Errorf("invalid source extension %q", ext) }
		if seenExtensions[ext] { return AnalyzerSpec{}, fmt.Errorf("duplicate source extension %q", ext) }
		seenExtensions[ext] = true
		extensions = append(extensions, ext)
	}
	sort.Strings(extensions)
	spec.SourceExtensions = extensions
	if spec.TimeoutSeconds < 1 || spec.TimeoutSeconds > 86400 {
		return AnalyzerSpec{}, fmt.Errorf("timeout_seconds must be between 1 and 86400")
	}
	if spec.MaxMemoryMB < 1 || spec.MaxMemoryMB > 1<<20 {
		return AnalyzerSpec{}, fmt.Errorf("max_memory_mb must be between 1 and 1048576")
	}
	if spec.MaxFileBytes < 1 || spec.MaxFileBytes > 1<<50 {
		return AnalyzerSpec{}, fmt.Errorf("max_file_bytes must be between 1 byte and 1 PiB")
	}
	return spec, nil
}

func cleanPlanRelativePath(value string, allowRoot bool) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || strings.ContainsRune(value, 0) {
		return "", fmt.Errorf("path must be non-empty and contain no NUL")
	}
	if path.IsAbs(value) || filepath.IsAbs(filepath.FromSlash(value)) || (len(value) >= 2 && value[1] == ':') {
		return "", fmt.Errorf("path must be repository-relative")
	}
	clean := path.Clean(value)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path escapes repository root")
	}
	if clean == "." && !allowRoot { return "", fmt.Errorf("file path must name a repository-relative file") }
	return clean, nil
}

func validPlanSHA256(value string) bool {
	if len(value) != 64 { return false }
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

// ValidatePlan recomputes every task key and the overall digest. Plans are
// declarative inputs and may come from stored artifacts; never trust their IDs.
func ValidatePlan(plan Plan) error {
	if plan.SchemaVersion != AnalyzerPlanSchemaVersion {
		return fmt.Errorf("unsupported plan schema_version %q", plan.SchemaVersion)
	}
	specs := make([]AnalyzerSpec, 0, len(plan.Tasks))
	for _, task := range plan.Tasks {
		if task.ID == "" || task.CacheKey == "" || task.ID != task.CacheKey {
			return fmt.Errorf("plan task ID must equal its cache key")
		}
		specs = append(specs, task.Spec)
	}
	expected, err := BuildPlan(plan.SnapshotSHA256, specs)
	if err != nil { return err }
	gotBytes, err := json.Marshal(plan)
	if err != nil { return err }
	expectedBytes, err := json.Marshal(expected)
	if err != nil { return err }
	if !bytes.Equal(gotBytes, expectedBytes) {
		return fmt.Errorf("plan digest or task inventory does not match canonical plan")
	}
	return nil
}

// ManifestID is an idempotency key for duplicate assessment requests. It is
// stable for the same snapshot and plan, and intentionally excludes wall-clock
// values, attempt counts, and results.
func ManifestID(plan Plan) string {
	return sha256Hex([]byte(plan.SchemaVersion + "\x00" + plan.SnapshotSHA256 + "\x00" + plan.PlanSHA256))
}

type TaskExecutor func(context.Context, AnalyzerTask) (ScanResult, error)

type RunOptions struct {
	MaxConcurrency int
	MaxAttempts    int
	Cache          ResultCache
}

type RunManifest struct {
	SchemaVersion  string          `json:"schema_version"`
	ManifestID     string          `json:"manifest_id"`
	SnapshotSHA256 string          `json:"snapshot_sha256"`
	PlanSHA256     string          `json:"plan_sha256"`
	Status         string          `json:"status"`
	Tasks          []TaskExecution `json:"tasks"`
	Errors         []string        `json:"errors,omitempty"`
}

type TaskExecution struct {
	TaskID    string     `json:"task_id"`
	CacheKey  string     `json:"cache_key"`
	Analyzer  string     `json:"analyzer"`
	Status    string     `json:"status"`
	Attempts  int        `json:"attempts"`
	Cached    bool       `json:"cached"`
	Result    ScanResult `json:"result"`
	RawSARIF  []byte     `json:"raw_sarif,omitempty"`
	Error     string     `json:"error,omitempty"`
	Warnings  []string   `json:"warnings,omitempty"`
}

type CacheEntry struct {
	CacheKey string     `json:"cache_key"`
	Result   ScanResult `json:"result"`
	RawSARIF []byte     `json:"raw_sarif"`
}

type ResultCache interface {
	Get(context.Context, string) (CacheEntry, bool, error)
	Put(context.Context, string, CacheEntry) error
}

type MemoryResultCache struct {
	mu      sync.RWMutex
	entries map[string]CacheEntry
}

func NewMemoryResultCache() *MemoryResultCache {
	return &MemoryResultCache{entries: make(map[string]CacheEntry)}
}

func (c *MemoryResultCache) Get(ctx context.Context, key string) (CacheEntry, bool, error) {
	var empty CacheEntry
	if err := ctx.Err(); err != nil { return empty, false, err }
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok { return empty, false, nil }
	return cloneCacheEntry(entry), true, nil
}

func (c *MemoryResultCache) Put(ctx context.Context, key string, entry CacheEntry) error {
	if err := ctx.Err(); err != nil { return err }
	if key == "" || entry.CacheKey != key { return fmt.Errorf("cache key mismatch") }
	c.mu.Lock()
	if c.entries == nil { c.entries = make(map[string]CacheEntry) }
	c.entries[key] = cloneCacheEntry(entry)
	c.mu.Unlock()
	return nil
}

func cloneCacheEntry(entry CacheEntry) CacheEntry {
	entry.RawSARIF = append([]byte(nil), entry.RawSARIF...)
	entry.Result.RawSARIF = append([]byte(nil), entry.Result.RawSARIF...)
	entry.Result.Analyzers = append([]AnalyzerInfo(nil), entry.Result.Analyzers...)
	entry.Result.Errors = append([]string(nil), entry.Result.Errors...)
	entry.Result.ScannedPaths = append([]string(nil), entry.Result.ScannedPaths...)
	entry.Result.Findings = append([]Finding(nil), entry.Result.Findings...)
	for i := range entry.Result.Findings {
		if entry.Result.Findings[i].Properties != nil {
			original := entry.Result.Findings[i].Properties
			copyMap := make(map[string]any, len(original))
			for k, v := range original { copyMap[k] = v }
			entry.Result.Findings[i].Properties = copyMap
		}
	}
	return entry
}

func RunPlan(ctx context.Context, plan Plan, options RunOptions, execute TaskExecutor) (RunManifest, error) {
	if ctx == nil { return RunManifest{}, fmt.Errorf("context is required") }
	if err := ValidatePlan(plan); err != nil { return RunManifest{}, fmt.Errorf("validate analyzer plan: %w", err) }
	if options.MaxConcurrency < 1 { return RunManifest{}, fmt.Errorf("max concurrency must be positive") }
	if options.MaxAttempts < 1 || options.MaxAttempts > 20 { return RunManifest{}, fmt.Errorf("max attempts must be between 1 and 20") }
	if execute == nil { return RunManifest{}, fmt.Errorf("task executor is required") }
	manifest := RunManifest{
		SchemaVersion: AnalyzerPlanSchemaVersion,
		ManifestID: ManifestID(plan),
		SnapshotSHA256: plan.SnapshotSHA256,
		PlanSHA256: plan.PlanSHA256,
		Status: ManifestStatusComplete,
		Tasks: make([]TaskExecution, len(plan.Tasks)),
		Errors: []string{},
	}
	sem := make(chan struct{}, options.MaxConcurrency)
	var wg sync.WaitGroup
	for i, task := range plan.Tasks {
		if err := ctx.Err(); err != nil {
			manifest.Tasks[i] = failedTask(task, fmt.Errorf("plan context canceled before task started: %w", err))
			continue
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			manifest.Tasks[i] = failedTask(task, fmt.Errorf("plan context canceled before task started: %w", ctx.Err()))
			continue
		}
		wg.Add(1)
		go func(index int, task AnalyzerTask) {
			defer wg.Done()
			defer func() { <-sem }()
			manifest.Tasks[index] = runOneTask(ctx, plan, task, options, execute)
		}(i, task)
	}
	wg.Wait()
	completed, failed := 0, 0
	for _, task := range manifest.Tasks {
		if task.Status == ManifestTaskComplete && task.Error == "" { completed++ } else {
			failed++
			if task.Error != "" { manifest.Errors = append(manifest.Errors, task.Analyzer+": "+task.Error) }
			for _, warning := range task.Warnings { manifest.Errors = append(manifest.Errors, task.Analyzer+": "+warning) }
		}
	}
	if failed == 0 {
		manifest.Status = ManifestStatusComplete
	} else if completed > 0 {
		manifest.Status = ManifestStatusPartial
	} else {
		manifest.Status = ManifestStatusError
	}
	return manifest, nil
}

func runOneTask(parent context.Context, plan Plan, task AnalyzerTask, options RunOptions, execute TaskExecutor) TaskExecution {
	report := TaskExecution{
		TaskID: task.ID, CacheKey: task.CacheKey, Analyzer: task.Spec.Name,
		Status: ManifestTaskError, Result: ScanResult{Status: ScanStatusError},
		Warnings: []string{},
	}
	if options.Cache != nil {
		entry, hit, err := options.Cache.Get(parent, task.CacheKey)
		if err != nil {
			report.Error = "read result cache: " + err.Error()
			return report
		}
		if hit {
			if cacheEntryMatchesPlan(entry, plan, task) {
				report.Status = ManifestTaskComplete
				report.Cached = true
				report.Result = entry.Result
				report.RawSARIF = append([]byte(nil), entry.RawSARIF...)
				report.Result.RawSARIF = append([]byte(nil), entry.RawSARIF...)
				return report
			}
			report.Warnings = append(report.Warnings, "ignored a cache entry that failed provenance/integrity validation")
		}
	}
	var lastResult ScanResult
	var lastErr error
	for attempt := 1; attempt <= options.MaxAttempts; attempt++ {
		report.Attempts = attempt
		if err := parent.Err(); err != nil {
			lastErr = err
			break
		}
		runCtx, cancel := context.WithTimeout(parent, time.Duration(task.Spec.TimeoutSeconds)*time.Second)
		result, err := execute(runCtx, task)
		cancel()
		lastResult, lastErr = result, err
		if err == nil && result.Status == ScanStatusComplete {
			if validationErr := validatePlanResult(plan, task, result); validationErr == nil {
				report.Status = ManifestTaskComplete
				report.Result = result
				report.RawSARIF = append([]byte(nil), result.RawSARIF...)
				if options.Cache != nil {
					entry := CacheEntry{CacheKey: task.CacheKey, Result: result, RawSARIF: append([]byte(nil), result.RawSARIF...)}
					if cacheErr := options.Cache.Put(parent, task.CacheKey, entry); cacheErr != nil {
						report.Warnings = append(report.Warnings, "store result cache: "+cacheErr.Error())
					}
				}
				return report
			} else {
				lastErr = fmt.Errorf("invalid analyzer result: %w", validationErr)
			}
		} else if err == nil {
			lastErr = fmt.Errorf("analyzer returned non-complete status %q", result.Status)
		}
		if parent.Err() != nil { break }
	}
	report.Result = lastResult
	report.Result.RawSARIF = append([]byte(nil), lastResult.RawSARIF...)
	report.RawSARIF = append([]byte(nil), lastResult.RawSARIF...)
	if lastResult.Status == ScanStatusPartial {
		report.Status = ManifestTaskPartial
	} else {
		report.Status = ManifestTaskError
	}
	if lastErr != nil { report.Error = lastErr.Error() } else { report.Error = "analyzer did not produce complete evidence" }
	return report
}

func failedTask(task AnalyzerTask, err error) TaskExecution {
	return TaskExecution{
		TaskID: task.ID, CacheKey: task.CacheKey, Analyzer: task.Spec.Name,
		Status: ManifestTaskError, Result: ScanResult{Status: ScanStatusError},
		Warnings: []string{}, Error: err.Error(),
	}
}

func cacheEntryMatchesPlan(entry CacheEntry, plan Plan, task AnalyzerTask) bool {
	if entry.CacheKey != task.CacheKey || entry.Result.Status != ScanStatusComplete || len(entry.RawSARIF) == 0 { return false }
	if entry.Result.RawOutputSHA256 == "" || entry.Result.RawOutputSHA256 != sha256Hex(entry.RawSARIF) { return false }
	if entry.Result.InputSHA256 != plan.SnapshotSHA256 { return false }
	if entry.Result.ConfigurationPath != task.Spec.ConfigurationPath || entry.Result.ConfigurationSHA256 != task.Spec.ConfigurationSHA256 { return false }
	if entry.Result.SARIFVersion != SARIFVersion { return false }
	for _, info := range entry.Result.Analyzers {
		if strings.EqualFold(info.Name, task.Spec.Name) && info.Version == task.Spec.Version { return true }
	}
	return false
}

func validatePlanResult(plan Plan, task AnalyzerTask, result ScanResult) error {
	if result.SchemaVersion == "" || result.SchemaVersion != "1" { return fmt.Errorf("unsupported scan-result schema_version %q", result.SchemaVersion) }
	if result.SARIFVersion != SARIFVersion { return fmt.Errorf("unsupported SARIF version %q", result.SARIFVersion) }
	if result.RawOutputSHA256 == "" || len(result.RawSARIF) == 0 || result.RawOutputSHA256 != sha256Hex(result.RawSARIF) {
		return fmt.Errorf("raw SARIF bytes do not match raw_output_sha256")
	}
	if result.InputSHA256 != plan.SnapshotSHA256 { return fmt.Errorf("input SHA-256 does not match plan snapshot") }
	if result.ConfigurationPath != task.Spec.ConfigurationPath || result.ConfigurationSHA256 != task.Spec.ConfigurationSHA256 {
		return fmt.Errorf("configuration provenance does not match the planned analyzer specification")
	}
	if len(result.Analyzers) == 0 { return fmt.Errorf("analyzer identity is missing") }
	found := false
	for _, info := range result.Analyzers {
		if strings.EqualFold(info.Name, task.Spec.Name) && info.Version == task.Spec.Version { found = true; break }
	}
	if !found { return fmt.Errorf("analyzer identity/version does not match planned specification") }
	if result.Status != ScanStatusComplete { return fmt.Errorf("scan status is %q", result.Status) }
	return nil
}
