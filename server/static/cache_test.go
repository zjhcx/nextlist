package static

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
)

func TestCacheHTMLReplacesCompleteFile(t *testing.T) {
	previous := conf.DataDir
	conf.DataDir = t.TempDir()
	t.Cleanup(func() { conf.DataDir = previous })
	for _, content := range []string{"<html>first</html>", "<html>updated</html>"} {
		if err := cacheHTML("index.html", content); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(conf.DataDir, "cache", "web", "index.html"))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != content {
			t.Fatalf("cached content = %q, want %q", got, content)
		}
	}
	entries, err := os.ReadDir(filepath.Join(conf.DataDir, "cache", "web"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temporary cache files remain: %v", entries)
	}
}
