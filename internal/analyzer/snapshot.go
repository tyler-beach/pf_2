package analyzer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// SnapshotFile is one eligible analyzer source path with content digest.
type SnapshotFile struct {
	Path      string
	SizeBytes int64
	SHA256    string
}

// InputSnapshot is a single-pass inventory of a repository revision: the full
// input-tree digest plus the eligible source inventory used by planning.
type InputSnapshot struct {
	InputSHA256 string
	Files       []SnapshotFile
}

var (
	hashFileMu    sync.Mutex
	hashFileCache = map[string]hashFileCacheEntry{}
)

type hashFileCacheEntry struct {
	size    int64
	modNano int64
	mode    os.FileMode
	digest  string
}

var sourceExtensions = map[string]bool{
	".go": true, ".py": true, ".js": true, ".jsx": true, ".mjs": true, ".cjs": true, ".ts": true, ".tsx": true,
	".java": true, ".cs": true, ".php": true, ".rb": true, ".json": true, ".yaml": true,
	".yml": true, ".tf": true, ".tfvars": true, ".hcl": true, ".sh": true, ".bash": true, ".zsh": true, ".fish": true,
	".kt": true, ".kts": true, ".scala": true, ".rs": true, ".c": true, ".h": true, ".cc": true,
	".cpp": true, ".hpp": true, ".dart": true, ".swift": true, ".lua": true, ".vb": true,
	".vbs": true, ".ml": true, ".mli": true, ".vue": true, ".html": true, ".xml": true,
	".sql": true, ".ex": true, ".exs": true, ".template": true, ".mod": true, ".toml": true,
}

var inputTreeSkipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".opengrep-cache": true, "analyzer-evidence": true, ".arch-go": true,
}

var inputTreeSkipFiles = map[string]bool{
	"assessment.json": true, "results.sarif": true, "report.md": true, "coverage.out": true,
	"ingested-adrs.json": true, "obligation-candidates.json": true, "obligation-assessment.json": true,
	"obligation-assessment.md": true, "obligation-candidates.md": true,
}

// SnapshotInputTree walks the repository once, computing the same input digest
// as HashInputTree while collecting eligible source digests for planning.
func SnapshotInputTree(root string) (InputSnapshot, error) {
	var empty InputSnapshot
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return empty, fmt.Errorf("resolve repository root: %w", err)
	}
	rootAbs, err = filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return empty, fmt.Errorf("resolve repository root symlinks: %w", err)
	}
	h := sha256.New()
	files := make([]SnapshotFile, 0, 64)
	err = filepath.WalkDir(rootAbs, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if current != rootAbs && inputTreeSkipDirs[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if inputTreeSkipFiles[entry.Name()] {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(rootAbs, current)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(relative)
		if _, err := io.WriteString(h, rel+"\x00"); err != nil {
			return err
		}
		data, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		if int64(len(data)) != info.Size() {
			return fmt.Errorf("file %s changed size while snapshotting", rel)
		}
		if _, err := h.Write(data); err != nil {
			return err
		}
		if _, err := io.WriteString(h, "\x00"); err != nil {
			return err
		}
		if !isEligibleAnalyzerSource(rel, "") {
			return nil
		}
		files = append(files, SnapshotFile{
			Path:      rel,
			SizeBytes: int64(len(data)),
			SHA256:    sha256Hex(data),
		})
		return nil
	})
	if err != nil {
		return empty, fmt.Errorf("snapshot repository input tree: %w", err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return InputSnapshot{InputSHA256: hex.EncodeToString(h.Sum(nil)), Files: files}, nil
}

// isEligibleAnalyzerSource reports whether a repository-relative path belongs in
// the analyzer source inventory. configRel, when non-empty, is excluded.
func isEligibleAnalyzerSource(rel, configRel string) bool {
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return false
	}
	if configRel != "" && rel == filepath.ToSlash(configRel) {
		return false
	}
	if rel == analyzerFixtureDirectory || strings.HasPrefix(rel, analyzerFixtureDirectory+"/") {
		return false
	}
	if rel == "adr/obligation-candidates.json" || rel == "adr/ingested-adrs.json" {
		return false
	}
	base := strings.ToLower(filepath.Base(rel))
	ext := strings.ToLower(filepath.Ext(base))
	if sourceExtensions[ext] {
		return true
	}
	return base == "dockerfile" || strings.HasPrefix(base, "dockerfile.") || base == "containerfile"
}

// HashFileCached returns HashFile(path) but reuses a process-local digest when
// the path's size, mtime, and mode are unchanged. Intended for pinned analyzer
// binaries that are hashed on every scan invocation.
func HashFileCached(file string) (string, error) {
	info, err := os.Lstat(file)
	if err != nil {
		return "", fmt.Errorf("inspect %s for hashing: %w", file, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return HashFile(file)
	}
	key := filepath.Clean(file)
	modNano := info.ModTime().UnixNano()
	hashFileMu.Lock()
	if entry, ok := hashFileCache[key]; ok &&
		entry.size == info.Size() && entry.modNano == modNano && entry.mode == info.Mode() {
		digest := entry.digest
		hashFileMu.Unlock()
		return digest, nil
	}
	hashFileMu.Unlock()

	digest, err := HashFile(file)
	if err != nil {
		return "", err
	}
	hashFileMu.Lock()
	hashFileCache[key] = hashFileCacheEntry{size: info.Size(), modNano: modNano, mode: info.Mode(), digest: digest}
	hashFileMu.Unlock()
	return digest, nil
}
