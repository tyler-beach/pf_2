package plan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"src/app.py": "print('safe')\n",
		"src/client.ts": "export const ready = true;\n",
		"config/service.json": "{\"timeout_seconds\":30}\n",
		"rules/security.yml": "rules: []\n",
		"analyzer-evidence/old.json": "{\"ignored\":true}\n",
		"testdata/analyzer-fixtures/vulnerable/secret.go": "package vulnerable\n",
	}
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil { t.Fatal(err) }
		if err := os.WriteFile(full, []byte(content), 0600); err != nil { t.Fatal(err) }
	}
	return root
}

func fixtureSpec() AnalyzerSpec {
	return AnalyzerSpec{
		Name: "opengrep", Version: "1.30.0", ConfigurationPath: "rules/security.yml",
		ScopePrefixes: []string{"src/"}, TimeoutMillis: 30_000, MaxRetries: 1,
		RetryableFailureKinds: []string{"timeout", "temporary_process_start"},
	}
}

func TestBuildManifestIsDeterministicAndSourceBound(t *testing.T) {
	root := writeFixture(t)
	first, err := BuildManifest(root, []AnalyzerSpec{fixtureSpec()}, 2)
	if err != nil { t.Fatal(err) }
	second, err := BuildManifest(root, []AnalyzerSpec{fixtureSpec()}, 2)
	if err != nil { t.Fatal(err) }
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) { t.Fatal("same source and plan specification produced different manifests") }
	if len(first.ManifestSHA256) != 64 || len(first.InputSHA256) != 64 { t.Fatalf("missing manifest/input digest: %+v", first) }
	if len(first.SourceInventory) != 4 { t.Fatalf("expected four source/config inventory files, got %d: %+v", len(first.SourceInventory), first.SourceInventory) }
	if len(first.Tasks) != 1 { t.Fatalf("expected one task: %+v", first.Tasks) }
	task := first.Tasks[0]
	if task.InputStatus != "READY" || strings.Join(task.SourcePaths, ",") != "src/app.py,src/client.ts" {
		t.Fatalf("scope was not normalized or generated data leaked into input inventory: %+v", task)
	}
	if task.ConfigurationPath != "rules/security.yml" || len(task.ConfigurationSHA256) != 64 { t.Fatalf("configuration provenance missing: %+v", task) }
	if len(task.CacheKey) != 64 || len(task.IdempotencyKey) != 64 || task.MaxAttempts != 2 {
		t.Fatalf("task execution contract missing: %+v", task)
	}
	if err := VerifyManifest(root, first); err != nil { t.Fatalf("valid manifest did not verify: %v", err) }
}

func TestCacheKeyInvalidatesSourceConfigVersionAndScope(t *testing.T) {
	root := writeFixture(t)
	base, err := BuildManifest(root, []AnalyzerSpec{fixtureSpec()}, 2)
	if err != nil { t.Fatal(err) }
	src := filepath.Join(root, "src", "app.py")
	if err := os.WriteFile(src, []byte("print('changed')\n"), 0600); err != nil { t.Fatal(err) }
	sourceChanged, err := BuildManifest(root, []AnalyzerSpec{fixtureSpec()}, 2)
	if err != nil { t.Fatal(err) }
	if base.Tasks[0].CacheKey == sourceChanged.Tasks[0].CacheKey { t.Fatal("source change did not invalidate cache key") }

	if err := os.WriteFile(filepath.Join(root, "src", "app.py"), []byte("print('safe')\n"), 0600); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(root, "rules", "security.yml"), []byte("rules: [changed]\n"), 0600); err != nil { t.Fatal(err) }
	configChanged, err := BuildManifest(root, []AnalyzerSpec{fixtureSpec()}, 2)
	if err != nil { t.Fatal(err) }
	if base.Tasks[0].CacheKey == configChanged.Tasks[0].CacheKey { t.Fatal("configuration change did not invalidate cache key") }

	if err := os.WriteFile(filepath.Join(root, "rules", "security.yml"), []byte("rules: []\n"), 0600); err != nil { t.Fatal(err) }
	changedSpec := fixtureSpec()
	changedSpec.Version = "1.30.1"
	versionChanged, err := BuildManifest(root, []AnalyzerSpec{changedSpec}, 2)
	if err != nil { t.Fatal(err) }
	if base.Tasks[0].CacheKey == versionChanged.Tasks[0].CacheKey { t.Fatal("analyzer version change did not invalidate cache key") }

	changedSpec = fixtureSpec()
	changedSpec.ScopePrefixes = []string{"src/app.py"}
	scopeChanged, err := BuildManifest(root, []AnalyzerSpec{changedSpec}, 2)
	if err != nil { t.Fatal(err) }
	if base.Tasks[0].CacheKey == scopeChanged.Tasks[0].CacheKey { t.Fatal("scope change did not invalidate cache key") }
}

