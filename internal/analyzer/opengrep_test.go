package analyzer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fixtureSARIF = `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"opengrep","version":"1.33.0","rules":[{"id":"pathfinder.test-eval"}]}},"results":[{"ruleId":"pathfinder.test-eval","level":"error","message":{"text":"Avoid eval"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"src/app.py"},"region":{"startLine":7,"startColumn":1}}}],"partialFingerprints":{"primaryLocationLineHash":"fixture-fingerprint"}}]}]}`

type runnerCall struct {
	binary string
	args   []string
	dir    string
}

type fakeRunner struct {
	calls           []runnerCall
	sarif           []byte
	scanExitCode    int
	versionOutput   []byte
	versionExitCode int
	scanErr         error
	scanStderr      []byte
	waitForCancel   bool
}

func (f *fakeRunner) Run(ctx context.Context, binary string, args []string, dir string) (CommandResult, error) {
	f.calls = append(f.calls, runnerCall{binary: binary, args: append([]string(nil), args...), dir: dir})
	if len(args) == 1 && args[0] == "--version" {
		output := f.versionOutput
		if output == nil {
			output = []byte("OpenGrep 1.33.0\n")
		}
		return CommandResult{ExitCode: f.versionExitCode, Stdout: output}, nil
	}
	if f.waitForCancel {
		<-ctx.Done()
		return CommandResult{ExitCode: -1}, fmt.Errorf("cancelled fake scan: %w", ctx.Err())
	}
	if f.scanErr != nil {
		return CommandResult{ExitCode: -1}, f.scanErr
	}
	var outputPath string
	for _, arg := range args {
		if strings.HasPrefix(arg, "--sarif-output=") {
			outputPath = strings.TrimPrefix(arg, "--sarif-output=")
		}
	}
	if outputPath == "" {
		return CommandResult{ExitCode: -1}, fmt.Errorf("missing SARIF output argument")
	}
	if err := os.WriteFile(outputPath, f.sarif, 0600); err != nil {
		return CommandResult{ExitCode: -1}, err
	}
	stderr := f.scanStderr
	if stderr == nil {
		stderr = []byte("fixture scanner stderr")
	}
	return CommandResult{ExitCode: f.scanExitCode, Stderr: stderr}, nil
}

func prepareOpenGrepFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "rules"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "rules", "security.yml"), []byte("rules: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "app.py"), []byte("print('fixture')\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestOpenGrepScannerRunsPinnedCLIAndNormalizesEvidence(t *testing.T) {
	root := prepareOpenGrepFixture(t)
	runner := &fakeRunner{sarif: []byte(fixtureSARIF)}
	scanner := OpenGrepScanner{Binary: "/opt/opengrep", Version: "1.33.0", ConfigPath: "rules/security.yml", Timeout: time.Second, Runner: runner}
	got, err := scanner.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if got.Status != ScanStatusComplete || len(got.Findings) != 1 {
		t.Fatalf("unexpected scan result: %+v", got)
	}
	if got.Findings[0].Fingerprint != "fixture-fingerprint" || got.Findings[0].Path != "src/app.py" {
		t.Fatalf("finding provenance lost: %+v", got.Findings[0])
	}
	if len(got.ConfigurationSHA256) != 64 || len(got.InputSHA256) != 64 || len(got.RawOutputSHA256) != 64 {
		t.Fatalf("missing evidence digests: %+v", got)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("expected version probe and scan, got %d calls", len(runner.calls))
	}
	if runner.calls[0].args[0] != "--version" {
		t.Fatalf("version was not probed first: %+v", runner.calls)
	}
	scan := runner.calls[1]
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if scan.binary != "/opt/opengrep" || scan.dir != rootReal {
		t.Fatalf("unexpected execution target: %+v (want dir %q)", scan, rootReal)
	}
	wantArgs := []string{"scan", "-f", "rules/security.yml", "--exclude", analyzerFixtureDirectory, "."}
	for _, want := range wantArgs {
		found := false
		for _, arg := range scan.args {
			if arg == want || (strings.HasPrefix(want, "scan") && arg == "scan") {
				found = true
			}
		}
		if !found {
			t.Errorf("scan arguments %v do not include %q", scan.args, want)
		}
	}
	if strings.Contains(strings.Join(scan.args, " "), "&&") {
		t.Fatal("arguments must not be interpreted by a shell")
	}
}

func TestOpenGrepScannerAcceptsFindingsExitCodeOne(t *testing.T) {
	root := prepareOpenGrepFixture(t)
	runner := &fakeRunner{sarif: []byte(fixtureSARIF), scanExitCode: 1}
	got, err := (OpenGrepScanner{Version: "1.33.0", ConfigPath: "rules/security.yml", Runner: runner}).Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("exit 1 with valid finding SARIF should be accepted: %v", err)
	}
	if got.Status != ScanStatusComplete || len(got.Findings) != 1 {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestOpenGrepScannerRejectsExitOneWithoutFindings(t *testing.T) {
	root := prepareOpenGrepFixture(t)
	empty := []byte(`{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"opengrep","version":"1.33.0"}},"results":[]}]}`)
	runner := &fakeRunner{sarif: empty, scanExitCode: 1}
	got, err := (OpenGrepScanner{Version: "1.33.0", ConfigPath: "rules/security.yml", Runner: runner}).Scan(context.Background(), root)
	if err == nil || got.Status != ScanStatusError {
		t.Fatalf("exit 1 without findings must fail closed, got result=%+v err=%v", got, err)
	}
}

func TestOpenGrepScannerRejectsFatalExitCode(t *testing.T) {
	root := prepareOpenGrepFixture(t)
	runner := &fakeRunner{sarif: []byte(fixtureSARIF), scanExitCode: 2}
	got, err := (OpenGrepScanner{Version: "1.33.0", ConfigPath: "rules/security.yml", Runner: runner}).Scan(context.Background(), root)
	if err == nil || got.Status != ScanStatusError || !strings.Contains(err.Error(), "exited 2") {
		t.Fatalf("expected fatal scan error, got %+v, %v", got, err)
	}
}

func TestOpenGrepScannerRejectsVersionMismatch(t *testing.T) {
	root := prepareOpenGrepFixture(t)
	runner := &fakeRunner{versionOutput: []byte("OpenGrep 9.9.9")}
	_, err := (OpenGrepScanner{Version: "1.33.0", ConfigPath: "rules/security.yml", Runner: runner}).Scan(context.Background(), root)
	if err == nil || !strings.Contains(err.Error(), "version mismatch") {
		t.Fatalf("expected version mismatch, got %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("scan should not run after version mismatch: %+v", runner.calls)
	}
}

func TestOpenGrepScannerRejectsConfigOutsideRepository(t *testing.T) {
	root := prepareOpenGrepFixture(t)
	outside := filepath.Join(t.TempDir(), "outside.yml")
	if err := os.WriteFile(outside, []byte("rules: []"), 0600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{sarif: []byte(fixtureSARIF)}
	_, err := (OpenGrepScanner{Version: "1.33.0", ConfigPath: outside, Runner: runner}).Scan(context.Background(), root)
	if err == nil || !strings.Contains(err.Error(), "inside repository root") {
		t.Fatalf("expected configuration path rejection, got %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("scanner should not launch for unsafe config: %+v", runner.calls)
	}
}

func TestOpenGrepScannerHonorsTimeout(t *testing.T) {
	root := prepareOpenGrepFixture(t)
	runner := &fakeRunner{waitForCancel: true}
	_, err := (OpenGrepScanner{Version: "1.33.0", ConfigPath: "rules/security.yml", Timeout: 15 * time.Millisecond, Runner: runner}).Scan(context.Background(), root)
	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("expected context timeout, got %v", err)
	}
}

func TestScannerEnvironmentExcludesAmbientCredentials(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "must-not-be-forwarded")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "must-not-be-forwarded")
	for _, entry := range scannerEnvironment() {
		if strings.HasPrefix(entry, "GITHUB_TOKEN=") || strings.HasPrefix(entry, "AWS_SECRET_ACCESS_KEY=") ||
			strings.HasPrefix(entry, "HOME=") || strings.HasPrefix(entry, "USERPROFILE=") {
			t.Fatalf("ambient credential or home configuration was forwarded to analyzer process: %q", entry)
		}
	}
}

func TestHashInputTreeIsStableAndTracksContent(t *testing.T) {
	root := prepareOpenGrepFixture(t)
	first, err := hashInputTree(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := hashInputTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("hash unstable: %s != %s", first, second)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "app.py"), []byte("print('changed')\n"), 0600); err != nil {
		t.Fatal(err)
	}
	third, err := hashInputTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatal("hash did not change when source content changed")
	}
}

func TestResolveConfigRejectsSymlinkEscapingRepository(t *testing.T) {
	root := prepareOpenGrepFixture(t)
	outside := filepath.Join(t.TempDir(), "secret.yml")
	if err := os.WriteFile(outside, []byte("rules: []"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "rules", "escape.yml")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	_, _, err := resolveConfig(root, "rules/escape.yml")
	if err == nil || !strings.Contains(err.Error(), "inside repository root") {
		t.Fatalf("expected symlink escape to be rejected, got %v", err)
	}
}

func TestHashInputTreeIgnoresGeneratedAssessmentArtifacts(t *testing.T) {
	root := prepareOpenGrepFixture(t)
	before, err := hashInputTree(root)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{
		"assessment.json":            "generated assessment",
		"results.sarif":              "generated SARIF",
		"report.md":                  "generated report",
		"coverage.out":               "coverage output",
		"ingested-adrs.json":         "generated ADR index",
		"obligation-assessment.json": "generated obligation assessment",
		"obligation-assessment.md":   "generated obligation report",
		"obligation-candidates.md":   "generated candidate queue",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, ".arch-go"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".arch-go", "report.json"), []byte("generated Arch-Go report"), 0600); err != nil {
		t.Fatal(err)
	}
	after, err := hashInputTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("generated artifacts changed source input hash: %s != %s", before, after)
	}
}

