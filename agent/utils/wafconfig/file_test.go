package wafconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomicReplacesConfigAndCleansTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "gateway.json")
	if err := WriteAtomic(path, []byte("old"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(path, []byte("new"), 0640); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "new" {
		t.Fatalf("read=%q err=%v", got, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "gateway.json" {
		t.Fatalf("temporary file leaked: %#v", entries)
	}
}
