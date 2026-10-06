package driver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/haramako/fc/internal/project"
	"github.com/haramako/fc/internal/regalloc"
)

// TestConcurrentBuilds: 1 つの Compiler で並行にビルド・検査してよい (ビルドごとの状態は compilation)。ディレクトリごとに
// fc.toml の @(build) の上書きと -O を変え、どのビルドも自分の設定の結果になるかを見る (go test -race で状態の共有を見張る)。
func TestConcurrentBuilds(t *testing.T) {
	t.Parallel()
	c := NewCompiler(absRepoRoot)
	const n = 8
	dirs := make([]string, n)
	for i := range dirs {
		dirs[i] = t.TempDir()
		files := map[string]string{
			"fc.toml": fmt.Sprintf("[define.t]\nK = %d\n", i*10),
			"t.fc":    "#fc 4\nuse console;\nconst K:u8 = 0 @(build);\nfunction main():void\n{\n\t@printf(\"{}\\n\", K);\n\tconsole.exit(K / 10);\n}\n",
		}
		for name, s := range files {
			if err := os.WriteFile(filepath.Join(dirs[i], name), []byte(s), 0o666); err != nil {
				t.Fatal(err)
			}
		}
	}
	var wg sync.WaitGroup
	errs := make([]string, n)
	for i := range dirs {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			var out strings.Builder
			level := -1
			if i%2 == 0 {
				level = 2
			}
			code, err := buildCode(t, c, "t.fc", &BuildOptions{Dir: dirs[i], Target: "emu", OptimizeLevel: level,
				Out: filepath.Join(dirs[i], "a.bin")}, true, &out, 0)
			if want := fmt.Sprintf("%d\n", i*10); err != nil || code != i || out.String() != want {
				errs[i] = fmt.Sprintf("build %d: exit %d, out %q, err %v (want exit %d, out %q)", i, code, out.String(), err, i, want)
			}
		}(i)
		go func(i int) {
			defer wg.Done()
			if _, err := c.Check("t.fc", &CheckOptions{Dir: dirs[i], Target: "emu"}); err != nil {
				t.Errorf("check %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	for _, e := range errs {
		if e != "" {
			t.Error(e)
		}
	}
}

// TestMemoryMapStack: base.s の FC_STACK の大きさ (MemoryMap の ZP_STACK) は、regalloc / codegen がフレームの上限に使う
// regalloc.StackSize と同じ。
func TestMemoryMapStack(t *testing.T) {
	for _, target := range []string{"nes", "emu"} {
		if got := project.DefaultMemoryMap(target).Stack.Size; got != regalloc.StackSize {
			t.Errorf("%s: ZP_STACK %d, regalloc.StackSize %d", target, got, regalloc.StackSize)
		}
	}
}
