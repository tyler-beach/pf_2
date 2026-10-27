package plan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tyler-beach/pf_2/internal/analyzer"
)

const cacheSchemaVersion = "1"
const maxCacheEntryBytes int64 = (512 << 20) + (8 << 20)

// FileCache stores only verified complete analyzer evidence. Cache keys are
// validated as SHA-256 hex before being used as paths; entries are immutable,
// private, and atomically published.
type FileCache struct { Root string }

type cacheRecord struct {
	SchemaVersion  string              `json:"schema_version"`
	CacheKey       string              `json:"cache_key"`
	TaskID         string              `json:"task_id"`
	Analyzer       string              `json:"analyzer"`
	IdempotencyKey string              `json:"idempotency_key"`
	Evidence       analyzer.ScanResult `json:"evidence"`
	RawSARIF       []byte              `json:"raw_sarif"`
}

func NewFileCache(root string) (*FileCache,error) {
	if strings.TrimSpace(root)=="" { return nil,fmt.Errorf("cache root is required") }
	absolute,err:=filepath.Abs(root)
	if err!=nil { return nil,fmt.Errorf("resolve cache root: %w",err) }
	if err:=os.MkdirAll(absolute,0700);err!=nil{return nil,fmt.Errorf("create cache root: %w",err)}
	info,err:=os.Lstat(absolute)
	if err!=nil{return nil,fmt.Errorf("inspect cache root: %w",err)}
	if info.Mode()&os.ModeSymlink!=0||!info.IsDir(){return nil,fmt.Errorf("cache root must be a real directory, not a symlink")}
	real,err:=filepath.EvalSymlinks(absolute)
	if err!=nil{return nil,fmt.Errorf("resolve cache root: %w",err)}
	if err:=os.Chmod(real,0700);err!=nil{return nil,fmt.Errorf("restrict cache root permissions: %w",err)}
	return &FileCache{Root:real},nil
}

func (c *FileCache) Load(cacheKey string)(TaskExecution,bool,error) {
	var empty TaskExecution
	if !validDigest(cacheKey){return empty,false,fmt.Errorf("invalid cache key")}
	if c==nil||strings.TrimSpace(c.Root)==""{return empty,false,fmt.Errorf("cache root is empty")}
	if err:=c.validateRoot();err!=nil{return empty,false,err}
	file:=filepath.Join(c.Root,cacheKey+".json")
	info,err:=os.Lstat(file)
	if os.IsNotExist(err){return empty,false,nil}
	if err!=nil{return empty,false,fmt.Errorf("inspect cache entry: %w",err)}
	if info.Mode()&os.ModeSymlink!=0||!info.Mode().IsRegular(){return empty,false,fmt.Errorf("cache entry must be a regular non-symlink file")}
	if info.Mode().Perm()&0077!=0{return empty,false,fmt.Errorf("cache entry permissions are not private")}
	if info.Size()<1||info.Size()>maxCacheEntryBytes{return empty,false,fmt.Errorf("cache entry size %d is invalid",info.Size())}
	data,err:=os.ReadFile(file)
	if err!=nil{return empty,false,fmt.Errorf("read cache entry: %w",err)}
	decoder:=json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record cacheRecord
	if err:=decoder.Decode(&record);err!=nil{return empty,false,fmt.Errorf("decode cache entry: %w",err)}
	var trailing any
	if err:=decoder.Decode(&trailing);err!=io.EOF{
		if err==nil{return empty,false,fmt.Errorf("cache entry has trailing JSON value")}
		return empty,false,fmt.Errorf("cache entry has trailing content: %w",err)
	}
	if record.SchemaVersion!=cacheSchemaVersion||record.CacheKey!=cacheKey{return empty,false,fmt.Errorf("cache entry schema or key mismatch")}
	if record.TaskID==""||record.Analyzer==""||record.IdempotencyKey==""||len(record.RawSARIF)==0 {
		return empty,false,fmt.Errorf("cache entry is missing required identity or raw SARIF")
	}
	record.Evidence.RawSARIF=append([]byte(nil),record.RawSARIF...)
	result:=TaskExecution{TaskID:record.TaskID,Analyzer:record.Analyzer,CacheKey:record.CacheKey,IdempotencyKey:record.IdempotencyKey,
		Status:TaskStatusComplete,Attempts:0,CacheHit:true,Evidence:&record.Evidence,RawSARIF:append([]byte(nil),record.RawSARIF...)}
	return result,true,nil
}

