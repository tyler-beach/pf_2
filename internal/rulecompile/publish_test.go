package rulecompile

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPublishAndVerifyPackage(t *testing.T) {
	opengrep := os.Getenv("OPENGREP_BIN")
	if opengrep == "" {
		opengrep = "/tmp/opengrep-bin/opengrep"
	}
	if _, err := os.Stat(opengrep); err != nil {
		t.Skip("opengrep binary not available")
	}
	if out, err := exec.Command(opengrep, "--version").CombinedOutput(); err != nil {
		t.Skipf("opengrep not runnable: %v (%s)", err, out)
	}

	root := filepath.Join("..", "..", "testdata", "corpus", "adrs")
	work := t.TempDir()
	compileOut := filepath.Join(work, "work")
	_, err := Compile(Options{
		Root:           root,
		OutDir:         compileOut,
		Provider:       OfflineProvider{},
		OpenGrepBinary: opengrep,
		Limit:          2,
		Repair:         false,
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	dist := filepath.Join(work, "dist")
	pub, err := Publish(PublishOptions{
		Name:    "pathfinder-adr-corpus",
		Version: "0.1.0",
		FromDir: compileOut,
		OutDir:  dist,
		TarGz:   true,
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if pub.Manifest.RuleCount != 2 {
		t.Fatalf("rule_count=%d want 2", pub.Manifest.RuleCount)
	}
	if pub.Manifest.RulesetSHA256 == "" {
		t.Fatal("missing ruleset_sha256")
	}
	if pub.TarGzPath == "" {
		t.Fatal("expected tar.gz")
	}
	if _, err := os.Stat(filepath.Join(pub.PackageDir, "rules.yml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(pub.PackageDir, "bindings.json")); err != nil {
		t.Fatal(err)
	}

	ver, err := VerifyPackage(pub.PackageDir)
	if err != nil {
		t.Fatal(err)
	}
	if !ver.OK {
		t.Fatalf("verify errors: %v", ver.Errors)
	}
	if ver.RulesetSHA256 != pub.Manifest.RulesetSHA256 {
		t.Fatalf("sha mismatch")
	}

	// Unique rewritten ids
	seen := map[string]bool{}
	for _, id := range pub.Manifest.RuleIDs {
		if seen[id] {
			t.Fatalf("duplicate id %s", id)
		}
		seen[id] = true
		if id == "pathfinder.compiled.offline-eval" {
			t.Fatal("expected rewritten stable ids, not offline template id")
		}
	}
}

func TestSemverRequired(t *testing.T) {
	_, err := Publish(PublishOptions{Name: "x", Version: "not-a-version", FromDir: t.TempDir(), OutDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected semver error")
	}
}
