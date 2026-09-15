// Package bench は fc が生成するコードの性能ベンチマーク (README.md)。
//
//	go test ./bench            結果を results.json と比べる (サイクル数・コードサイズ・出力)
//	go test ./bench -update    現在の値で results.json を書き換える
//	go test ./bench -v         一覧表を表示する
//
// 各 *.fc は emu ターゲットで動かし、stdio.bench_start / bench_end で囲んだ区間のサイクル数
// (エミュレータが数える。決定的) と、そのモジュールのセグメントのサイズ (ld65 の map から) を取る。
// 出力 (チェックサム) が変わったらコンパイラのバグ、サイクル数・サイズが変わったら最適化の効果か退行。
package bench

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/driver"
)

var update = flag.Bool("update", false, "results.json を現在の値で書き換える")

const resultsFile = "results.json"

// Entry は 1 つのベンチマークの記録。
type Entry struct {
	Cycles int64  `json:"cycles"` // bench_start〜bench_end の合計サイクル数
	Size   int    `json:"size"`   // モジュールのセグメントのバイト数 (コード + ROM データ)
	Out    string `json:"out"`    // プログラムの出力 (チェックサム)
}

func TestBench(t *testing.T) {
	benchDir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Dir(benchDir)
	files, _ := filepath.Glob(filepath.Join(benchDir, "*.fc"))
	if len(files) == 0 {
		t.Fatal("bench/*.fc が無い")
	}

	want := map[string]Entry{}
	if data, err := os.ReadFile(filepath.Join(benchDir, resultsFile)); err == nil {
		if err := json.Unmarshal(data, &want); err != nil {
			t.Fatalf("%s: %v", resultsFile, err)
		}
	}

	got := map[string]Entry{}
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".fc")
		got[name] = run(t, repoRoot, benchDir, name)
	}

	names := make([]string, 0, len(got))
	for n := range got {
		names = append(names, n)
	}
	sort.Strings(names)

	var table strings.Builder
	fmt.Fprintf(&table, "%-12s %12s %12s %8s   %8s %8s %7s\n", "bench", "cycles", "prev", "diff", "size", "prev", "diff")
	changed := false
	for _, n := range names {
		g := got[n]
		w, ok := want[n]
		if !ok {
			fmt.Fprintf(&table, "%-12s %12d %12s %8s   %8d %8s %7s  (new)\n", n, g.Cycles, "-", "", g.Size, "-", "")
			changed = true
			continue
		}
		fmt.Fprintf(&table, "%-12s %12d %12d %+7.1f%%   %8d %8d %+6.1f%%\n", n, g.Cycles, w.Cycles, pct(g.Cycles, w.Cycles), g.Size, w.Size, pct(int64(g.Size), int64(w.Size)))
		if g.Out != w.Out {
			if *update {
				t.Logf("%s: 出力を更新: %q → %q", n, w.Out, g.Out)
			} else {
				t.Errorf("%s: 出力が変わった (コンパイラのバグの可能性): got %q want %q", n, g.Out, w.Out)
			}
		}
		if g.Cycles != w.Cycles || g.Size != w.Size {
			changed = true
		}
	}
	for n := range want {
		if _, ok := got[n]; !ok {
			t.Errorf("%s: results.json にあるが bench/%s.fc が無い", n, n)
		}
	}
	t.Log("\n" + table.String())

	if *update {
		data, _ := json.MarshalIndent(got, "", "  ")
		if err := os.WriteFile(filepath.Join(benchDir, resultsFile), append(data, '\n'), 0o666); err != nil {
			t.Fatal(err)
		}
		return
	}
	if changed {
		t.Errorf("サイクル数またはサイズが results.json と違う。意図した変化なら `go test ./bench -update` で更新する\n%s", table.String())
	}
}

func pct(got, want int64) float64 {
	if want == 0 {
		return 0
	}
	return float64(got-want) * 100 / float64(want)
}

// run は 1 つのベンチマークを emu ターゲットでビルドして実行する。
func run(t *testing.T, repoRoot, benchDir, name string) Entry {
	t.Helper()
	tmp := t.TempDir()
	var out strings.Builder
	res, err := driver.NewCompiler(repoRoot).BuildContext(context.Background(), name+".fc", &driver.BuildOptions{
		Dir:      benchDir,
		BuildDir: filepath.Join(tmp, "b"),
		Out:      filepath.Join(tmp, name+".bin"),
		Run:      true,
		Stdout:   &out,
	})
	if err != nil {
		t.Fatalf("%s: ビルド失敗: %v", name, err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("%s: 終了コード %d", name, res.ExitCode)
	}
	return Entry{Cycles: res.Cycles, Size: segmentSize(t, res.MapFile, name), Out: strings.TrimSpace(out.String())}
}

// segmentSize は ld65 の map ファイルの "Segment list" から名前 seg のサイズを読む。
func segmentSize(t *testing.T, mapFile, seg string) int {
	t.Helper()
	f, err := os.Open(mapFile)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	in := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "Segment list:"):
			in = true
		case in && strings.HasPrefix(line, "Exports list"):
			in = false
		case in:
			fields := strings.Fields(line)
			if len(fields) >= 4 && fields[0] == seg {
				n, err := strconv.ParseInt(fields[3], 16, 32)
				if err != nil {
					t.Fatal(err)
				}
				return int(n)
			}
		}
	}
	t.Fatalf("%s: セグメント %s が map に無い", mapFile, seg)
	return 0
}
