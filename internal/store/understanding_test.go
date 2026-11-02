package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// A text stored against the empty digest would read current whatever its
// module's code did, so the store refuses it and writes nothing.
func TestATextWithoutADigestIsNotStored(t *testing.T) {
	root := t.TempDir()
	s := Open(root)
	if err := s.SaveUnderstanding(Understanding{Unit: "MOD-0001", Origin: model.Inferred, Text: "the module."}); err == nil {
		t.Fatal("a text without a digest must be refused")
	}
	if _, err := os.Stat(filepath.Join(root, Dir, "understanding", "MOD-0001.md")); !os.IsNotExist(err) {
		t.Fatalf("nothing may be written for a refused text: %v", err)
	}
}
