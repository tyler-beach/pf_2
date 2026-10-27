package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tyler-beach/pf_2/internal/analyzer"
	"github.com/tyler-beach/pf_2/internal/plan"
)

func planFixtureRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{"src/app.py": "print('safe')\n", "rules/security.yml": "rules: []\n"}
	for rel, body := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil { t.Fatal(err) }
		if err := os.WriteFile(full, []byte(body), 0600); err != nil { t.Fatal(err) }
	}
	return root
}

func TestPlanRepoCLIIsDeterministicAndVerifyDetectsDrift(t *testing.T) {
	root := planFixtureRoot(t)
	output := filepath.Join(root, "analyzer-evidence", "plan.json")
	args := []string{"--root", root, "--analyzer", "opengrep", "--version", "1.30.0",
		"--config", "rules/security.yml", "--scope", "src/", "--binary-sha256", strings.Repeat("b",64), "--json-out", output}
	if err := runPlanRepo(args); err != nil { t.Fatal(err) }
	first, err := os.ReadFile(output)
	if err != nil { t.Fatal(err) }
	if err := runPlanRepo(args); err != nil { t.Fatal(err) }
	second, err := os.ReadFile(output)
	if err != nil { t.Fatal(err) }
	if !bytes.Equal(first, second) { t.Fatal("plan-repo CLI output is not deterministic") }
	if err := runVerifyPlan([]string{"--root", root, "--manifest", output}); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(root, "src", "app.py"), []byte("print('changed')\n"), 0600); err != nil { t.Fatal(err) }
	if err := runVerifyPlan([]string{"--root", root, "--manifest", output}); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("verify-plan should reject source drift, got %v", err)
	}
}


func TestExecutePlanCLIUsesCacheAndWritesRawSARIF(t *testing.T) {
	root:=planFixtureRoot(t)
	manifestPath:=filepath.Join(root,"analyzer-evidence","plan.json")
	if err:=runPlanRepo([]string{"--root",root,"--analyzer","opengrep","--version","1.30.0","--config","rules/security.yml","--scope","src/","--binary-sha256",strings.Repeat("b",64),"--json-out",manifestPath});err!=nil{t.Fatal(err)}
	cacheDir:=filepath.Join(t.TempDir(),"cache")
	runPath:=filepath.Join(root,"analyzer-evidence","execution.json")
	sarifDir:=filepath.Join(root,"analyzer-evidence","raw")
	var calls int
	runner:=plan.TaskRunnerFunc(func(ctx context.Context,root string,task plan.Task)(analyzer.ScanResult,error){
		calls++
		raw:=[]byte(`{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"Opengrep OSS Version","version":"1.30.0","rules":[]}},"results":[]}]}`)
		evidence,err:=analyzer.ParseSARIF(raw,root)
		if err!=nil{return evidence,err}
		evidence.ConfigurationPath=task.ConfigurationPath
		evidence.ConfigurationSHA256,err=analyzer.HashFile(filepath.Join(root,filepath.FromSlash(task.ConfigurationPath)));if err!=nil{return evidence,err}
		evidence.RuleSetSHA256=evidence.ConfigurationSHA256
		evidence.BinarySHA256=task.BinarySHA256
		evidence.InputSHA256,err=analyzer.HashInputTree(root);if err!=nil{return evidence,err}
		evidence.ScannedPaths=append([]string(nil),task.SourcePaths...)
		return evidence,nil
	})
	args:=[]string{"--root",root,"--manifest",manifestPath,"--cache-dir",cacheDir,"--json-out",runPath,"--sarif-dir",sarifDir}
	if err:=runExecutePlanWithRunner(args,runner);err!=nil{t.Fatal(err)}
	first,err:=os.ReadFile(runPath);if err!=nil{t.Fatal(err)}
	var decoded plan.RunResult
	if err:=json.Unmarshal(first,&decoded);err!=nil{t.Fatal(err)}
	if decoded.Status!=plan.RunStatusComplete||len(decoded.Tasks)!=1||decoded.Tasks[0].RawSARIFPath==""{t.Fatalf("missing execution evidence: %+v",decoded)}
	if _,err:=os.Stat(decoded.Tasks[0].RawSARIFPath);err!=nil{t.Fatalf("raw SARIF artifact not published: %v",err)}
	if err:=runExecutePlanWithRunner(args,runner);err!=nil{t.Fatal(err)}
	if calls!=1{t.Fatalf("second execution should use cache; runner calls=%d",calls)}
}
