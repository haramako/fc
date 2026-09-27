package driver

// 変換しても結果が変わらないはずの組み合わせの差分テスト (TestRandomMetamorphic)。生成したプログラムを、fcc の実行ファイルで
//
//	普通に / インライン展開を全部切って (FC_DISABLE=inline,autoinline) / 最適化の段を 1〜3 個切って
//
// 動かして出力を比べる。-O 0 と -O 2 の比較 (TestRandomPrograms) は最適化を「全部」か「無し」でしか比べないので、段どうしの
// 組み合わせ (ある段が前の段の出力の形に頼っている) の食い違いが見えにくい。FC_DISABLE はプロセスで 1 回だけ読まれるので、
// 段を切った版は別のプロセス (fcc の実行ファイル) で動かす。食い違ったら、切った段の名前を報告する (どの段の問題かの手がかり)。
//
//	go test ./internal/driver -run TestRandomMetamorphic -metan 200 -randseed 5000

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var metaN = flag.Int("metan", 4, "TestRandomMetamorphic のプログラム数")

// rmPasses は切って比べる段 (ir.Disabled の名前。どれを切っても意味は変わらないはず)。
var rmPasses = []string{"ssa", "mul", "sink", "fuse", "fuse-index", "indexoff", "fieldindex", "coalesce", "chain", "induction", "unroll",
	"narrow", "scale", "commute", "carry", "split", "ywalk", "resident", "func-resident", "peephole", "shift8", "switch", "devirt", "step", "rotate", "dup"}

var (
	fccOnce sync.Once
	fccPath string
	fccErr  error
)

// fccBinary は fcc の実行ファイル (テストの間に 1 回だけビルドする)。
func fccBinary(t *testing.T) string {
	t.Helper()
	fccOnce.Do(func() {
		dir, err := os.MkdirTemp("", "fcc-meta-")
		if err != nil {
			fccErr = err
			return
		}
		fccPath = filepath.Join(dir, "fcc")
		if runtime.GOOS == "windows" {
			fccPath += ".exe"
		}
		cmd := exec.Command("go", "build", "-o", fccPath, "./cmd/fcc")
		cmd.Dir = absRepoRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			fccErr = fmt.Errorf("go build: %v\n%s", err, out)
		}
	})
	if fccErr != nil {
		t.Fatal(fccErr)
	}
	return fccPath
}

// fccRun は files を fcc run (-O 2) で動かした出力。disable は FC_DISABLE。
func fccRun(t *testing.T, files map[string]string, disable string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o666); err != nil {
			return "", err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, fccBinary(t), "run", "t.fc")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "FC_DISABLE="+disable, "FC_VERIFY_REGS=1")
	var out strings.Builder
	cmd.Stdout = &out
	var errb strings.Builder
	cmd.Stderr = &errb
	err := cmd.Run()
	if ctx.Err() != nil {
		return out.String(), fmt.Errorf("timeout")
	}
	if err != nil {
		return out.String(), fmt.Errorf("%v: %s", err, errb.String())
	}
	return out.String(), nil
}

// TestRandomMetamorphic は段を切っても出力が変わらないことを確かめる。
func TestRandomMetamorphic(t *testing.T) {
	t.Parallel()
	base := *randSeed
	if base == 0 {
		base = 1
	}
	for k := 0; k < *metaN; k++ {
		seed := base + int64(k)
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			t.Parallel()
			g := &rpGen{r: rand.New(rand.NewSource(seed)), v3: &rpV3{}}
			g.genProgram()
			files := g.sources()
			want, err := fccRun(t, files, "")
			if err != nil {
				t.Skipf("普通のビルドが通らない / 止まらない (seed %d。TestRandomV3Programs の担当): %v", seed, firstLine(err.Error()))
			}
			r := rand.New(rand.NewSource(seed * 7919))
			variants := []string{"inline,autoinline"}
			for i := 0; i < 2; i++ {
				n := 1 + r.Intn(3)
				var ps []string
				for _, j := range r.Perm(len(rmPasses))[:n] {
					ps = append(ps, rmPasses[j])
				}
				variants = append(variants, strings.Join(ps, ","))
			}
			for _, v := range variants {
				got, err := fccRun(t, files, v)
				if err != nil && (strings.Contains(err.Error()+got, "frame size over") || strings.Contains(err.Error()+got, "memory area overflow")) {
					continue // 段を切ると大きくなって入らない (プログラムの問題)
				}
				if err != nil || got != want {
					t.Errorf("FC_DISABLE=%s で出力が変わる (seed %d):\n%s\n普通: %q\n切った: %q %v\n使った機能: %s", v, seed, g.allSource(), want, got, err, strings.Join(g.usedFeatures(), ","))
				}
			}
		})
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
