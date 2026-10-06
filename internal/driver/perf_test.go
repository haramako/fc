package driver

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPerfNotSlowerThanO0: testdata/perf/ の各プログラム (fuzz で見つけた、-O 2 が -O 0 より遅かった形) は、-O 2 のサイクル数が
// -O 0 を超えない。unroll-slower: ループを展開した関数が静的フレームの参照の数を増やし、ループの中で呼ばれる関数をゼロページから
// 追い出していた (frames.frameRefs が展開の写しを数えないようにした。2015 万 → 1625 万サイクル、-O 0 は 1890 万)。
func TestPerfNotSlowerThanO0(t *testing.T) {
	t.Parallel()
	dirs, err := filepath.Glob(filepath.Join("testdata", "perf", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("testdata/perf が無い: %v", err)
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			files := map[string]string{}
			ms, _ := filepath.Glob(filepath.Join(dir, "*.fc"))
			for _, m := range ms {
				b, err := os.ReadFile(m)
				if err != nil {
					t.Fatal(err)
				}
				files[filepath.Base(m)] = string(b)
			}
			var cycles [2]int64
			var outs [2]string
			for i, level := range []int{-1, 0} {
				r := testBuild(t, buildSpec{Files: files, Run: true, Level: level, MaxCycles: 200_000_000})
				if r.Err != nil {
					t.Fatalf("level %d: %v", level, r.Err)
				}
				cycles[i], outs[i] = r.Run.Cycles, r.Stdout
			}
			if outs[0] != outs[1] {
				t.Fatalf("-O 0 と -O 2 で出力が違う:\n%q\n%q", outs[0], outs[1])
			}
			if cycles[1] > cycles[0] {
				t.Errorf("-O 2 (%d サイクル) が -O 0 (%d サイクル) より遅い", cycles[1], cycles[0])
			}
		})
	}
}
