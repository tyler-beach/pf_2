package analyzer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func planTestSpec(name string) AnalyzerSpec {
	return AnalyzerSpec{
		Name: name,
		Version: "1.30.0",
		BinarySHA256: strings.Repeat("a", 64),
		ConfigurationPath: "rules/security.yml",
		ConfigurationSHA256: strings.Repeat("b", 64),
		RuleSetSHA256: strings.Repeat("c", 64),
		PathPrefixes: []string{"internal/", "cmd/"},
		SourceExtensions: []string{".go", ".py"},
		TimeoutSeconds: 15,
		MaxMemoryMB: 512,
		MaxFileBytes: 4 << 20,
	}
}

func planTestResult(spec AnalyzerSpec, snapshot string, status string) ScanResult {
	raw := []byte(`{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"opengrep","version":"1.30.0"}},"results":[]}]}`)
	result, _ := ParseSARIF(raw, ".")
	result.Status = status
	result.Analyzers = []AnalyzerInfo{{Name: spec.Name, Version: spec.Version}}
	result.ConfigurationPath = spec.ConfigurationPath
	result.ConfigurationSHA256 = spec.ConfigurationSHA256
	result.InputSHA256 = snapshot
	result.RawOutputSHA256 = sha256Hex(raw)
	result.RawSARIF = append([]byte(nil), raw...)
	return result
}

func TestBuildPlanIsDeterministicAcrossSpecificationOrdering(t *testing.T) {
	snapshot := strings.Repeat("d", 64)
	a := planTestSpec("opengrep")
	b := planTestSpec("gosec")
	b.ConfigurationPath = "rules/gosec.yml"
	b.ConfigurationSHA256 = strings.Repeat("e", 64)
	one, err := BuildPlan(snapshot, []AnalyzerSpec{a, b})
	if err != nil { t.Fatal(err) }
	two, err := BuildPlan(snapshot, []AnalyzerSpec{b, a})
	if err != nil { t.Fatal(err) }
	oneJSON, err := json.Marshal(one)
	if err != nil { t.Fatal(err) }
	twoJSON, err := json.Marshal(two)
	if err != nil { t.Fatal(err) }
	if string(oneJSON) != string(twoJSON) {
		t.Fatalf("plans should have deterministic canonical order:\n%s\n%s", oneJSON, twoJSON)
	}
	if one.PlanSHA256 != two.PlanSHA256 { t.Fatalf("plan digest changed with order: %s != %s", one.PlanSHA256, two.PlanSHA256) }
	if err := ValidatePlan(one); err != nil { t.Fatalf("valid plan rejected: %v", err) }
}

func TestAnalyzerCacheKeyInvalidatesEveryMaterialInput(t *testing.T) {
	snapshot := strings.Repeat("d", 64)
	base := planTestSpec("opengrep")
	original, err := BuildPlan(snapshot, []AnalyzerSpec{base})
	if err != nil { t.Fatal(err) }
	mutations := map[string]func(*AnalyzerSpec, *string){
		"source snapshot": func(_ *AnalyzerSpec, hash *string) { *hash = strings.Repeat("f", 64) },
		"analyzer version": func(s *AnalyzerSpec, _ *string) { s.Version = "1.30.1" },
		"binary digest": func(s *AnalyzerSpec, _ *string) { s.BinarySHA256 = strings.Repeat("1", 64) },
		"configuration digest": func(s *AnalyzerSpec, _ *string) { s.ConfigurationSHA256 = strings.Repeat("2", 64) },
		"ruleset digest": func(s *AnalyzerSpec, _ *string) { s.RuleSetSHA256 = strings.Repeat("3", 64) },
		"configuration path": func(s *AnalyzerSpec, _ *string) { s.ConfigurationPath = "rules/alternate.yml" },
		"path scope": func(s *AnalyzerSpec, _ *string) { s.PathPrefixes = []string{"internal/domain/"} },
		"language scope": func(s *AnalyzerSpec, _ *string) { s.SourceExtensions = []string{".go"} },
		"timeout": func(s *AnalyzerSpec, _ *string) { s.TimeoutSeconds++ },
		"memory limit": func(s *AnalyzerSpec, _ *string) { s.MaxMemoryMB++ },
		"file size limit": func(s *AnalyzerSpec, _ *string) { s.MaxFileBytes++ },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			spec := base
			spec.PathPrefixes = append([]string(nil), base.PathPrefixes...)
			spec.SourceExtensions = append([]string(nil), base.SourceExtensions...)
			input := snapshot
			mutate(&spec, &input)
			changed, err := BuildPlan(input, []AnalyzerSpec{spec})
			if err != nil { t.Fatal(err) }
			if changed.Tasks[0].CacheKey == original.Tasks[0].CacheKey {
				t.Fatalf("cache key did not change after changing %s", name)
			}
		})
	}
}

