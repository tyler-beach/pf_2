package plan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyler-beach/pf_2/internal/analyzer"
)

const emptyCompleteSARIF = `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"Opengrep OSS Version","version":"1.30.0","rules":[]}},"results":[]}]}`

func executionFixture(t *testing.T, count int) (string, Manifest) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root,"rules"),0700); err != nil { t.Fatal(err) }
	specs := make([]AnalyzerSpec,0,count)
	for i:=0;i<count;i++ {
		area := string(rune('a'+i))
		source := filepath.Join(root,"src",area,"app.py")
		config := filepath.Join(root,"rules",area+".yml")
		if err := os.MkdirAll(filepath.Dir(source),0700); err != nil { t.Fatal(err) }
		if err := os.WriteFile(source,[]byte("print('safe')\n"),0600); err != nil { t.Fatal(err) }
		if err := os.WriteFile(config,[]byte("rules: []\n"),0600); err != nil { t.Fatal(err) }
		specs=append(specs,AnalyzerSpec{Name:"opengrep",Version:"1.30.0",BinarySHA256:strings.Repeat("b",64),ConfigurationPath:filepath.ToSlash(filepath.Join("rules",area+".yml")),ScopePrefixes:[]string{filepath.ToSlash(filepath.Join("src",area))},TimeoutMillis:1000,MaxRetries:1,MaxOutputBytes:1024*1024,RetryBackoffMillis:1,MaxRetryBackoffMillis:4,RetryableFailureKinds:[]string{FailureTimeout,FailureTemporaryProcessStart}})
	}
	manifest,err:=BuildManifest(root,specs,2)
	if err!=nil { t.Fatal(err) }
	return root,manifest
}

func validTaskEvidence(root string, task Task) (analyzer.ScanResult,error) {
	raw:=[]byte(emptyCompleteSARIF)
	result,err:=analyzer.ParseSARIF(raw,root)
	if err!=nil { return result,err }
	result.ConfigurationPath=task.ConfigurationPath
	result.ConfigurationSHA256,err=analyzer.HashFile(filepath.Join(root,filepath.FromSlash(task.ConfigurationPath)))
	if err!=nil { return result,err }
	result.RuleSetSHA256=result.ConfigurationSHA256
	result.BinarySHA256=task.BinarySHA256
	result.InputSHA256,err=analyzer.HashInputTree(root)
	if err!=nil { return result,err }
	paths,err:=analyzer.DiscoverScannedPaths(root)
	if err!=nil { return result,err }
	for _,p:=range paths { if p!=task.ConfigurationPath { result.ScannedPaths=append(result.ScannedPaths,p) } }
	return result,nil
}

func TestExecuteManifestBoundsConcurrencyAndProducesResults(t *testing.T) {
	root,manifest:=executionFixture(t,4)
	var active,maxActive,calls atomic.Int32
	runner:=TaskRunnerFunc(func(ctx context.Context, root string, task Task)(analyzer.ScanResult,error){
		current:=active.Add(1)
		defer active.Add(-1)
		for {
			old:=maxActive.Load()
			if current<=old || maxActive.CompareAndSwap(old,current) { break }
		}
		calls.Add(1)
		select { case <-time.After(15*time.Millisecond): case <-ctx.Done(): return analyzer.ScanResult{},ctx.Err() }
		return validTaskEvidence(root,task)
	})
	result,err:=ExecuteManifest(context.Background(),root,manifest,runner,nil)
	if err!=nil { t.Fatal(err) }
	if result.Status!=RunStatusComplete || len(result.Tasks)!=4 { t.Fatalf("unexpected run result: %+v",result) }
	if maxActive.Load()>int32(manifest.MaxConcurrency) { t.Fatalf("concurrency cap %d exceeded with %d active tasks",manifest.MaxConcurrency,maxActive.Load()) }
	if calls.Load()!=4 { t.Fatalf("runner called %d times, want 4",calls.Load()) }
	for _,task:=range result.Tasks {
		if task.Status!=TaskStatusComplete || task.Attempts!=1 || task.CacheHit { t.Errorf("unexpected task state: %+v",task) }
	}
}

func TestExecuteManifestRetriesOnlyConfiguredTransientFailure(t *testing.T) {
	root,manifest:=executionFixture(t,1)
	var calls atomic.Int32
	runner:=TaskRunnerFunc(func(ctx context.Context,root string,task Task)(analyzer.ScanResult,error){
		if calls.Add(1)==1 { return analyzer.ScanResult{},&RunFailure{Kind:FailureTimeout,Err:errors.New("temporary timeout")} }
		return validTaskEvidence(root,task)
	})
	result,err:=ExecuteManifest(context.Background(),root,manifest,runner,nil)
	if err!=nil { t.Fatal(err) }
	if result.Status!=RunStatusComplete || calls.Load()!=2 || result.Tasks[0].Attempts!=2 {
		t.Fatalf("expected a single bounded retry: result=%+v calls=%d",result,calls.Load())
	}
}

func TestExecuteManifestDoesNotRetryNonRetryableFailure(t *testing.T) {
	root,manifest:=executionFixture(t,1)
	var calls atomic.Int32
	runner:=TaskRunnerFunc(func(ctx context.Context,root string,task Task)(analyzer.ScanResult,error){
		calls.Add(1); return analyzer.ScanResult{},errors.New("malformed SARIF")
	})
	result,err:=ExecuteManifest(context.Background(),root,manifest,runner,nil)
	if err!=nil { t.Fatal(err) }
	if result.Status!=RunStatusFailed || result.Tasks[0].Status!=TaskStatusError || calls.Load()!=1 {
		t.Fatalf("non-retryable failure was retried or reported successful: %+v calls=%d",result,calls.Load())
	}
}

