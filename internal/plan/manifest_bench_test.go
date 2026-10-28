package plan

import (
	"os"
	"testing"
)

func BenchmarkBuildManifest(b *testing.B) {
	root := os.Getenv("PF_BENCH_ROOT")
	if root == "" {
		b.Skip("PF_BENCH_ROOT unset")
	}
	specs := []AnalyzerSpec{{
		Name: "opengrep", Version: "1.33.0",
		BinarySHA256:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ConfigurationPath: "rules/pathfinder-security.yml",
		ScopePrefixes:     []string{"."},
	}}
	if _, err := BuildManifest(root, specs, 2); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := BuildManifest(root, specs, 2); err != nil {
			b.Fatal(err)
		}
	}
}
