package plan

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tyler-beach/pf_2/internal/analyzer"
)

const (
	RunStatusComplete  = "COMPLETE"
	RunStatusPartial   = "PARTIAL"
	RunStatusFailed    = "FAILED"
	RunStatusStaleInput = "STALE_INPUT"

	TaskStatusComplete        = "COMPLETE"
	TaskStatusPartial         = "PARTIAL"
	TaskStatusError           = "ERROR"
	TaskStatusEvidenceMissing = "EVIDENCE_MISSING"
	TaskStatusCancelled       = "CANCELLED"

	FailureTimeout                = "timeout"
	FailureTemporaryProcessStart  = "temporary_process_start"
	FailureAnalyzer               = "analyzer_error"
	FailureUnsupported            = "unsupported"
)

type TaskRunner interface {
	Run(ctx context.Context, root string, task Task) (analyzer.ScanResult, error)
}

type TaskRunnerFunc func(ctx context.Context, root string, task Task) (analyzer.ScanResult, error)

func (f TaskRunnerFunc) Run(ctx context.Context, root string, task Task) (analyzer.ScanResult, error) {
	return f(ctx, root, task)
}

// RunFailure is a classified task error. The executor retries only failure kinds
// explicitly allow-listed by the signed-off plan policy.
type RunFailure struct {
	Kind string
	Err  error
}

func (e *RunFailure) Error() string {
	if e == nil { return "<nil>" }
	if e.Err == nil { return e.Kind }
	return e.Kind + ": " + e.Err.Error()
}
func (e *RunFailure) Unwrap() error { if e==nil{return nil}; return e.Err }

type TaskExecution struct {
	TaskID         string              `json:"task_id"`
	Analyzer       string              `json:"analyzer"`
	BinarySHA256   string              `json:"binary_sha256,omitempty"`
	RuleSetSHA256  string              `json:"ruleset_sha256,omitempty"`
	CacheKey       string              `json:"cache_key"`
	IdempotencyKey string              `json:"idempotency_key"`
	Status         string              `json:"status"`
	Attempts       int                 `json:"attempts"`
	CacheHit       bool                `json:"cache_hit"`
	Evidence       *analyzer.ScanResult `json:"evidence,omitempty"`
	RawSARIF       []byte              `json:"-"`
	RawSARIFPath   string              `json:"raw_sarif_path,omitempty"`
	Error          string              `json:"error,omitempty"`
}

type RunResult struct {
	SchemaVersion      string          `json:"schema_version"`
	PlanManifestSHA256 string          `json:"plan_manifest_sha256"`
	InputSHA256        string          `json:"input_sha256"`
	Status             string          `json:"status"`
	Tasks              []TaskExecution `json:"tasks"`
}

type ResultCache interface {
	Load(cacheKey string) (TaskExecution, bool, error)
	Store(cacheKey string, result TaskExecution) error
}

// OpenGrepTaskRunner adapts the source-bound plan to the current OpenGrep CLI.
// It never invokes a shell; task timeouts are enforced by ExecuteManifest.
type OpenGrepTaskRunner struct { Binary string }

func (r OpenGrepTaskRunner) Run(ctx context.Context, root string, task Task) (analyzer.ScanResult, error) {
	if !strings.EqualFold(strings.TrimSpace(task.Analyzer), "opengrep") {
		return analyzer.ScanResult{}, &RunFailure{Kind:FailureUnsupported,Err:fmt.Errorf("no executor registered for analyzer %q",task.Analyzer)}
	}
	if task.BinarySHA256 == "" || !validDigest(task.BinarySHA256) {
		return analyzer.ScanResult{}, &RunFailure{Kind:FailureAnalyzer,Err:fmt.Errorf("OpenGrep plan must pin binary_sha256")}
	}
	if task.RuleSetSHA256 == "" || !validDigest(task.RuleSetSHA256) {
		return analyzer.ScanResult{}, &RunFailure{Kind:FailureAnalyzer,Err:fmt.Errorf("OpenGrep plan must pin ruleset_sha256")}
	}
	scanner:=analyzer.OpenGrepScanner{Binary:r.Binary,BinarySHA256:task.BinarySHA256,RuleSetSHA256:task.RuleSetSHA256,Version:task.Version,ConfigPath:task.ConfigurationPath,ScanPaths:append([]string(nil),task.SourcePaths...)}
	result,err:=scanner.Scan(ctx,root)
	if err==nil { return result,nil }
	kind:=FailureAnalyzer
	if errors.Is(ctx.Err(),context.DeadlineExceeded) {
		kind=FailureTimeout
	} else if strings.Contains(strings.ToLower(err.Error()),"could not start analyzer") {
		kind=FailureTemporaryProcessStart
	}
	return result,&RunFailure{Kind:kind,Err:err}
}