func TestManifestReportsEmptyScopeInsteadOfClaimingCoverage(t *testing.T) {
	root := writeFixture(t)
	spec := fixtureSpec()
	spec.ScopePrefixes = []string{"internal/missing"}
	got, err := BuildManifest(root, []AnalyzerSpec{spec}, 1)
	if err != nil { t.Fatal(err) }
	if got.Tasks[0].InputStatus != "EMPTY" || len(got.Tasks[0].SourcePaths) != 0 {
		t.Fatalf("empty scope must be explicit, got %+v", got.Tasks[0])
	}
}

func TestBuildManifestRejectsUnsafePathsLimitsAndDuplicateTasks(t *testing.T) {
	root := writeFixture(t)
	cases := []struct{name string; spec AnalyzerSpec; concurrency int}{
		{"absolute configuration", AnalyzerSpec{Name:"opengrep",Version:"1.30.0",ConfigurationPath:filepath.Join(root,"rules","security.yml")}, 1},
		{"traversing configuration", AnalyzerSpec{Name:"opengrep",Version:"1.30.0",ConfigurationPath:"../../etc/passwd"}, 1},
		{"traversing scope", AnalyzerSpec{Name:"opengrep",Version:"1.30.0",ConfigurationPath:"rules/security.yml",ScopePrefixes:[]string{"../outside"}}, 1},
		{"zero concurrency", fixtureSpec(), 0},
		{"excess concurrency", fixtureSpec(), MaxConcurrency+1},
		{"negative retries", func() AnalyzerSpec { s:=fixtureSpec();s.MaxRetries=-1;return s }(), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := BuildManifest(root, []AnalyzerSpec{tc.spec}, tc.concurrency); err == nil {
				t.Fatalf("expected invalid plan rejection")
			}
		})
	}
	if _, err := BuildManifest(root, []AnalyzerSpec{fixtureSpec(), fixtureSpec()}, 1); err == nil {
		t.Fatal("duplicate identical analyzer tasks must be rejected")
	}
}

func TestBuildManifestRejectsSymlinkedConfiguration(t *testing.T) {
	root := writeFixture(t)
	outside := filepath.Join(t.TempDir(), "outside.yml")
	if err := os.WriteFile(outside, []byte("rules: []\n"), 0600); err != nil { t.Fatal(err) }
	config := filepath.Join(root, "rules", "linked.yml")
	if err := os.Symlink(outside, config); err != nil { t.Skipf("symlink unavailable: %v", err) }
	spec := fixtureSpec()
	spec.ConfigurationPath = "rules/linked.yml"
	if _, err := BuildManifest(root, []AnalyzerSpec{spec}, 1); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink configuration rejection, got %v", err)
	}
}

func TestVerifyManifestRejectsStaleSourceAndTampering(t *testing.T) {
	root := writeFixture(t)
	manifest, err := BuildManifest(root, []AnalyzerSpec{fixtureSpec()}, 1)
	if err != nil { t.Fatal(err) }
	manifest.Tasks[0].CacheKey = strings.Repeat("0", 64)
	if err := VerifyManifest(root, manifest); err == nil { t.Fatal("tampered plan unexpectedly verified") }

	manifest, err = BuildManifest(root, []AnalyzerSpec{fixtureSpec()}, 1)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(root, "src", "app.py"), []byte("print('drift')\n"), 0600); err != nil { t.Fatal(err) }
	if err := VerifyManifest(root, manifest); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("source drift should reject plan, got %v", err)
	}
}
