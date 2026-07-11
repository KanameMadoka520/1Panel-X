package clam

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareAndRemoveInfectedDirectory(t *testing.T) {
	baseDir := t.TempDir()
	runDir, err := PrepareInfectedDirectory(baseDir, "daily scan", "20260710-120000")
	if err != nil {
		t.Fatalf("PrepareInfectedDirectory() error = %v", err)
	}
	info, err := os.Stat(runDir)
	if err != nil {
		t.Fatalf("stat prepared directory: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("prepared directory mode = %o, want 700", info.Mode().Perm())
	}
	if err := RemoveInfectedDirectory(baseDir, "daily scan"); err != nil {
		t.Fatalf("RemoveInfectedDirectory() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "1panel-infected", "daily scan")); !os.IsNotExist(err) {
		t.Fatalf("expected rule directory to be removed, stat error = %v", err)
	}
}

func TestRemoveInfectedDirectoryRejectsTraversalAndSymlink(t *testing.T) {
	baseDir := t.TempDir()
	outsideDir := t.TempDir()
	if err := RemoveInfectedDirectory(baseDir, "../../escape"); err == nil {
		t.Fatal("expected traversal rule name to be rejected")
	}

	root := filepath.Join(baseDir, "1panel-infected")
	if err := os.Symlink(outsideDir, root); err != nil {
		t.Skipf("create quarantine symlink: %v", err)
	}
	if err := RemoveInfectedDirectory(baseDir, "safe-name"); err == nil {
		t.Fatal("expected symlinked quarantine root to be rejected")
	}
	if _, err := os.Stat(outsideDir); err != nil {
		t.Fatalf("outside directory was affected: %v", err)
	}
}

func TestRemoveInfectedDirectoryRejectsFilesystemRoot(t *testing.T) {
	baseDir := t.TempDir()
	volume := filepath.VolumeName(baseDir)
	filesystemRoot := volume + string(filepath.Separator)
	if volume == "" {
		filesystemRoot = string(filepath.Separator)
	}

	err := RemoveInfectedDirectory(filesystemRoot, "legacy-rule")
	if err == nil || !strings.Contains(err.Error(), "filesystem root") {
		t.Fatalf("expected filesystem-root quarantine base to be rejected, got %v", err)
	}
}
