package driver

// test/v2/ (test/*.fc を fcc migrate で fc 2 に移行した複製) が test/ と同じ生成物を出すことを確認する
// (doc/v2_grammar.md §7 Q5)。ir golden は可視性 (pub) の違いで一致しないので、asm (正規化)・bin・stdout を比べる。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/codegen"
	"github.com/haramako/fc/internal/sema"
)

func testV2Dir() string { return filepath.Join(absRepoRoot, "test", "v2") }

func TestGoldenV2(t *testing.T) {
	dirs, err := os.ReadDir(filepath.Join(absGoldenRoot, "asm"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("golden asm が見つからない: %v", err)
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		name := d.Name()
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srcName, target := goldenKeyInfo(name)
			src, err := os.ReadFile(filepath.Join(testV2Dir(), srcName+".fc"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(string(src), "#fc 2") {
				t.Fatalf("test/v2/%s.fc は fc 2 ではない", srcName)
			}

			// asm (正規化)
			fclib := filepath.ToSlash(filepath.Join(absRepoRoot, "fclib"))
			prog, err := sema.Compile(testV2Dir(), []string{".", fclib, fclib + "/" + target}, srcName+".fc")
			if err != nil {
				t.Fatalf("コンパイル失敗: %v", err)
			}
			llc := codegen.NewLlc(2, prog.Types)
			for _, mod := range prog.Modules.List() {
				asm, inc, err := llc.Compile(mod)
				if err != nil {
					t.Fatalf("コード生成失敗: %v", err)
				}
				compareGoldenAsm(t, "asm/"+name+"/"+mod.Id+".s", normalizeAsm(asm))
				compareGoldenAsm(t, "asm/"+name+"/"+mod.Id+".inc", normalizeAsm(inc))
			}

			// bin / stdout
			tmp := t.TempDir()
			out := filepath.Join(tmp, "a.bin")
			if target == "nes" {
				out = filepath.Join(tmp, "a.nes")
			}
			var stdout strings.Builder
			run := target == "emu"
			code, err := NewCompiler(absRepoRoot).Build(srcName+".fc", &BuildOptions{
				Target: target, Out: out, Run: run, Stdout: &stdout,
				Dir: testV2Dir(), BuildDir: filepath.Join(tmp, "build"),
			})
			if err != nil {
				t.Fatalf("ビルド失敗: %v", err)
			}
			got, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			compareGoldenBytes(t, "bin/"+name+filepath.Ext(out), got)
			if run {
				compareGolden(t, "stdout/"+name+".txt", stdout.String())
				compareGolden(t, "stdout/"+name+".exit", fmt.Sprintf("%d\n", code))
			}
		})
	}
}
