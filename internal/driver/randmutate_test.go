package driver

// 正しいプログラムを少し壊した入力でコンパイラが panic しないことを確かめる (TestRandomMutate)。
//
// go の fuzz (FuzzCheck) はバイト単位で壊すので、構文の段階のエラーで止まる入力が大半で、意味解析のエラーの経路
// (宣言がエラーの soa を引数の型に使う、など。2026-09-27 に FuzzCheck で見つかった 709ae7d) にはなかなか届かない。
// ここでは生成した正しいプログラム (TestRandomV3Programs と同じ) を、行・名前・型・数・キャストの単位で壊して
// Check (構文〜コード生成。ファイルは書かない) に通す。エラーになるのはよい。panic だけを失敗にする。
//
//	go test ./internal/driver -run TestRandomMutate -mutn 500 -randseed 5000

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var mutN = flag.Int("mutn", 10, "TestRandomMutate のプログラム数 (1 本を 20 通りに壊す)")

var (
	rmIdent  = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\b`)
	rmNumber = regexp.MustCompile(`\b[0-9]+\b`)
	rmTypeV2 = []string{"int", "sint", "int16", "sint16", "bool", "S", "E", "*S", "*int", "[4]int", "fn(int):int", "void"}
	rmTypeV3 = []string{"u8", "i8", "u16", "i16", "bool", "VS", "Color", "*VS", "[]u8", "[:u16]u8", "*const u8", "[4]u16", "fn(u8):u8", "void", "vsa"}
)

// rmMutate は src の 1 か所を壊す。
func rmMutate(r *rand.Rand, src string, v3 bool) string {
	lines := strings.Split(src, "\n")
	li := 1 + r.Intn(len(lines)-1) // #fc の行は残す
	line := lines[li]
	types := rmTypeV2
	if v3 {
		types = rmTypeV3
	}
	switch r.Intn(9) {
	case 0: // 行を消す
		lines = append(lines[:li], lines[li+1:]...)
	case 1: // 行を複製する
		lines = append(lines[:li+1], lines[li:]...)
	case 2: // 行を入れ替える
		lj := 1 + r.Intn(len(lines)-1)
		lines[li], lines[lj] = lines[lj], lines[li]
	case 3: // 名前を別の名前にする
		ids := rmIdent.FindAllString(src, -1)
		if locs := rmIdent.FindAllStringIndex(line, -1); len(locs) > 0 && len(ids) > 0 {
			l := locs[r.Intn(len(locs))]
			lines[li] = line[:l[0]] + ids[r.Intn(len(ids))] + line[l[1]:]
		}
	case 4: // 型の名前を別の型にする (`:T` の T)
		if i := strings.Index(line, ":"); i >= 0 {
			rest := line[i+1:]
			if l := rmIdent.FindStringIndex(rest); l != nil && l[0] <= 1 {
				lines[li] = line[:i+1] + types[r.Intn(len(types))] + rest[l[1]:]
			}
		}
	case 5: // 数を極端な値にする
		if locs := rmNumber.FindAllStringIndex(line, -1); len(locs) > 0 {
			l := locs[r.Intn(len(locs))]
			lines[li] = line[:l[0]] + []string{"0", "255", "256", "65535", "65536", "99999999", "-1"}[r.Intn(7)] + line[l[1]:]
		}
	case 6: // 名前の後ろにキャストを足す
		if locs := rmIdent.FindAllStringIndex(line, -1); len(locs) > 0 {
			l := locs[r.Intn(len(locs))]
			lines[li] = line[:l[1]] + " as " + types[r.Intn(len(types))] + line[l[1]:]
		}
	case 7: // 1 文字消す
		if len(line) > 0 {
			k := r.Intn(len(line))
			lines[li] = line[:k] + line[k+1:]
		}
	default: // 記号を 1 つ差し込む
		k := r.Intn(len(line) + 1)
		lines[li] = line[:k] + []string{"&", "*", "[", "]", "(", ")", "{", "}", ";", ",", ".", "..", "..=", "@", "-"}[r.Intn(15)] + line[k:]
	}
	return strings.Join(lines, "\n")
}

// TestRandomMutate は壊したプログラムで Check が panic しないことを確かめる。
func TestRandomMutate(t *testing.T) {
	t.Parallel()
	base := *randSeed
	if base == 0 {
		base = 1
	}
	for k := 0; k < *mutN; k++ {
		seed := base + int64(k)
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			t.Parallel()
			g := &rpGen{r: rand.New(rand.NewSource(seed)), v3: &rpV3{}}
			g.genProgram()
			files := g.sources()
			r := rand.New(rand.NewSource(seed * 31337))
			for m := 0; m < 20; m++ {
				name := "t.fc"
				if _, ok := files["v3m.fc"]; ok && r.Intn(2) == 0 {
					name = "v3m.fc"
				}
				mut := map[string]string{}
				for n, s := range files {
					mut[n] = s
				}
				for c := 0; c < 1+r.Intn(3); c++ {
					mut[name] = rmMutate(r, mut[name], name == "v3m.fc")
				}
				if msg := rmCheckNoPanic(t, mut); msg != "" {
					t.Fatalf("壊したプログラムで panic (seed %d、%d 通り目、%s):\n%s\n%s", seed, m, name, mut[name], msg)
				}
			}
		})
	}
}

// rmCheckNoPanic は files を Check に通し、panic したらその内容を返す (エラーは "")。
func rmCheckNoPanic(t *testing.T, files map[string]string) (msg string) {
	t.Helper()
	dir := t.TempDir()
	for n, s := range files {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(s), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		if r := recover(); r != nil {
			msg = fmt.Sprint(r)
		}
	}()
	c := NewCompiler(absRepoRoot)
	_, err := c.Check("t.fc", &CheckOptions{Dir: dir})
	if err != nil && strings.Contains(err.Error(), "panic") {
		return err.Error()
	}
	return ""
}
