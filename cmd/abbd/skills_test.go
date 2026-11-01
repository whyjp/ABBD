package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func readPlugin(t *testing.T, rel ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(append([]string{"..", "..", "plugins", "abbd"}, rel...)...))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The plugin ships only its skills: no hooks and no agent of its own.
func TestThePluginShipsOnlySkills(t *testing.T) {
	var manifest struct{ Name string }
	if err := json.Unmarshal([]byte(readPlugin(t, ".claude-plugin", "plugin.json")), &manifest); err != nil || manifest.Name != "abbd" {
		t.Fatalf("plugin.json must parse and name the plugin abbd: %v %+v", err, manifest)
	}
	for _, gone := range []string{"hooks", "agents"} {
		if _, err := os.Stat(filepath.Join("..", "..", "plugins", "abbd", gone)); err == nil {
			t.Errorf("the plugin must not ship %s/", gone)
		}
	}
}
