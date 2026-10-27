// Package plan defines deterministic, source-bound analyzer plans. It contains
// no cloud runtime dependencies and never executes analyzers during planning.
package plan

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tyler-beach/pf_2/internal/analyzer"
)

const (
	SchemaVersion             = "1"
	DefaultTimeoutMillis int64 = 5 * 60 * 1000
	DefaultOutputLimit   int64 = 64 << 20
	DefaultRetryBackoff  int64 = 500
	DefaultMaxBackoff    int64 = 10 * 1000
	MaxRetryCount              = 5
	MaxConcurrency             = 16
	MaxPlanTasks               = 32
)

type AnalyzerSpec struct {
	Name                   string   `json:"name"`
	Version                string   `json:"version"`
	BinarySHA256           string   `json:"binary_sha256,omitempty"`
	RuleSetSHA256          string   `json:"ruleset_sha256,omitempty"`
	ConfigurationPath      string   `json:"configuration_path"`
	ScopePrefixes          []string `json:"scope_prefixes,omitempty"`
	TimeoutMillis          int64    `json:"timeout_millis,omitempty"`
	MaxRetries             int      `json:"max_retries"`
	MaxOutputBytes         int64    `json:"max_output_bytes,omitempty"`
	RetryBackoffMillis     int64    `json:"retry_backoff_millis,omitempty"`
	MaxRetryBackoffMillis  int64    `json:"max_retry_backoff_millis,omitempty"`
	RetryableFailureKinds  []string `json:"retryable_failure_kinds,omitempty"`
}

