package analyzer

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestParseExecutionStatusAcceptsOnlyDeclaredStates(t *testing.T) {
	for raw, want := range map[string]ExecutionStatus{
		"COMPLETE": ExecutionStatusComplete,
		"PARTIAL":  ExecutionStatusPartial,
		"ERROR":    ExecutionStatusError,
	} {
		got, err := ParseExecutionStatus(raw)
		if err != nil {
			t.Errorf("ParseExecutionStatus(%q) returned error: %v", raw, err)
		}
		if got != want {
			t.Errorf("ParseExecutionStatus(%q) = %q, want %q", raw, got, want)
		}
	}
	if _, err := ParseExecutionStatus("SUCCESS"); err == nil {
		t.Fatal("undeclared adapter status must be rejected")
	}
}

func TestTaskExecutorFromAdapterPreservesCleanAndFindingEvidence(t *testing.T) {
	snapshot := strings.Repeat("d", 64)
	task := AnalyzerTask{ID: "opengrep-task", Spec: planTestSpec("opengrep")}
	for _, tc := range []struct {
		name string
		addFinding bool
		wantFindings int
	}{
		{name: "clean result", wantFindings: 0},
		{name: "finding result", addFinding: true, wantFindings: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter := AnalyzerAdapterFunc(func(ctx context.Context, got AnalyzerTask) (ScanResult, error) {
				if got.ID != task.ID || got.Spec.Name != task.Spec.Name {
					t.Fatalf("adapter received wrong task: %+v", got)
				}
				result := planTestResult(got.Spec, snapshot, ScanStatusComplete)
				if tc.addFinding {
					result.Findings = append(result.Findings, Finding{
						Analyzer: "opengrep", AnalyzerVersion: "1.30.0",
						RuleID: "pathfinder.test-rule", Path: "internal/example.go",
						StartLine: 7, Message: "fixture finding", Fingerprint: "stable-fingerprint",
					})
				}
				return result, nil
			})
			executor := TaskExecutorFromAdapter(adapter)
			result, err := executor(context.Background(), task)
			if err != nil {
				t.Fatalf("execute adapter: %v", err)
			}
			if result.Status != ScanStatusComplete {
				t.Fatalf("status = %s, want COMPLETE", result.Status)
			}
			if len(result.Findings) != tc.wantFindings {
				t.Fatalf("findings = %d, want %d", len(result.Findings), tc.wantFindings)
			}
			if tc.addFinding && (result.Findings[0].Fingerprint != "stable-fingerprint" || result.Findings[0].Path != "internal/example.go") {
				t.Fatalf("normalized finding provenance was not preserved: %+v", result.Findings[0])
			}
		})
	}
}

func TestTaskExecutorFromAdapterFailClosesUnknownAndMismatchedStatuses(t *testing.T) {
	task := AnalyzerTask{ID: "task", Spec: planTestSpec("opengrep")}
	snapshot := strings.Repeat("d", 64)
	tests := []struct {
		name string
		status string
		runErr error
		wantError string
	}{
		{name: "unknown status", status: "SUCCESS", wantError: "unsupported adapter execution status"},
		{name: "error hidden behind complete", status: ScanStatusComplete, runErr: errors.New("scanner timed out"), wantError: "COMPLETE status"},
		{name: "error status without diagnostic", status: ScanStatusError, wantError: "ERROR status without an error"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			adapter := AnalyzerAdapterFunc(func(context.Context, AnalyzerTask) (ScanResult, error) {
				return planTestResult(task.Spec, snapshot, tc.status), tc.runErr
			})
			result, err := TaskExecutorFromAdapter(adapter)(context.Background(), task)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("expected adapter contract error containing %q, got %v", tc.wantError, err)
			}
			if result.Status != ScanStatusError {
				t.Fatalf("invalid adapter result must fail closed as ERROR, got %q", result.Status)
			}
			if len(result.Errors) == 0 {
				t.Fatal("adapter contract failure must be recorded in normalized errors")
			}
		})
	}
}

func TestTaskExecutorFromAdapterPreservesPartialStatusAndDiagnostic(t *testing.T) {
	task := AnalyzerTask{ID: "task", Spec: planTestSpec("opengrep")}
	adapterErr := errors.New("scanner timed out after producing partial SARIF")
	adapter := AnalyzerAdapterFunc(func(context.Context, AnalyzerTask) (ScanResult, error) {
		result := planTestResult(task.Spec, strings.Repeat("d", 64), ScanStatusPartial)
		result.Errors = []string{"analysis timed out"}
		return result, adapterErr
	})
	result, err := TaskExecutorFromAdapter(adapter)(context.Background(), task)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("partial execution error was lost: %v", err)
	}
	if result.Status != ScanStatusPartial || len(result.Errors) == 0 {
		t.Fatalf("partial status and diagnostics must be preserved: %+v", result)
	}
}

func TestOpenGrepTaskAdapterRejectsUnsupportedAnalyzerBeforeLaunch(t *testing.T) {
	runner := &fakeRunner{}
	adapter := OpenGrepTaskAdapter{
		Root: t.TempDir(),
		Scanner: OpenGrepScanner{
			Version: "1.30.0",
			ConfigPath: "rules/security.yml",
			Runner: runner,
		},
	}
	task := AnalyzerTask{ID: "conftest-task", Spec: planTestSpec("conftest")}
	result, err := adapter.Execute(context.Background(), task)
	if err == nil || !strings.Contains(err.Error(), "does not support analyzer") {
		t.Fatalf("expected unsupported analyzer rejection, got %v", err)
	}
	if result.Status != ScanStatusError || len(result.Errors) == 0 {
		t.Fatalf("unsupported adapter request must be recorded as ERROR: %+v", result)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("unsupported analyzer request must be rejected before running a binary: %+v", runner.calls)
	}
}
