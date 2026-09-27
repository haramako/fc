package driver

// fuzz の生成器の機能の名前と入れ切り (効果の測定のため。doc/development_notes.md「fuzz の効果の測定」)。
//
//	go test ./internal/driver -run TestRandom -fuzzoff soa,far     // soa と far call を作らない
//	go test ./internal/driver -run TestRandom -fuzzonly uninit      // 名前の付いた機能は uninit だけ (ほかは切る)
//
// 生成器は機能を選ぶ所で g.want(名前, 確率) を呼ぶ。乱数は機能を切っても同じだけ使う (切らなければ種の番号は今までと
// 同じプログラム)。使った機能は g.used に残り、失敗の報告と FUZZ_STATS (1 行 1 本の JSON) に出る。
// -O 0 / -O 2 / インタプリタの判定、最小化、定数の畳み込みの差分テスト、自己検査の仕組みは必須なので切れない。

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

var (
	fuzzOff  = flag.String("fuzzoff", "", "生成器の機能を切る (カンマ区切り。rpFeatures の名前)")
	fuzzOnly = flag.String("fuzzonly", "", "名前の付いた生成器の機能をこれだけにする (カンマ区切り)")
)

// rpFeatures は生成器の機能の名前 (fuzzoff / fuzzonly に書ける名前)。
var rpFeatures = map[string]string{
	// TestRandomPrograms (fc 2 の本体)
	"struct":     "struct S と s0 / sa[4]",
	"soa":        "soa E:[8]S",
	"nested":     "struct T (S の入れ子と配列フィールド)、u0 / ua:[2]T、T へのポインタ",
	"alias":      "同じ場所を配列 ab と struct w で読み書きする",
	"sval":       "struct を値で受けて値で返す関数 sv",
	"const":      "const の表 ct0.. (ROM)",
	"far":        "別バンクのモジュール far1 の関数 (far call)",
	"farfn":      "far1 の関数の farfn 表 fq0",
	"rec":        "自分を呼ぶ関数 r0 (stack ABI)",
	"fptable":    "関数ポインタ表 fp0",
	"fnvar":      "main の関数ポインタのローカル変数 fv",
	"lambda":     "main のラムダ lf",
	"larray":     "関数のローカル配列 la0..",
	"inline":     "@(inline) の関数",
	"fastcall":   "fastcall の関数",
	"switch":     "switch (密な整数の case はジャンプ表)",
	"while":      "条件つきの while",
	"unroll":     "減らしながらの for (展開の対象)",
	"walk":       "ポインタをずらすループ",
	"structcopy": "struct の値のコピー",
	"uninit":     "初期値なしの変数とループ・if / else での代入 (2026-09-27)",
	"widen":      "狭い型の値での初期化・代入 (暗黙の拡張。2026-09-27)",
	"regpress":   "常駐レジスタに負荷をかけるループ (過去の常駐のバグの形。2026-09-28)",
	// TestRandomV3Programs (v3m)
	"v3big":       "v3m: 2 バイトの要素の 200 要素の配列 (2026-09-27)",
	"v3negoff":    "v3m: ポインタの負の添字・i8 のずれ (2026-09-27)",
	"v3foreach":   "v3m: for-each (2026-09-27)",
	"v3range":     "v3m: 範囲のループ (2026-09-27)",
	"v3caserange": "v3m: case の範囲 (2026-09-27)",
	"v3slice":     "v3m: slice (範囲・添字・@copy・広い slice。`..=` を含む)",
	"v3enum":      "v3m: enum と switch",
	"v3aos":       "v3m: struct の配列のフィールド (2026-09-27)",
	"v3fnconst":   "v3m: const の表の無名関数 (2026-09-27)",
	"v3fold":      "v3m: 型付きの定数と @min / @max (2026-09-27)",
	"v3self":      "v3m: 自己検査 (2026-09-27)",
}

var (
	rpOffOnce sync.Once
	rpOff     map[string]bool
)

// rpFeatOff は切った機能の集合 (-fuzzoff / -fuzzonly。知らない名前は panic)。
func rpFeatOff() map[string]bool {
	rpOffOnce.Do(func() {
		rpOff = map[string]bool{}
		check := func(list string) []string {
			var names []string
			for _, n := range strings.Split(list, ",") {
				n = strings.TrimSpace(n)
				if n == "" {
					continue
				}
				if _, ok := rpFeatures[n]; !ok {
					panic(fmt.Sprintf("fuzz: 知らない機能 %q (randfeat_test.go の rpFeatures)", n))
				}
				names = append(names, n)
			}
			return names
		}
		for _, n := range check(*fuzzOff) {
			rpOff[n] = true
		}
		if only := check(*fuzzOnly); len(only) > 0 {
			keep := map[string]bool{}
			for _, n := range only {
				keep[n] = true
			}
			for n := range rpFeatures {
				if !keep[n] {
					rpOff[n] = true
				}
			}
		}
	})
	return rpOff
}

// feat は機能 name が有効か (有効なら使った印を付ける)。
func (g *rpGen) feat(name string) bool {
	if _, ok := rpFeatures[name]; !ok {
		panic("fuzz: 登録していない機能 " + name)
	}
	if rpFeatOff()[name] {
		return false
	}
	if g.used == nil {
		g.used = map[string]bool{}
	}
	g.used[name] = true
	return true
}

// want は確率 p で機能 name を使うか (乱数は機能を切っても同じだけ使う)。
func (g *rpGen) want(name string, p float64) bool {
	c := g.chance(p)
	if !c {
		return false
	}
	return g.feat(name)
}

// usedFeatures は使った機能の名前 (並べたもの)。
func (g *rpGen) usedFeatures() []string {
	var r []string
	for n := range g.used {
		r = append(r, n)
	}
	sort.Strings(r)
	return r
}

// rpStat は FUZZ_STATS に書く 1 本分の記録。
type rpStat struct {
	Test     string   `json:"test"`
	Seed     int64    `json:"seed"`
	Kind     string   `json:"kind"`
	Features []string `json:"features"`
}

var rpStatMu sync.Mutex

// rpRecord は FUZZ_STATS (ファイル名) があれば 1 本分の結果を追記する (夜間の fuzz の集計用)。
func rpRecord(test string, seed int64, kind string, g *rpGen) {
	path := os.Getenv("FUZZ_STATS")
	if path == "" {
		return
	}
	b, _ := json.Marshal(rpStat{Test: test, Seed: seed, Kind: kind, Features: g.usedFeatures()})
	rpStatMu.Lock()
	defer rpStatMu.Unlock()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(b, '\n'))
}
