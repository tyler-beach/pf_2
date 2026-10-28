package analyzer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotInputTreeMatchesDiscoverAndFileDigests(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(rel, body string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("src/app.py", "print('hi')\n")
	mustWrite("src/util.go", "package src\n")
	mustWrite("README.md", "# fixture\n")
	mustWrite("node_modules/pkg/index.js", "module.exports=1\n")
	mustWrite(analyzerFixtureDirectory+"/vuln.py", "eval('x')\n")
	mustWrite("rules/security.yml", "rules: []\n")

	snap, err := SnapshotInputTree(root)
	if err != nil {
		t.Fatal(err)
	}
	wantPaths, err := DiscoverScannedPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	gotPaths := make([]string, 0, len(snap.Files))
	for _, f := range snap.Files {
		gotPaths = append(gotPaths, f.Path)
	}
	if len(gotPaths) != len(wantPaths) {
		t.Fatalf("inventory paths=%v want %v", gotPaths, wantPaths)
	}
	for i := range wantPaths {
		if gotPaths[i] != wantPaths[i] {
			t.Fatalf("inventory[%d]=%q want %q", i, gotPaths[i], wantPaths[i])
		}
	}
	for _, f := range snap.Files {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f.Path)))
		if err != nil {
			t.Fatal(err)
		}
		if f.SHA256 != sha256Hex(data) || f.SizeBytes != int64(len(data)) {
			t.Fatalf("digest/size mismatch for %s", f.Path)
		}
	}
	// README.md is hashed into the input digest but is not an eligible source.
	// Fixture / node_modules trees must not appear in the inventory.
	for _, f := range snap.Files {
		if f.Path == "README.md" || f.Path == "node_modules/pkg/index.js" ||
			f.Path == analyzerFixtureDirectory+"/vuln.py" {
			t.Fatalf("ineligible path leaked into inventory: %s", f.Path)
		}
	}
	if snap.InputSHA256 == "" || len(snap.InputSHA256) != 64 {
		t.Fatalf("unexpected input digest %q", snap.InputSHA256)
	}
	// Re-hashing through HashInputTree must stay stable with SnapshotInputTree.
	again, err := HashInputTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if again != snap.InputSHA256 {
		t.Fatalf("HashInputTree drifted from SnapshotInputTree")
	}
}

func TestNormalizeScanPathsSkipsFullTreeWhenScoped(t *testing.T) {
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite := func(rel, body string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("src/app.py", "print(1)\n")
	mustWrite("src/other.py", "print(2)\n")
	mustWrite("rules/security.yml", "rules: []\n")
	mustWrite(analyzerFixtureDirectory+"/x.py", "eval(1)\n")

	got, err := normalizeScanPaths(root, []string{"src/app.py"}, "rules/security.yml")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "src/app.py" {
		t.Fatalf("got %v", got)
	}
	if _, err = normalizeScanPaths(root, []string{"rules/security.yml"}, "rules/security.yml"); err == nil {
		t.Fatal("expected config path rejection")
	}
	if _, err = normalizeScanPaths(root, []string{analyzerFixtureDirectory + "/x.py"}, "rules/security.yml"); err == nil {
		t.Fatal("expected fixture path rejection")
	}
}

func TestHashFileCachedReusesDigest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bin")
	payload := []byte("fake-binary-bytes")
	if err := os.WriteFile(path, payload, 0o700); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	want := hex.EncodeToString(sum[:])
	first, err := HashFileCached(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashFileCached(path)
	if err != nil {
		t.Fatal(err)
	}
	if first != want || second != want {
		t.Fatalf("cached=%s/%s want %s", first, second, want)
	}
}

func TestOpenGrepSkipsVersionProbeWhenBinaryDigestPinned(t *testing.T) {
	root := prepareOpenGrepFixture(t)
	binary := filepath.Join(t.TempDir(), "fake-opengrep")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	digest, err := HashFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	inputHash, err := HashInputTree(root)
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{sarif: []byte(fixtureSARIF)}
	if _, err := (OpenGrepScanner{
		Binary: binary, BinarySHA256: digest, Version: "1.33.0",
		ConfigPath: "rules/security.yml", Runner: runner,
		PrecomputedInputSHA256: inputHash,
	}).Scan(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected a single scan call without --version, got %+v", runner.calls)
	}
	for _, arg := range runner.calls[0].args {
		if arg == "--version" {
			t.Fatalf("version probe should be skipped when binary digest is verified; calls=%v", runner.calls)
		}
	}
}