// ExecuteManifest verifies the plan before dispatch and after all task results.
// It runs task adapters with bounded parallelism; incomplete/invalid output never
// enters the cache and a plan/source drift invalidates the overall run.
func ExecuteManifest(ctx context.Context, root string, manifest Manifest, runner TaskRunner, cache ResultCache) (RunResult,error) {
	out:=RunResult{SchemaVersion:"1",PlanManifestSHA256:manifest.ManifestSHA256,InputSHA256:manifest.InputSHA256,Tasks:make([]TaskExecution,len(manifest.Tasks))}
	if ctx==nil { out.Status=RunStatusFailed; return out,fmt.Errorf("execution context is nil") }
	if runner==nil { out.Status=RunStatusFailed; return out,fmt.Errorf("task runner is required") }
	if err:=VerifyManifest(root,manifest);err!=nil { out.Status=RunStatusFailed; return out,fmt.Errorf("verify plan before execution: %w",err) }
	if ctx.Err()!=nil {
		for i,task:=range manifest.Tasks { out.Tasks[i]=newTaskResult(task,TaskStatusCancelled,ctx.Err().Error()) }
		out.Status=RunStatusFailed
		return out,ctx.Err()
	}
	limit:=manifest.MaxConcurrency
	if limit<1||limit>MaxConcurrency { out.Status=RunStatusFailed; return out,fmt.Errorf("invalid manifest concurrency limit %d",limit) }
	semaphore:=make(chan struct{},limit)
	var wg sync.WaitGroup
	for i,task:=range manifest.Tasks {
		wg.Add(1)
		go func(index int, task Task) {
			defer wg.Done()
			select {
			case semaphore<-struct{}{}:
			case <-ctx.Done():
				out.Tasks[index]=newTaskResult(task,TaskStatusCancelled,ctx.Err().Error())
				return
			}
			defer func(){<-semaphore}()
			out.Tasks[index]=executeOne(ctx,root,task,runner,cache)
		}(i,task)
	}
	wg.Wait()
	if err:=VerifyManifest(root,manifest);err!=nil {
		out.Status=RunStatusStaleInput
		return out,fmt.Errorf("repository source changed during execution or plan became stale: %w",err)
	}
	out.Status=aggregateRunStatus(out.Tasks)
	if ctx.Err()!=nil { return out,ctx.Err() }
	return out,nil
}

