package driver

// 常駐レジスタの自己修正 (codegen.Llc.CompileLambda) のテスト。regalloc の見積もりで「触らない」(ResFree) とした命令の
// 本体が実際には常駐レジスタを書いていたら、その命令だけ退避 / 復帰に直してコンパイルし直す。普段のビルドでは見積もりが
// 外れないのでこの経路は通らず (カバレッジ 0。2026-09-28)、壊れていても気づけない。ここでは見積もりをわざと外し
// (BuildOptions.MisclassifyResident)、自己修正が働いて (ResidentFixes > 0)、実行結果が普段のビルドと同じになることを確かめる。

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResidentSelfCorrection(t *testing.T) {
	t.Parallel()
	total := 0
	for seed := int64(1); seed <= 6; seed++ {
		g := &rpGen{r: rand.New(rand.NewSource(seed)), v3: &rpV3{}}
		g.genProgram()
		files := g.sources()
		want, _, err := rfBuild(t, files, false)
		if err != nil {
			continue // 生成したプログラムが大きすぎるなど (この検査の対象外)
		}
		got, fixes, err := rfBuild(t, files, true)
		if err != nil {
			t.Fatalf("seed %d: 見積もりを外したビルドが失敗: %v\n%s", seed, err, g.allSource())
		}
		if got != want {
			t.Fatalf("seed %d: 自己修正した結果が違う (直した命令 %d)\n普段: %q\n修正: %q\n%s", seed, fixes, want, got, g.allSource())
		}
		total += fixes
	}
	if total == 0 {
		t.Fatalf("見積もりを外しても自己修正が一度も働かなかった (テストが何も確かめていない)")
	}
	t.Logf("直した命令: %d", total)
}

// rfBuild は files をビルドして emu で走らせ、出力と、自己修正で直した命令の数を返す。
func rfBuild(t *testing.T, files map[string]string, misclassify bool) (string, int, error) {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	var o strings.Builder
	res, err := NewCompiler(absRepoRoot).BuildContext(context.Background(), "t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"),
		Out: filepath.Join(dir, "a.bin"), Run: true, Stdout: &o, MaxCycles: 20_000_000, MisclassifyResident: misclassify})
	if err != nil {
		return "", 0, err
	}
	if res.ExitCode != 0 {
		return "", 0, fmt.Errorf("exit code %d: %s", res.ExitCode, o.String())
	}
	return o.String(), res.ResidentFixes, nil
}
