package driver

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ResolveFCHome must reach the embedded-library fallback: neither the temporary
// working directory nor the go test executable's directory contains FC sources.
func isolatedHomeLookup(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("FC_HOME", "")
	return dir
}

// blockHomeCache はキャッシュへの展開を失敗させる (一時ディレクトリへの展開を試すテスト用)。
func blockHomeCache(t *testing.T, dir string) {
	t.Helper()
	blocked := filepath.Join(dir, "cache-is-a-file")
	if err := os.WriteFile(blocked, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FC_CACHE_DIR", blocked)
}

func setHomeTempDir(t *testing.T, dir string) {
	t.Helper()
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, dir)
	}
}

func TestResolveFCHomeTempFailure(t *testing.T) {
	dir := isolatedHomeLookup(t)
	blocked := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blocked, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FC_CACHE_DIR", blocked)
	setHomeTempDir(t, blocked)
	home, cleanup, err := ResolveFCHome()
	if cleanup != nil {
		defer cleanup()
	}
	if err == nil || home != "" || cleanup != nil {
		t.Fatalf("home=%q cleanup=%v error=%v", home, cleanup != nil, err)
	}
	var cause *os.PathError
	if !errors.As(err, &cause) {
		t.Fatalf("original OS error was lost: %v", err)
	}
	tempEnv := "TMPDIR"
	if runtime.GOOS == "windows" {
		tempEnv = "TMP and TEMP"
	}
	for _, want := range []string{"failed to create a temporary directory for bundled FC libraries", "parent: " + blocked, cause.Error(), "Set " + tempEnv + " to a writable directory", "FC_HOME", "fclib/ and share/", "pass these variables to the fcc process"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
	data, readErr := os.ReadFile(blocked)
	if readErr != nil || string(data) != "keep" {
		t.Fatalf("temp path was changed: data=%q err=%v", data, readErr)
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil || len(entries) != 1 {
		t.Fatalf("unexpected fallback files: entries=%v err=%v", entries, readErr)
	}
}

func TestResolveFCHomeSuccessAndCleanup(t *testing.T) {
	dir := isolatedHomeLookup(t)
	tmp := filepath.Join(dir, "tmp")
	if err := os.Mkdir(tmp, 0700); err != nil {
		t.Fatal(err)
	}
	blockHomeCache(t, dir)
	setHomeTempDir(t, tmp)
	dir = tmp
	home, cleanup, err := ResolveFCHome()
	if err != nil {
		t.Fatal(err)
	}
	if cleanup == nil {
		t.Fatal("embedded home has no cleanup")
	}
	defer cleanup()
	if filepath.Dir(home) != dir {
		t.Fatalf("home=%q want child of %q", home, dir)
	}
	for _, name := range []string{"fclib", "share"} {
		info, err := os.Stat(filepath.Join(home, name))
		if err != nil || !info.IsDir() {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	cleanup()
	if _, err := os.Stat(home); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("home remains after cleanup: %v", err)
	}
}

func TestResolveFCHomeExistingHomeSkipsTemp(t *testing.T) {
	for _, mode := range []string{"environment", "working-directory"} {
		t.Run(mode, func(t *testing.T) {
			dir := isolatedHomeLookup(t)
			home := filepath.Join(dir, "existing")
			for _, name := range []string{"fclib", "share", "nested"} {
				if err := os.MkdirAll(filepath.Join(home, name), 0700); err != nil {
					t.Fatal(err)
				}
			}
			setHomeTempDir(t, filepath.Join(dir, "missing", "temp"))
			if mode == "environment" {
				t.Setenv("FC_HOME", home)
			} else {
				t.Chdir(filepath.Join(home, "nested"))
			}
			got, cleanup, err := ResolveFCHome()
			if cleanup != nil {
				defer cleanup()
			}
			if err != nil || got != home || cleanup != nil {
				t.Fatalf("home=%q cleanup=%v error=%v", got, cleanup != nil, err)
			}
		})
	}
}

// TestResolveFCHomeCache: 同梱の fclib/ share/ はキャッシュの home-<中身のハッシュ> に展開して使い回す (消さない。
// -g の .s に入る fclib のパスが実行ごとに変わらないように)。
func TestResolveFCHomeCache(t *testing.T) {
	dir := isolatedHomeLookup(t)
	cache := filepath.Join(dir, "cache")
	t.Setenv("FC_CACHE_DIR", cache)
	setHomeTempDir(t, filepath.Join(dir, "missing", "temp")) // 一時ディレクトリは使わない
	var homes []string
	for i := 0; i < 2; i++ {
		home, cleanup, err := ResolveFCHome()
		if err != nil || cleanup != nil {
			t.Fatalf("home=%q cleanup=%v error=%v", home, cleanup != nil, err)
		}
		homes = append(homes, home)
	}
	if homes[0] != homes[1] || filepath.Dir(homes[0]) != cache || !strings.HasPrefix(filepath.Base(homes[0]), "home-") {
		t.Fatalf("homes=%q (同じ %s/home-* のはず)", homes, cache)
	}
	for _, name := range []string{"fclib", "share", homeComplete} {
		if _, err := os.Stat(filepath.Join(homes[0], name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
	entries, _ := os.ReadDir(cache)
	if len(entries) != 1 {
		t.Errorf("キャッシュに展開の途中のものが残っている: %v", entries)
	}
}