func executeOne(ctx context.Context,root string,task Task,runner TaskRunner,cache ResultCache) TaskExecution {
	if task.InputStatus!="READY" || len(task.SourcePaths)==0 {
		return newTaskResult(task,TaskStatusEvidenceMissing,"declared analyzer scope contains no source paths; analyzer was not executed")
	}
	if cache!=nil {
		cached,found,err:=cache.Load(task.CacheKey)
		if err!=nil { return newTaskResult(task,TaskStatusError,"read analyzer cache: "+err.Error()) }
		if found {
			if cached.CacheKey!=task.CacheKey||cached.TaskID!=task.ID||cached.IdempotencyKey!=task.IdempotencyKey||
				cached.BinarySHA256!=task.BinarySHA256||cached.RuleSetSHA256!=task.RuleSetSHA256 {
				return newTaskResult(task,TaskStatusError,"cache entry identity does not match the planned task")
			}
			if cached.Evidence==nil { return newTaskResult(task,TaskStatusError,"cache entry is missing normalized analyzer evidence") }
			verified,err:=verifyTaskEvidence(root,task,*cached.Evidence,cached.RawSARIF)
			if err!=nil { return newTaskResult(task,TaskStatusError,"cached analyzer evidence failed verification: "+err.Error()) }
			cached.Status=TaskStatusComplete
			cached.Attempts=0
			cached.CacheHit=true
			cached.Error=""
			cached.Evidence=&verified
			cached.RawSARIF=append([]byte(nil),cached.RawSARIF...)
			return cached
		}
	}
	result:=newTaskResult(task,TaskStatusError,"")
	maxAttempts:=task.MaxAttempts
	if maxAttempts<1||maxAttempts>MaxRetryCount+1 { result.Error=fmt.Sprintf("invalid planned attempt count %d",maxAttempts);return result }
	for attempt:=1;attempt<=maxAttempts;attempt++ {
		if ctx.Err()!=nil {
			result.Status=TaskStatusCancelled
			result.Attempts=attempt-1
			result.Error=ctx.Err().Error()
			return result
		}
		taskCtx,cancel:=context.WithTimeout(ctx,time.Duration(task.TimeoutMillis)*time.Millisecond)
		evidence,runErr:=runner.Run(taskCtx,root,task)
		taskCtxErr:=taskCtx.Err()
		cancel()
		result.Attempts=attempt
		if runErr==nil {
			if evidence.Status!=analyzer.ScanStatusComplete {
				status:=TaskStatusError
				if evidence.Status==analyzer.ScanStatusPartial { status=TaskStatusPartial }
				result.Status=status
				result.Error=fmt.Sprintf("analyzer returned non-complete evidence status %q",evidence.Status)
				if len(evidence.RawSARIF)>0 { result.RawSARIF=append([]byte(nil),evidence.RawSARIF...);result.Evidence=&evidence }
				return result
			}
			raw:=append([]byte(nil),evidence.RawSARIF...)
			if len(raw)==0 { result.Status=TaskStatusError;result.Error="analyzer returned no raw SARIF bytes";return result }
			if int64(len(raw))>task.MaxOutputBytes { result.Status=TaskStatusError;result.Error=fmt.Sprintf("raw analyzer output is %d bytes, above the planned %d-byte limit",len(raw),task.MaxOutputBytes);return result }
			verified,err:=verifyTaskEvidence(root,task,evidence,raw)
			if err!=nil { result.Status=TaskStatusError;result.Error="analyzer evidence failed verification: "+err.Error();result.RawSARIF=raw;result.Evidence=&evidence;return result }
			result.Status=TaskStatusComplete
			result.Evidence=&verified
			result.RawSARIF=raw
			if cache!=nil {
				if err:=cache.Store(task.CacheKey,result);err!=nil {
					result.Status=TaskStatusError
					result.Error="store validated analyzer evidence in cache: "+err.Error()
					return result
				}
			}
			return result
		}
		result.Error=runErr.Error()
		result.Status=TaskStatusError
		if len(evidence.RawSARIF)>0 { result.RawSARIF=append([]byte(nil),evidence.RawSARIF...);result.Evidence=&evidence }
		if errors.Is(ctx.Err(),context.Canceled) {
			result.Status=TaskStatusCancelled
			result.Error=ctx.Err().Error()
			return result
		}
		if attempt==maxAttempts || !retryableFailure(runErr,task.RetryableFailureKinds) { return result }
		backoff:=retryDelay(task.RetryBackoffMillis,task.MaxRetryBackoffMillis,attempt)
		if backoff<=0 { continue }
		timer:=time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			if !timer.Stop() { <-timer.C }
			result.Status=TaskStatusCancelled
			result.Error=ctx.Err().Error()
			return result
		case <-timer.C:
		}
		_ = taskCtxErr
	}
	return result
}

