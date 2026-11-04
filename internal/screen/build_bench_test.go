package screen

import (
	"path/filepath"
	"testing"

	"github.com/whyjp/abbd/internal/store"
)

// BenchmarkBuild times Build on each real state, the units already loaded.
func BenchmarkBuild(b *testing.B) {
	for _, name := range fixtures {
		units, err := store.Open(filepath.Join("testdata", name)).LoadUnits()
		if err != nil {
			b.Fatal(err)
		}
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				Build(Input{Units: units})
			}
		})
	}
}
