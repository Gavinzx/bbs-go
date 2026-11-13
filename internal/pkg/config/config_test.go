package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetDbDefaultsSetsDefaultLogLevel(t *testing.T) {
	var cfg DBConfig

	SetDbDefaults(&cfg)

	if cfg.LogLevel != DefaultDBLogLevel {
		t.Fatalf("expected default db log level %q, got %q", DefaultDBLogLevel, cfg.LogLevel)
	}
}

func TestGetConfigFilePathResolvesSymlink(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}
	target := filepath.Join(dataDir, "bbs-go.yaml")
	if err := os.WriteFile(target, []byte("installed: false\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(root, "bbs-go.yaml")); err != nil {
		t.Fatalf("symlink config: %v", err)
	}
	t.Chdir(root)

	got := getConfigFilePath("bbs-go.yaml")
	want, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("eval target: %v", err)
	}
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