func TestBuildPlanRejectsUnsafeOrAmbiguousSpecifications(t *testing.T) {
	snapshot := strings.Repeat("d", 64)
	cases := []struct {
		name string
		mutate func(*AnalyzerSpec)
		specs int
		hash string
	}{
		{"path traversal scope", func(s *AnalyzerSpec) { s.PathPrefixes = []string{"../outside"} }, 1, snapshot},
		{"absolute config path", func(s *AnalyzerSpec) { s.ConfigurationPath = "/tmp/rules.yml" }, 1, snapshot},
		{"missing binary digest", func(s *AnalyzerSpec) { s.BinarySHA256 = "" }, 1, snapshot},
		{"missing timeout", func(s *AnalyzerSpec) { s.TimeoutSeconds = 0 }, 1, snapshot},
		{"missing resource limit", func(s *AnalyzerSpec) { s.MaxMemoryMB = 0 }, 1, snapshot},
		{"invalid snapshot digest", func(*AnalyzerSpec) {}, 1, "not-a-sha"},
		{"duplicate task", func(*AnalyzerSpec) {}, 2, snapshot},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := planTestSpec("opengrep")
			tc.mutate(&spec)
			specs := []AnalyzerSpec{spec}
			if tc.specs == 2 { specs = []AnalyzerSpec{spec, spec} }
			if _, err := BuildPlan(tc.hash, specs); err == nil { t.Fatal("invalid plan unexpectedly succeeded") }
		})
	}
}