func newTaskResult(task Task,status,reason string) TaskExecution {
	return TaskExecution{TaskID:task.ID,Analyzer:task.Analyzer,BinarySHA256:task.BinarySHA256,RuleSetSHA256:task.RuleSetSHA256,CacheKey:task.CacheKey,IdempotencyKey:task.IdempotencyKey,Status:status,Error:reason}
}

func retryableFailure(err error,allowed []string) bool {
	var classified *RunFailure
	if !errors.As(err,&classified) { return false }
	for _,kind:=range allowed { if strings.EqualFold(strings.TrimSpace(kind),classified.Kind) { return true } }
	return false
}

func retryDelay(initialMillis,maxMillis int64,attempt int) time.Duration {
	if initialMillis<=0||maxMillis<=0 { return 0 }
	value:=initialMillis
	for i:=1;i<attempt;i++ {
		if value>=maxMillis/2 { value=maxMillis;break }
		value*=2
	}
	if value>maxMillis { value=maxMillis }
	return time.Duration(value)*time.Millisecond
}

func verifyTaskEvidence(root string,task Task,evidence analyzer.ScanResult,raw []byte)(analyzer.ScanResult,error) {
	if evidence.Status!=analyzer.ScanStatusComplete { return evidence,fmt.Errorf("status is %s, not COMPLETE",evidence.Status) }
	if len(raw)==0 { return evidence,fmt.Errorf("raw SARIF is empty") }
	if int64(len(raw))>task.MaxOutputBytes { return evidence,fmt.Errorf("raw SARIF exceeds configured output limit") }
	verified,err:=analyzer.VerifyScopedScanEvidence(root,evidence,raw,task.SourcePaths)
	if err!=nil { return verified,err }
	if verified.InputSHA256!=task.InputSHA256 { return verified,fmt.Errorf("input SHA-256 does not match planned snapshot") }
	if task.BinarySHA256!="" && verified.BinarySHA256!=task.BinarySHA256 { return verified,fmt.Errorf("binary SHA-256 does not match planned executable") }
	if task.RuleSetSHA256!="" && verified.RuleSetSHA256!=task.RuleSetSHA256 { return verified,fmt.Errorf("ruleset SHA-256 does not match planned rule set") }
	if verified.ConfigurationPath!=task.ConfigurationPath||verified.ConfigurationSHA256!=task.ConfigurationSHA256 {
		return verified,fmt.Errorf("configuration identity/digest does not match planned task")
	}
	found:=false
	for _,info:=range verified.Analyzers {
		name:=strings.ToLower(strings.TrimSpace(info.Name))
		if name==strings.ToLower(task.Analyzer)||strings.Contains(name,strings.ToLower(task.Analyzer)) {
			if info.Version!=task.Version { return verified,fmt.Errorf("analyzer version %q does not match planned version %q",info.Version,task.Version) }
			found=true
		}
	}
	if !found { return verified,fmt.Errorf("output contains no result from planned analyzer %q",task.Analyzer) }
	inventory:=make(map[string]bool,len(verified.ScannedPaths))
	for _,source:=range verified.ScannedPaths { inventory[filepath.ToSlash(source)]=true }
	for _,source:=range task.SourcePaths {
		if !inventory[filepath.ToSlash(source)] { return verified,fmt.Errorf("planned source path %q is absent from verified scan inventory",source) }
	}
	verified.RawSARIF=append([]byte(nil),raw...)
	return verified,nil
}

func aggregateRunStatus(tasks []TaskExecution) string {
	if len(tasks)==0 { return RunStatusFailed }
	completed:=0
	for _,task:=range tasks { if task.Status==TaskStatusComplete { completed++ } }
	if completed==len(tasks) { return RunStatusComplete }
	if completed==0 { return RunStatusFailed }
	return RunStatusPartial
}