func (c *FileCache) Store(cacheKey string,result TaskExecution) error {
	if !validDigest(cacheKey){return fmt.Errorf("invalid cache key")}
	if c==nil||strings.TrimSpace(c.Root)==""{return fmt.Errorf("cache root is empty")}
	if err:=c.validateRoot();err!=nil{return err}
	if result.Status!=TaskStatusComplete||result.Evidence==nil||len(result.RawSARIF)==0 {
		return fmt.Errorf("only complete results with normalized evidence and raw SARIF may be cached")
	}
	if result.CacheKey!=cacheKey||result.TaskID==""||result.Analyzer==""||result.IdempotencyKey=="" {
		return fmt.Errorf("cache result identity does not match cache key")
	}
	evidence:=*result.Evidence
	evidence.RawSARIF=nil
	record:=cacheRecord{SchemaVersion:cacheSchemaVersion,CacheKey:cacheKey,TaskID:result.TaskID,Analyzer:result.Analyzer,
		IdempotencyKey:result.IdempotencyKey,Evidence:evidence,RawSARIF:append([]byte(nil),result.RawSARIF...)}
	data,err:=json.MarshalIndent(record,"","  ")
	if err!=nil{return fmt.Errorf("encode cache record: %w",err)}
	data=append(data,'\n')
	target:=filepath.Join(c.Root,cacheKey+".json")
	if existing,err:=os.Lstat(target);err==nil{
		if existing.Mode()&os.ModeSymlink!=0||!existing.Mode().IsRegular(){return fmt.Errorf("existing cache entry is not a regular file")}
		current,readErr:=os.ReadFile(target)
		if readErr!=nil{return fmt.Errorf("read existing cache entry: %w",readErr)}
		if bytes.Equal(current,data){return nil}
		return fmt.Errorf("refusing to overwrite existing cache entry with different content")
	}else if !os.IsNotExist(err){return fmt.Errorf("inspect target cache entry: %w",err)}
	temp,err:=os.CreateTemp(c.Root,".cache-tmp-*")
	if err!=nil{return fmt.Errorf("create temporary cache entry: %w",err)}
	tempPath:=temp.Name()
	defer os.Remove(tempPath)
	if err:=temp.Chmod(0600);err!=nil{_ = temp.Close();return fmt.Errorf("restrict temporary cache entry: %w",err)}
	if _,err:=temp.Write(data);err!=nil{_ = temp.Close();return fmt.Errorf("write temporary cache entry: %w",err)}
	if err:=temp.Sync();err!=nil{_ = temp.Close();return fmt.Errorf("sync temporary cache entry: %w",err)}
	if err:=temp.Close();err!=nil{return fmt.Errorf("close temporary cache entry: %w",err)}
	if err:=os.Link(tempPath,target);err!=nil{
		if os.IsExist(err){
			current,readErr:=os.ReadFile(target)
			if readErr==nil&&bytes.Equal(current,data){return nil}
			if readErr!=nil{return fmt.Errorf("read concurrently created cache entry: %w",readErr)}
			return fmt.Errorf("refusing to overwrite concurrently created cache entry with different content")
		}
		return fmt.Errorf("publish immutable cache entry: %w",err)
	}
	if directory,err:=os.Open(c.Root);err==nil{_ = directory.Sync();_ = directory.Close()}
	return nil
}

func (c *FileCache) validateRoot()error{
	info,err:=os.Lstat(c.Root)
	if err!=nil{return fmt.Errorf("inspect cache root: %w",err)}
	if info.Mode()&os.ModeSymlink!=0||!info.IsDir(){return fmt.Errorf("cache root is no longer a real directory")}
	if info.Mode().Perm()&0077!=0{return fmt.Errorf("cache root permissions are not private")}
	real,err:=filepath.EvalSymlinks(c.Root)
	if err!=nil{return fmt.Errorf("resolve cache root: %w",err)}
	if filepath.Clean(real)!=filepath.Clean(c.Root){return fmt.Errorf("cache root path traverses a symlink")}
	return nil
}

func validDigest(value string)bool{
	if len(value)!=64{return false}
	for _,r:=range value { if !((r>='0'&&r<='9')||(r>='a'&&r<='f')){return false} }
	return true
}