func TestDiscoverScannedPathsIsSortedAndSkipsGeneratedDirectories(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{"src", ".git", "analyzer-evidence", "node_modules", ".arch-go", "testdata/analyzer-fixtures/vulnerable"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for file, body := range map[string]string{
		"src/z.go":                     "package z",
		"src/a.py":                     "print('a')",
		".git/ignored.go":              "package ignored",
		"analyzer-evidence/ignored.ts": "const ignored = true",
		"node_modules/ignored.js":      "ignored",
		".arch-go/report.json":         `{"generated":true}`,
		"testdata/analyzer-fixtures/vulnerable/RuntimeExec.java": "class RuntimeExec {}",
		"testdata/analyzer-fixtures/vulnerable/eval.py":          "eval(input())",
		"README.md": "docs",
	} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(file)), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := discoverScannedPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"src/a.py", "src/z.go"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("inventory = %v, want %v", got, want)
	}
}

func TestOpenGrepScannerRejectsPartialAnalysisEvenWhenSARIFExists(t *testing.T) {
	root := prepareOpenGrepFixture(t)
	runner := &fakeRunner{
		sarif:        []byte(fixtureSARIF),
		scanExitCode: 1,
		scanStderr:   []byte("Partially scanned: 1 files only partially analyzed due to parsing or internal OpenGrep errors"),
	}
	got, err := (OpenGrepScanner{Version: "1.33.0", ConfigPath: "rules/security.yml", Runner: runner}).Scan(context.Background(), root)
	if err == nil || got.Status != ScanStatusPartial {
		t.Fatalf("partial analysis must not be accepted as complete: result=%+v err=%v", got, err)
	}
	if len(got.RawSARIF) == 0 {
		t.Fatal("raw SARIF should be retained for diagnosis")
	}
	if len(got.Errors) == 0 || !strings.Contains(got.Errors[0], "Partially scanned") {
		t.Fatalf("partial scan warning was not preserved: %+v", got.Errors)
	}
}

func TestExecCommandRunnerBuildsCredentialFreeEnvironment(t *testing.T) {
	t.Setenv("HOME", filepath.Join(t.TempDir(), "real-home"))
	t.Setenv("USERPROFILE", filepath.Join(t.TempDir(), "real-profile"))
	t.Setenv("OPENAI_API_KEY", "do-not-forward")
	t.Setenv("ANTHROPIC_API_KEY", "do-not-forward")
	t.Setenv("GITHUB_TOKEN", "do-not-forward")
	env := scannerEnvironment()
	joined := strings.Join(env, "\n")
	for _, forbidden := range []string{"HOME=", "USERPROFILE=", "OPENAI_API_KEY=", "ANTHROPIC_API_KEY=", "GITHUB_TOKEN="} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("scanner environment unexpectedly includes %q", forbidden)
		}
	}
	home := filepath.Join(t.TempDir(), "isolated")
	runner := ExecCommandRunner{HomeDir: home}
	if runner.HomeDir != home {
		t.Fatal("isolated analyzer home was not configured")
	}
}

func TestOpenGrepScannerAcceptsGenericSkippedFileSummary(t *testing.T) {
	root := prepareOpenGrepFixture(t)
	runner := &fakeRunner{
		sarif:        []byte(fixtureSARIF),
		scanExitCode: 1,
		scanStderr:   []byte("Some files were skipped or only partially analyzed."),
	}
	got, err := (OpenGrepScanner{Version: "1.33.0", ConfigPath: "rules/security.yml", Runner: runner}).Scan(context.Background(), root)
	if err != nil || got.Status != ScanStatusComplete {
		t.Fatalf("generic skipped-file summary should not make a successful rule scan partial: result=%+v err=%v", got, err)
	}
}