func TestRunPlanBoundsConcurrencyAndSortsResults(t *testing.T) {
	snapshot := strings.Repeat("d", 64)
	specs := []AnalyzerSpec{planTestSpec("opengrep"), planTestSpec("gosec"), planTestSpec("conftest")}
	specs[1].ConfigurationPath = "rules/gosec.yml"
	specs[1].ConfigurationSHA256 = strings.Repeat("e", 64)
	specs[2].ConfigurationPath = "rules/conftest.yml"
	specs[2].ConfigurationSHA256 = strings.Repeat("f", 64)
	plan, err := BuildPlan(snapshot, specs)
	if err != nil { t.Fatal(err) }
	var active, maximum atomic.Int32
	executor := func(ctx context.Context, task AnalyzerTask) (ScanResult, error) {
		n := active.Add(1)
		for {
			old := maximum.Load()
			if n <= old || maximum.CompareAndSwap(old, n) { break }
		}
		defer active.Add(-1)
		select {
		case <-time.After(20 * time.Millisecond):
		case <-ctx.Done(): return ScanResult{}, ctx.Err()
		}
		return planTestResult(task.Spec, snapshot, ScanStatusComplete), nil
	}
	manifest, err := RunPlan(context.Background(), plan, RunOptions{MaxConcurrency:2, MaxAttempts:1}, executor)
	if err != nil { t.Fatal(err) }
	if maximum.Load() > 2 || maximum.Load() < 2 {
		t.Fatalf("observed concurrency=%d, want exactly 2 at peak", maximum.Load())
	}
	if manifest.Status != ManifestStatusComplete || len(manifest.Tasks) != 3 {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	for i := 1; i < len(manifest.Tasks); i++ {
		if manifest.Tasks[i-1].TaskID > manifest.Tasks[i].TaskID {
			t.Fatalf("task results must be canonically ordered: %+v", manifest.Tasks)
		}
	}
	if manifest.ManifestID == "" || manifest.ManifestID != ManifestID(plan) {
		t.Fatalf("manifest ID is not stable/idempotent: %+v", manifest)
	}
}

func TestRunPlanRetriesFailuresAndPreservesPartialStatus(t *testing.T) {
	snapshot := strings.Repeat("d", 64)
	good, bad := planTestSpec("gosec"), planTestSpec("opengrep")
	good.ConfigurationPath = "rules/gosec.yml"
	good.ConfigurationSHA256 = strings.Repeat("e", 64)
	plan, err := BuildPlan(snapshot, []AnalyzerSpec{good, bad})
	if err != nil { t.Fatal(err) }
	var goodAttempts, badAttempts atomic.Int32
	executor := func(ctx context.Context, task AnalyzerTask) (ScanResult, error) {
		if task.Spec.Name == "gosec" {
			if goodAttempts.Add(1) == 1 { return ScanResult{}, errors.New("transient failure") }
			return planTestResult(task.Spec, snapshot, ScanStatusComplete), nil
		}
		badAttempts.Add(1)
		return planTestResult(task.Spec, snapshot, ScanStatusPartial), nil
	}
	manifest, err := RunPlan(context.Background(), plan, RunOptions{MaxConcurrency:2, MaxAttempts:2}, executor)
	if err != nil { t.Fatal(err) }
	if goodAttempts.Load() != 2 || badAttempts.Load() != 2 {
		t.Fatalf("unexpected retry counts: good=%d partial=%d", goodAttempts.Load(), badAttempts.Load())
	}
	if manifest.Status != ManifestStatusPartial {
		t.Fatalf("mixed complete/partial results must remain PARTIAL, got %s", manifest.Status)
	}
	var sawComplete, sawPartial bool
	for _, result := range manifest.Tasks {
		if result.TaskID == "" || result.Attempts < 1 { t.Fatalf("missing task provenance: %+v", result) }
		if result.Analyzer == "gosec" && result.Status == ManifestTaskComplete { sawComplete = true }
		if result.Analyzer == "opengrep" && result.Status == ManifestTaskPartial { sawPartial = true }
	}
	if !sawComplete || !sawPartial { t.Fatalf("task-level results lost completion state: %+v", manifest.Tasks) }
}

func TestRunPlanCancellationDoesNotLaunchRemainingTasks(t *testing.T) {
	snapshot := strings.Repeat("d", 64)
	plan, err := BuildPlan(snapshot, []AnalyzerSpec{planTestSpec("opengrep"), planTestSpec("gosec"), planTestSpec("conftest")})
	if err != nil { t.Fatal(err) }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 1)
	var executed atomic.Int32
	done := make(chan RunManifest, 1)
	go func() {
		manifest, _ := RunPlan(ctx, plan, RunOptions{MaxConcurrency:1, MaxAttempts:3}, func(ctx context.Context, task AnalyzerTask) (ScanResult, error) {
			executed.Add(1)
			select { case started <- struct{}{}: default: }
			<-ctx.Done()
			return ScanResult{}, ctx.Err()
		})
		done <- manifest
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second): t.Fatal("executor did not start")
	}
	cancel()
	select {
	case manifest := <-done:
		if manifest.Status != ManifestStatusError { t.Fatalf("canceled plan status=%s, want ERROR", manifest.Status) }
		if executed.Load() != 1 { t.Fatalf("cancellation started %d tasks; want exactly 1", executed.Load()) }
		for _, result := range manifest.Tasks {
			if result.Status != ManifestTaskError { t.Fatalf("canceled/unstarted task not marked ERROR: %+v", result) }
		}
	case <-time.After(2 * time.Second): t.Fatal("RunPlan did not cancel promptly")
	}
}