type SourceEntry struct {
	Path      string `json:"path"`
	Language  string `json:"language"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

type Task struct {
	ID                     string   `json:"id"`
	Analyzer               string   `json:"analyzer"`
	Version                string   `json:"version"`
	BinarySHA256           string   `json:"binary_sha256,omitempty"`
	RuleSetSHA256          string   `json:"ruleset_sha256"`
	ConfigurationPath      string   `json:"configuration_path"`
	ConfigurationSHA256    string   `json:"configuration_sha256"`
	InputSHA256             string   `json:"input_sha256"`
	ScopePrefixes          []string `json:"scope_prefixes"`
	SourcePaths            []string `json:"source_paths"`
	InputStatus            string   `json:"input_status"`
	CacheKey               string   `json:"cache_key"`
	IdempotencyKey         string   `json:"idempotency_key"`
	TimeoutMillis          int64    `json:"timeout_millis"`
	MaxAttempts            int      `json:"max_attempts"`
	MaxOutputBytes         int64    `json:"max_output_bytes"`
	RetryBackoffMillis     int64    `json:"retry_backoff_millis"`
	MaxRetryBackoffMillis  int64    `json:"max_retry_backoff_millis"`
	RetryableFailureKinds  []string `json:"retryable_failure_kinds"`
}

type Manifest struct {
	SchemaVersion   string        `json:"schema_version"`
	InputSHA256     string        `json:"input_sha256"`
	SourceInventory []SourceEntry `json:"source_inventory"`
	MaxConcurrency  int           `json:"max_concurrency"`
	Tasks           []Task        `json:"tasks"`
	ManifestSHA256  string        `json:"manifest_sha256,omitempty"`
}

type normalizedSpec struct {
	spec      AnalyzerSpec
	config    string
	configSHA string
}

// BuildManifest creates a deterministic manifest for a repository snapshot.
// It hashes source/configuration inputs and records empty scoped inventories as
// EMPTY rather than silently treating them as compliant.
func BuildManifest(root string, specs []AnalyzerSpec, maxConcurrency int) (Manifest, error) {
	var out Manifest
	rootAbs, err := filepath.Abs(root)
	if err != nil { return out, fmt.Errorf("resolve repository root: %w", err) }
	rootAbs, err = filepath.EvalSymlinks(rootAbs)
	if err != nil { return out, fmt.Errorf("resolve repository root symlinks: %w", err) }
	rootInfo, err := os.Stat(rootAbs)
	if err != nil { return out, fmt.Errorf("inspect repository root: %w", err) }
	if !rootInfo.IsDir() { return out, fmt.Errorf("repository root %q is not a directory", rootAbs) }
	if maxConcurrency < 1 || maxConcurrency > MaxConcurrency {
		return out, fmt.Errorf("max concurrency must be between 1 and %d", MaxConcurrency)
	}
	if len(specs) == 0 || len(specs) > MaxPlanTasks {
		return out, fmt.Errorf("plan must contain between 1 and %d analyzer tasks", MaxPlanTasks)
	}
	inputHash, err := analyzer.HashInputTree(rootAbs)
	if err != nil { return out, fmt.Errorf("hash repository input: %w", err) }
	paths, err := analyzer.DiscoverScannedPaths(rootAbs)
	if err != nil { return out, fmt.Errorf("discover source inventory: %w", err) }
	inventory := make([]SourceEntry, 0, len(paths))
	for _, rel := range paths {
		absolute := filepath.Join(rootAbs, filepath.FromSlash(rel))
		info, err := os.Lstat(absolute)
		if err != nil { return out, fmt.Errorf("inspect source inventory %s: %w", rel, err) }
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return out, fmt.Errorf("source inventory path %q changed type while planning", rel)
		}
		data, err := os.ReadFile(absolute)
		if err != nil { return out, fmt.Errorf("read source inventory %s: %w", rel, err) }
		if int64(len(data)) != info.Size() {
			return out, fmt.Errorf("source inventory path %q changed while planning", rel)
		}
		inventory = append(inventory, SourceEntry{Path: rel, Language: languageForPath(rel), SizeBytes: int64(len(data)), SHA256: digest(data)})
	}
	sort.Slice(inventory, func(i, j int) bool { return inventory[i].Path < inventory[j].Path })

	normalized := make([]normalizedSpec, 0, len(specs))
	for i, raw := range specs {
		spec := raw
		spec.Name = strings.TrimSpace(spec.Name)
		spec.Version = strings.TrimSpace(spec.Version)
		if !validToken(spec.Name) { return out, fmt.Errorf("analyzer spec %d has invalid name %q", i, spec.Name) }
		if !validToken(spec.Version) { return out, fmt.Errorf("analyzer %q has invalid version %q", spec.Name, spec.Version) }
		configPath, configAbs, err := resolveConfiguration(rootAbs, spec.ConfigurationPath)
		if err != nil { return out, fmt.Errorf("analyzer %q configuration: %w", spec.Name, err) }
		configHash, err := analyzer.HashFile(configAbs)
		if err != nil { return out, err }
		binarySHA := strings.ToLower(strings.TrimSpace(spec.BinarySHA256))
		if binarySHA != "" && !validDigest(binarySHA) {
			return out, fmt.Errorf("analyzer %q binary_sha256 must be a lowercase SHA-256 hex digest", spec.Name)
		}
		if strings.EqualFold(spec.Name, "opengrep") && binarySHA == "" {
			return out, fmt.Errorf("OpenGrep plans must pin binary_sha256 before they can be executed")
		}
		ruleSetSHA := strings.ToLower(strings.TrimSpace(spec.RuleSetSHA256))
		if ruleSetSHA == "" { ruleSetSHA = configHash }
		if !validDigest(ruleSetSHA) {
			return out, fmt.Errorf("analyzer %q ruleset_sha256 must be a lowercase SHA-256 hex digest", spec.Name)
		}
		if strings.EqualFold(spec.Name, "opengrep") && ruleSetSHA != configHash {
			return out, fmt.Errorf("OpenGrep ruleset_sha256 must match the SHA-256 of its single configuration/rules file")
		}
		spec.BinarySHA256 = binarySHA
		spec.RuleSetSHA256 = ruleSetSHA
		scopes, err := normalizeScopes(spec.ScopePrefixes)
		if err != nil { return out, fmt.Errorf("analyzer %q scope: %w", spec.Name, err) }
		spec.ConfigurationPath = configPath
		spec.ScopePrefixes = scopes
		if spec.TimeoutMillis == 0 { spec.TimeoutMillis = DefaultTimeoutMillis }
		if spec.TimeoutMillis < 1 || spec.TimeoutMillis > 24*60*60*1000 {
			return out, fmt.Errorf("analyzer %q timeout must be between 1ms and 24h", spec.Name)
		}
		if spec.MaxRetries < 0 || spec.MaxRetries > MaxRetryCount {
			return out, fmt.Errorf("analyzer %q max retries must be between 0 and %d", spec.Name, MaxRetryCount)
		}
		if spec.MaxOutputBytes == 0 { spec.MaxOutputBytes = DefaultOutputLimit }
		if spec.MaxOutputBytes < 1 || spec.MaxOutputBytes > 512<<20 {
			return out, fmt.Errorf("analyzer %q output limit must be between 1 byte and 512 MiB", spec.Name)
		}
		if spec.RetryBackoffMillis == 0 { spec.RetryBackoffMillis = DefaultRetryBackoff }
		if spec.RetryBackoffMillis < 0 || spec.RetryBackoffMillis > 5*60*1000 {
			return out, fmt.Errorf("analyzer %q retry backoff must be between 0 and 5m", spec.Name)
		}
		if spec.MaxRetryBackoffMillis == 0 { spec.MaxRetryBackoffMillis = DefaultMaxBackoff }
		if spec.MaxRetryBackoffMillis < spec.RetryBackoffMillis || spec.MaxRetryBackoffMillis > 30*60*1000 {
			return out, fmt.Errorf("analyzer %q maximum retry backoff must be at least the initial backoff and at most 30m", spec.Name)
		}
		kinds, err := normalizeFailureKinds(spec.RetryableFailureKinds)
		if err != nil { return out, fmt.Errorf("analyzer %q retry policy: %w", spec.Name, err) }
		spec.RetryableFailureKinds = kinds
		normalized = append(normalized, normalizedSpec{spec: spec, config: configPath, configSHA: configHash})
	}
	sort.Slice(normalized, func(i, j int) bool {
		a, b := normalized[i], normalized[j]
		if a.spec.Name != b.spec.Name { return a.spec.Name < b.spec.Name }
		if a.spec.Version != b.spec.Version { return a.spec.Version < b.spec.Version }
		if a.config != b.config { return a.config < b.config }
		return strings.Join(a.spec.ScopePrefixes, ",") < strings.Join(b.spec.ScopePrefixes, ",")
	})
	out = Manifest{SchemaVersion: SchemaVersion, InputSHA256: inputHash, SourceInventory: inventory, MaxConcurrency: maxConcurrency, Tasks: make([]Task, 0, len(normalized))}
	seen := map[string]bool{}
	for _, item := range normalized {
		spec := item.spec
		identity := strings.Join([]string{spec.Name, spec.Version, item.config, strings.Join(spec.ScopePrefixes, ",")}, string(rune(0)))
		if seen[identity] { return Manifest{}, fmt.Errorf("duplicate analyzer task %s@%s for configuration %s and identical scope", spec.Name, spec.Version, item.config) }
		seen[identity] = true
		scopedPaths := make([]string, 0, len(inventory))
		for _, file := range inventory {
			if file.Path == item.config { continue }
			if matchesScope(file.Path, spec.ScopePrefixes) { scopedPaths = append(scopedPaths, file.Path) }
		}
		inputStatus := "READY"
		if len(scopedPaths) == 0 { inputStatus = "EMPTY" }
		cacheKey, err := makeCacheKey(spec, item.configSHA, inputHash, scopedPaths)
		if err != nil { return Manifest{}, err }
		maxAttempts := spec.MaxRetries + 1
		idempotencyKey, err := makeIdempotencyKey(cacheKey, spec, maxAttempts)
		if err != nil { return Manifest{}, err }
		out.Tasks = append(out.Tasks, Task{
			ID: "task-" + idempotencyKey[:16], Analyzer: spec.Name, Version: spec.Version,
			ConfigurationPath: item.config, ConfigurationSHA256: item.configSHA, InputSHA256: inputHash,
			ScopePrefixes: append([]string(nil), spec.ScopePrefixes...), SourcePaths: scopedPaths,
			BinarySHA256: spec.BinarySHA256, RuleSetSHA256: spec.RuleSetSHA256,
			InputStatus: inputStatus, CacheKey: cacheKey, IdempotencyKey: idempotencyKey,
			TimeoutMillis: spec.TimeoutMillis, MaxAttempts: maxAttempts, MaxOutputBytes: spec.MaxOutputBytes,
			RetryBackoffMillis: spec.RetryBackoffMillis, MaxRetryBackoffMillis: spec.MaxRetryBackoffMillis,
			RetryableFailureKinds: append([]string(nil), spec.RetryableFailureKinds...),
		})
	}
	if err := sealManifest(&out); err != nil { return Manifest{}, err }
	return out, nil
}

// VerifyManifest recomputes source inventory and all keys from current inputs.
// It rejects stale or edited plans before an executor may use their cache keys.
func VerifyManifest(root string, supplied Manifest) error {
	if supplied.SchemaVersion != SchemaVersion { return fmt.Errorf("unsupported plan schema_version %q", supplied.SchemaVersion) }
	if supplied.ManifestSHA256 == "" { return fmt.Errorf("plan manifest is missing manifest_sha256") }
	specs := make([]AnalyzerSpec, 0, len(supplied.Tasks))
	for _, task := range supplied.Tasks {
		if task.MaxAttempts < 1 { return fmt.Errorf("task %q has invalid max_attempts", task.ID) }
		specs = append(specs, AnalyzerSpec{
			Name: task.Analyzer, Version: task.Version, BinarySHA256: task.BinarySHA256, RuleSetSHA256: task.RuleSetSHA256,
			ConfigurationPath: task.ConfigurationPath, ScopePrefixes: task.ScopePrefixes,
			TimeoutMillis: task.TimeoutMillis, MaxRetries: task.MaxAttempts-1,
			MaxOutputBytes: task.MaxOutputBytes, RetryBackoffMillis: task.RetryBackoffMillis,
			MaxRetryBackoffMillis: task.MaxRetryBackoffMillis, RetryableFailureKinds: task.RetryableFailureKinds,
		})
	}
	expected, err := BuildManifest(root, specs, supplied.MaxConcurrency)
	if err != nil { return fmt.Errorf("rebuild plan manifest: %w", err) }
	expectedJSON, err := json.Marshal(expected)
	if err != nil { return fmt.Errorf("encode expected plan: %w", err) }
	suppliedJSON, err := json.Marshal(supplied)
	if err != nil { return fmt.Errorf("encode supplied plan: %w", err) }
	if !bytes.Equal(expectedJSON, suppliedJSON) {
		return fmt.Errorf("plan manifest is stale or does not match the current source/configuration snapshot")
	}
	return nil
}

func resolveConfiguration(root, raw string) (string, string, error) {
	normalized, err := normalizeRepoPath(raw, false)
	if err != nil { return "", "", err }
	absolute := filepath.Join(root, filepath.FromSlash(normalized))
	info, err := os.Lstat(absolute)
	if err != nil { return "", "", fmt.Errorf("inspect %s: %w", normalized, err) }
	if info.Mode()&os.ModeSymlink != 0 { return "", "", fmt.Errorf("configuration path %q must not be a symlink", normalized) }
	if !info.Mode().IsRegular() { return "", "", fmt.Errorf("configuration path %q is not a regular file", normalized) }
	real, err := filepath.EvalSymlinks(absolute)
	if err != nil { return "", "", fmt.Errorf("resolve configuration path %q: %w", normalized, err) }
	if filepath.Clean(real) != filepath.Clean(absolute) {
		return "", "", fmt.Errorf("configuration path %q traverses a symlink", normalized)
	}
	rel, err := filepath.Rel(root, real)
	if err != nil { return "", "", fmt.Errorf("make configuration path repository-relative: %w", err) }
	if rel == ".." || strings.HasPrefix(filepath.ToSlash(rel), "../") || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("configuration path %q escapes repository root", normalized)
	}
	return filepath.ToSlash(rel), real, nil
}

func normalizeRepoPath(raw string, allowRoot bool) (string, error) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" || strings.ContainsRune(raw, 0) || path.IsAbs(raw) || (len(raw) >= 2 && raw[1] == ':') {
		return "", fmt.Errorf("path must be a non-empty repository-relative path")
	}
	clean := path.Clean(raw)
	if clean == "." {
		if allowRoot { return ".", nil }
		return "", fmt.Errorf("path cannot name the repository root")
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path %q escapes the repository root", raw)
	}
	return clean, nil
}

func normalizeScopes(raw []string) ([]string, error) {
	if len(raw) == 0 { raw = []string{"."} }
	unique := map[string]bool{}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		scope, err := normalizeRepoPath(item, true)
		if err != nil { return nil, err }
		if !unique[scope] { out = append(out, scope); unique[scope] = true }
	}
	sort.Strings(out)
	return out, nil
}

func normalizeFailureKinds(raw []string) ([]string, error) {
	if raw == nil { raw = []string{"timeout"} }
	unique := map[string]bool{}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(strings.ToLower(item))
		if !validToken(item) { return nil, fmt.Errorf("invalid failure kind %q", item) }
		if !unique[item] { out = append(out, item); unique[item] = true }
	}
	sort.Strings(out)
	return out, nil
}

func validToken(value string) bool {
	if value == "" { return false }
	for i, r := range value {
		valid := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '+' || r == '-'
		if !valid { return false }
		if i == 0 && !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) { return false }
	}
	return true
}

func matchesScope(file string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if prefix == "." || file == prefix || strings.HasPrefix(file, prefix+"/") { return true }
	}
	return false
}

func languageForPath(file string) string {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".go": return "go"
	case ".json": return "json"
	case ".py": return "python"
	case ".js", ".jsx", ".mjs", ".cjs": return "javascript"
	case ".ts", ".tsx": return "typescript"
	case ".java": return "java"
	case ".cs": return "csharp"
	case ".php": return "php"
	case ".rb": return "ruby"
	case ".rs": return "rust"
	case ".sh", ".bash", ".zsh", ".fish": return "shell"
	case ".dart": return "dart"
	case ".swift": return "swift"
	case ".lua": return "lua"
	case ".vb", ".vbs": return "visualbasic"
	case ".ml", ".mli": return "ocaml"
	case ".yaml", ".yml": return "yaml"
	case ".tf", ".hcl": return "hcl"
	case ".kt", ".kts": return "kotlin"
	case ".scala": return "scala"
	case ".c", ".h": return "c"
	case ".cc", ".cpp", ".hpp": return "cpp"
	case ".vue": return "vue"
	case ".html": return "html"
	case ".xml": return "xml"
	case ".sql": return "sql"
	case ".ex", ".exs": return "elixir"
	default: return "unknown"
	}
}

func makeCacheKey(spec AnalyzerSpec, configSHA, inputSHA string, paths []string) (string, error) {
	material := struct {
		SchemaVersion string   `json:"schema_version"`
		Name          string   `json:"name"`
		Version       string   `json:"version"`
		BinarySHA256  string   `json:"binary_sha256,omitempty"`
		RuleSetSHA256 string   `json:"ruleset_sha256"`
		ConfigPath    string   `json:"configuration_path"`
		ConfigSHA256  string   `json:"configuration_sha256"`
		InputSHA256   string   `json:"input_sha256"`
		Scopes        []string `json:"scope_prefixes"`
		SourcePaths   []string `json:"source_paths"`
	}{SchemaVersion, spec.Name, spec.Version, spec.BinarySHA256, spec.RuleSetSHA256, spec.ConfigurationPath, configSHA, inputSHA, spec.ScopePrefixes, paths}
	data, err := json.Marshal(material)
	if err != nil { return "", fmt.Errorf("encode cache-key material: %w", err) }
	return digest(data), nil
}

func makeIdempotencyKey(cacheKey string, spec AnalyzerSpec, attempts int) (string, error) {
	material := struct {
		CacheKey       string   `json:"cache_key"`
		TimeoutMillis  int64    `json:"timeout_millis"`
		MaxAttempts    int      `json:"max_attempts"`
		MaxOutputBytes int64    `json:"max_output_bytes"`
		BackoffMillis  int64    `json:"retry_backoff_millis"`
		MaxBackoff     int64    `json:"max_retry_backoff_millis"`
		RetryableKinds []string `json:"retryable_failure_kinds"`
	}{cacheKey, spec.TimeoutMillis, attempts, spec.MaxOutputBytes, spec.RetryBackoffMillis, spec.MaxRetryBackoffMillis, spec.RetryableFailureKinds}
	data, err := json.Marshal(material)
	if err != nil { return "", fmt.Errorf("encode idempotency-key material: %w", err) }
	return digest(data), nil
}

func sealManifest(m *Manifest) error {
	m.ManifestSHA256 = ""
	data, err := json.Marshal(m)
	if err != nil { return fmt.Errorf("encode plan manifest for digest: %w", err) }
	m.ManifestSHA256 = digest(data)
	return nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