func TestIncompleteAnalysisDiagnosticIsSpecific(t *testing.T) {
	cases := []struct {
		message string
		want    bool
	}{
		{"Some files were skipped or only partially analyzed.", false},
		{"Partially scanned: 1 files only partially analyzed due to parsing or internal OpenGrep errors", true},
		{"Partially scanned: 0 files only partially analyzed", false},
		{"[ERROR] Rule parse error in rule rule.bad", true},
		{"", false},
	}
	for _, tc := range cases {
		if got := incompleteAnalysisDiagnostic(tc.message); got != tc.want {
			t.Errorf("incompleteAnalysisDiagnostic(%q) = %v, want %v", tc.message, got, tc.want)
		}
	}
}

func TestOpenGrepScannerRejectsBinaryDigestMismatchBeforeExecution(t *testing.T) {
	root := prepareOpenGrepFixture(t)
	binary := filepath.Join(t.TempDir(), "fake-opengrep")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{sarif: []byte(fixtureSARIF)}
	_, err := (OpenGrepScanner{
		Binary: binary, BinarySHA256: strings.Repeat("f", 64),
		Version: "1.33.0", ConfigPath: "rules/security.yml", Runner: runner,
	}).Scan(context.Background(), root)
	if err == nil || !strings.Contains(err.Error(), "binary SHA-256 mismatch") {
		t.Fatalf("expected binary integrity failure, got %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("unverified binary was executed: %+v", runner.calls)
	}
}

func TestOpenGrepScannerEnforcesSourceFileSizeLimitBeforeExecution(t *testing.T) {
	root := prepareOpenGrepFixture(t)
	path := filepath.Join(root, "src", "app.py")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 100)), 0600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{sarif: []byte(fixtureSARIF)}
	_, err := (OpenGrepScanner{
		Version: "1.33.0", ConfigPath: "rules/security.yml",
		MaxFileBytes: 32, SourceExtensions: []string{".py"}, Runner: runner,
	}).Scan(context.Background(), root)
	if err == nil || !strings.Contains(err.Error(), "exceeds the configured") {
		t.Fatalf("expected source-size limit failure, got %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("oversized source was scanned: %+v", runner.calls)
	}
}

func TestOpenGrepScannerHonorsExplicitSourceScope(t *testing.T) {
	root := prepareOpenGrepFixture(t)
	if err := os.WriteFile(filepath.Join(root, "src", "other.py"), []byte("print('other')\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{sarif: []byte(fixtureSARIF)}
	scanner := OpenGrepScanner{
		Binary: "/opt/opengrep", Version: "1.33.0", ConfigPath: "rules/security.yml",
		ScanPaths: []string{"src/app.py"}, Runner: runner,
	}
	got, err := scanner.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(got.ScannedPaths) != 1 || got.ScannedPaths[0] != "src/app.py" {
		t.Fatalf("scanned inventory did not preserve the explicit scope: %v", got.ScannedPaths)
	}
	scanArgs := runner.calls[1].args
	if !containsArg(scanArgs, "./src/app.py") {
		t.Fatalf("selected source path missing from args: %v", scanArgs)
	}
	if containsArg(scanArgs, ".") {
		t.Fatalf("scoped scan must not scan repository root: %v", scanArgs)
	}
	if containsArg(scanArgs, "./src/other.py") {
		t.Fatalf("out-of-scope file was passed to analyzer: %v", scanArgs)
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func TestOpenGrepScannerRejectsExcludedFixturePaths(t *testing.T) {
	root := prepareOpenGrepFixture(t)
	fixture := filepath.Join(root, filepath.FromSlash(analyzerFixtureDirectory), "vulnerable", "secret.go")
	if err := os.MkdirAll(filepath.Dir(fixture), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture, []byte("package vulnerable\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{sarif: []byte(fixtureSARIF)}
	scanner := OpenGrepScanner{
		Version: "1.33.0", ConfigPath: "rules/security.yml",
		ScanPaths: []string{analyzerFixtureDirectory + "/vulnerable/secret.go"}, Runner: runner,
	}
	if _, err := scanner.Scan(context.Background(), root); err == nil || !strings.Contains(err.Error(), "eligible analyzer source inventory") {
		t.Fatalf("expected excluded fixture path to be rejected, got %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("scanner must reject excluded paths before starting a scan; calls=%+v", runner.calls)
	}
}

func TestDiscoverScannedPathsIncludesAllEnabledRuleExtensions(t *testing.T) {
	root := prepareOpenGrepFixture(t)
	extensions := []string{".bash", ".dart", ".swift", ".lua", ".vb", ".ml"}
	for _, ext := range extensions {
		name := filepath.Join(root, "src", "fixture"+ext)
		if err := os.WriteFile(name, []byte("fixture\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := discoverScannedPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	inventory := make(map[string]bool, len(paths))
	for _, path := range paths {
		inventory[path] = true
	}
	for _, ext := range extensions {
		if !inventory["src/fixture"+ext] {
			t.Errorf("source inventory omitted enabled language extension %s: %v", ext, paths)
		}
	}
}
