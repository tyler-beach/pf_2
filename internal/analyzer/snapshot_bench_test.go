package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkSnapshotInputTree(b *testing.B) {
	root := os.Getenv("PF_BENCH_ROOT")
	if root == "" {
		root = filepath.Join("..", "..")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := SnapshotInputTree(root); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHashInputTree(b *testing.B) {
	root := os.Getenv("PF_BENCH_ROOT")
	if root == "" {
		root = filepath.Join("..", "..")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := HashInputTree(root); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDiscoverScannedPaths(b *testing.B) {
	root := os.Getenv("PF_BENCH_ROOT")
	if root == "" {
		root = filepath.Join("..", "..")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := DiscoverScannedPaths(root); err != nil {
			b.Fatal(err)
		}
	}
}