func TestExecuteManifestUsesVerifiedFileCache(t *testing.T) {
	root,manifest:=executionFixture(t,1)
	cache,err:=NewFileCache(t.TempDir())
	if err!=nil { t.Fatal(err) }
	var calls atomic.Int32
	runner:=TaskRunnerFunc(func(ctx context.Context,root string,task Task)(analyzer.ScanResult,error){
		calls.Add(1); return validTaskEvidence(root,task)
	})
	first,err:=ExecuteManifest(context.Background(),root,manifest,runner,cache)
	if err!=nil { t.Fatal(err) }
	if first.Status!=RunStatusComplete || first.Tasks[0].CacheHit { t.Fatalf("first run should scan: %+v",first) }
	second,err:=ExecuteManifest(context.Background(),root,manifest,runner,cache)
	if err!=nil { t.Fatal(err) }
	if second.Status!=RunStatusComplete || !second.Tasks[0].CacheHit || second.Tasks[0].Attempts!=0 {
		t.Fatalf("second run should use validated cache: %+v",second)
	}
	if calls.Load()!=1 { t.Fatalf("cache failed to suppress duplicate scan; calls=%d",calls.Load()) }
}

func TestExecuteManifestCacheCorruptionFailsClosed(t *testing.T) {
	root,manifest:=executionFixture(t,1)
	cacheRoot:=t.TempDir()
	cache,err:=NewFileCache(cacheRoot)
	if err!=nil { t.Fatal(err) }
	runner:=TaskRunnerFunc(func(ctx context.Context,root string,task Task)(analyzer.ScanResult,error){ return validTaskEvidence(root,task) })
	if _,err:=ExecuteManifest(context.Background(),root,manifest,runner,cache); err!=nil { t.Fatal(err) }
	entries,err:=filepath.Glob(filepath.Join(cacheRoot,"*.json"))
	if err!=nil || len(entries)!=1 { t.Fatalf("expected one cache entry, got %v / %v",entries,err) }
	if err:=os.WriteFile(entries[0],[]byte("{not-json"),0600); err!=nil { t.Fatal(err) }
	var calls atomic.Int32
	result,err:=ExecuteManifest(context.Background(),root,manifest,TaskRunnerFunc(func(ctx context.Context,root string,task Task)(analyzer.ScanResult,error){calls.Add(1);return validTaskEvidence(root,task)}),cache)
	if err!=nil { t.Fatal(err) }
	if result.Status!=RunStatusFailed || result.Tasks[0].Status!=TaskStatusError || calls.Load()!=0 {
		t.Fatalf("corrupt cache must fail closed and must not trigger a blind rescan: %+v calls=%d",result,calls.Load())
	}
}

func TestExecuteManifestEmptyScopeIsMissingEvidence(t *testing.T) {
	root,original:=executionFixture(t,1)
	task:=original.Tasks[0]
	manifest,err:=BuildManifest(root,[]AnalyzerSpec{{
		Name:task.Analyzer, Version:task.Version, BinarySHA256:task.BinarySHA256, RuleSetSHA256:task.RuleSetSHA256, ConfigurationPath:task.ConfigurationPath,
		ScopePrefixes:[]string{"internal/missing"}, TimeoutMillis:task.TimeoutMillis, MaxRetries:task.MaxAttempts-1,
		MaxOutputBytes:task.MaxOutputBytes, RetryBackoffMillis:task.RetryBackoffMillis,
		MaxRetryBackoffMillis:task.MaxRetryBackoffMillis, RetryableFailureKinds:task.RetryableFailureKinds,
	}},original.MaxConcurrency)
	if err!=nil { t.Fatal(err) }
	var calls atomic.Int32
	result,err:=ExecuteManifest(context.Background(),root,manifest,TaskRunnerFunc(func(ctx context.Context,root string,task Task)(analyzer.ScanResult,error){calls.Add(1);return validTaskEvidence(root,task)}),nil)
	if err!=nil { t.Fatal(err) }
	if result.Status!=RunStatusFailed || result.Tasks[0].Status!=TaskStatusEvidenceMissing || calls.Load()!=0 {
		t.Fatalf("empty scope must not execute or pass: %+v calls=%d",result,calls.Load())
	}
}

func TestExecuteManifestRejectsSourceDriftDuringExecution(t *testing.T) {
	root,manifest:=executionFixture(t,1)
	runner:=TaskRunnerFunc(func(ctx context.Context,root string,task Task)(analyzer.ScanResult,error){
		result,err:=validTaskEvidence(root,task)
		if err!=nil{return result,err}
		file:=filepath.Join(root,"src","a","app.py")
		if err:=os.WriteFile(file,[]byte("print('changed')\n"),0600);err!=nil{return analyzer.ScanResult{},err}
		return result,nil
	})
	result,err:=ExecuteManifest(context.Background(),root,manifest,runner,nil)
	if err==nil || result.Status!=RunStatusStaleInput || !strings.Contains(err.Error(),"source") {
		t.Fatalf("source drift must invalidate the run, got result=%+v err=%v",result,err)
	}
}

func TestExecuteManifestCancellationDoesNotRetry(t *testing.T) {
	root,manifest:=executionFixture(t,1)
	ctx,cancel:=context.WithCancel(context.Background()); cancel()
	var calls atomic.Int32
	result,err:=ExecuteManifest(ctx,root,manifest,TaskRunnerFunc(func(ctx context.Context,root string,task Task)(analyzer.ScanResult,error){calls.Add(1);return analyzer.ScanResult{},ctx.Err()}),nil)
	if err==nil || result.Status!=RunStatusFailed || calls.Load()!=0 {
		t.Fatalf("pre-cancelled run must not dispatch tasks: result=%+v calls=%d err=%v",result,calls.Load(),err)
	}
}
