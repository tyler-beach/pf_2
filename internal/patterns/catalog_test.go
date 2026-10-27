package patterns

import (
	"path/filepath"
	"testing"
)

func TestLoadRepoPatternCatalog(t *testing.T) {
	path := filepath.Join("..", "..", "adr", "patterns.json")
	catalog, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Digest == "" || len(catalog.Digest) != 64 {
		t.Fatalf("missing digest: %+v", catalog)
	}
	if _, ok := catalog.Lookup("security.source_pattern"); !ok {
		t.Fatal("expected security.source_pattern")
	}
	if !catalog.CompatibleVerifier("security.source_pattern", "analyzer-rule") {
		t.Fatal("security pattern should accept analyzer-rule")
	}
	if catalog.CompatibleVerifier("manual_review", "analyzer-rule") {
		t.Fatal("manual_review must not accept analyzer-rule")
	}
}