func TestRunPlanUsesCacheAndPreservesRawArtifact(t *testing.T) {
	snapshot := strings.Repeat("d", 64)
	spec := planTestSpec("opengrep")
	plan, err := BuildPlan(snapshot, []AnalyzerSpec{spec})
	if err != nil { t.Fatal(err) }
	cache := NewMemoryResultCache()
	var calls atomic.Int32
	executor := func(ctx context.Context, task AnalyzerTask) (ScanResult, error) {
		calls.Add(1)
		return planTestResult(task.Spec, snapshot, ScanStatusComplete), nil
	}
	opts := RunOptions{MaxConcurrency:1, MaxAttempts:1, Cache:cache}
	first, err := RunPlan(context.Background(), plan, opts, executor)
	if err != nil { t.Fatal(err) }
	second, err := RunPlan(context.Background(), plan, opts, executor)
	if err != nil { t.Fatal(err) }
	if calls.Load() != 1 { t.Fatalf("second identical event should use cache, executor ran %d times", calls.Load()) }
	if len(second.Tasks) != 1 || !second.Tasks[0].Cached || second.Tasks[0].Attempts != 0 {
		t.Fatalf("expected cache hit in second manifest: %+v", second.Tasks)
	}
	if len(first.Tasks[0].RawSARIF) == 0 || string(first.Tasks[0].RawSARIF) != string(second.Tasks[0].RawSARIF) {
		t.Fatal("cache hit did not preserve raw scan artifact")
	}
	if first.ManifestID != second.ManifestID { t.Fatalf("duplicate input should produce the same idempotency key: %s != %s", first.ManifestID, second.ManifestID) }
}

func TestMemoryCacheRejectsMutationOfReturnedRawBytes(t *testing.T) {
	cache := NewMemoryResultCache()
	raw := []byte("immutable bytes")
	entry := CacheEntry{CacheKey:"key", RawSARIF:raw}
	if err := cache.Put(context.Background(), "key", entry); err != nil { t.Fatal(err) }
	raw[0] = 'X'
	loaded, ok, err := cache.Get(context.Background(), "key")
	if err != nil || !ok { t.Fatalf("cache read failed: ok=%v err=%v", ok, err) }
	if string(loaded.RawSARIF) != "immutable bytes" { t.Fatalf("cache retained aliased slice: %q", loaded.RawSARIF) }
	loaded.RawSARIF[0] = 'Y'
	again, _, err := cache.Get(context.Background(), "key")
	if err != nil { t.Fatal(err) }
	if string(again.RawSARIF) != "immutable bytes" { t.Fatalf("cache exposed internal slice: %q", again.RawSARIF) }
}

func TestPlanDigestAndCacheKeyAreValidSHA256(t *testing.T) {
	plan, err := BuildPlan(strings.Repeat("d", 64), []AnalyzerSpec{planTestSpec("opengrep")})
	if err != nil { t.Fatal(err) }
	for name, digest := range map[string]string{"plan": plan.PlanSHA256, "cache": plan.Tasks[0].CacheKey, "task": plan.Tasks[0].ID} {
		if len(digest) != 64 { t.Errorf("%s digest length=%d: %q", name, len(digest), digest) }
	}
	encoded, err := json.Marshal(plan)
	if err != nil { t.Fatal(err) }
	if strings.Contains(string(encoded), "RawSARIF") { t.Fatal("plan should not carry scan artifacts") }
	if len(fmt.Sprint(plan.Tasks[0].Spec.Name)) == 0 { t.Fatal("unreachable") }
	var decoded Plan
	if err := json.Unmarshal(encoded, &decoded); err != nil { t.Fatal(err) }
	if err := ValidatePlan(decoded); err != nil { t.Fatalf("serialized plan rejected: %v", err) }
}
