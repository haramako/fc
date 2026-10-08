package driver

// 2 次元の配列とループのランダムテスト (TestRandomGridV4)。opt の fieldindex (a[y][x] を 1 バイトの添字 y * s + x に)・licm (ループ
// 不変の計算を前へ)・strength (ループで進む変数の掛け算・加算を一緒に進む変数に) を狙う (Agent/wiki/design/ssa.md §12)。
// fc 4 のソースを直接作り、期待値は生成器が Go で同じプログラムを実行して計算する。
//
// 生成するもの:
//   - グローバルの 2 次元の配列 3 つ: u8 で 256 バイト以内 (fieldindex が畳む)・u8 で 256 バイトを超えることがあるもの・u16 の要素
//   - 関数ごとに 1〜3 重のループ: 増えるループ (歩幅 1〜3)・減るループ (添字は v - 1)・while (末尾で進める)。本体で配列の読み書き、
//     添字は ループの変数 ± 1・定数・ループの前の変数、手で書いた y * C + x (1 次元に見た添字)、条件付きの読み書き、continue・break、
//     ループの変数を本体の途中で 1 進める (範囲を超えない条件で: 誘導変数の定義が 2 つになる)
//   - 最後に配列と acc の和を出す
//
// 添字は生成器が必ず範囲内にする (範囲外の添字は未定義)。値の計算は u8 / u16 の環の演算で、幅で切って Go と合わせる。

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// rgArr はグローバルの 2 次元の配列。
type rgArr struct {
	name string
	r, c int
	wide bool // u16 の要素
}

// rgVar は添字に使える変数 (ループの変数か、ループの前の変数) と、本体で取りうる値の範囲 [lo, hi]。
type rgVar struct {
	name   string
	lo, hi int
	minus1 bool // 減るループの変数 (添字は name - 1 の形で、その値が [lo, hi])
}

// rgState は Go で実行するときの状態。
type rgState struct {
	arrs map[string][]int // 行優先
	vars map[string]int
	acc  int
	work int // 実行した文の数 (無限ループの保険)
}

// rgCtl はループの流れ (break / continue)。
type rgCtl int

const (
	rgNext rgCtl = iota
	rgBreak
	rgContinue
)

// rgStmt は生成した文 (fc のソースと、Go で実行する関数)。
type rgStmt struct {
	src string
	run func(st *rgState) rgCtl
}

// rgExpr は生成した式 (fc のソースと値)。
type rgExpr struct {
	src string
	val func(st *rgState) int
}

type rgGen struct {
	r     *rand.Rand
	arrs  []*rgArr
	nvar  int
	funcs []string
	runs  []func(st *rgState)
}

func (g *rgGen) pick(n int) int        { return g.r.Intn(n) }
func (g *rgGen) chance(p float64) bool { return g.r.Float64() < p }

func (g *rgGen) newName(prefix string) string {
	g.nvar++
	return fmt.Sprintf("%s%d", prefix, g.nvar)
}

// index は大きさ n の次元の添字の式 (vars の範囲から必ず 0〜n-1 になるもの)。
func (g *rgGen) index(n int, vars []*rgVar) rgExpr {
	var cands []rgExpr
	for _, v := range vars {
		base := v.name
		if v.minus1 {
			base = "(" + v.name + " - 1)"
		}
		for d := -1; d <= 1; d++ {
			if v.lo+d < 0 || v.hi+d >= n {
				continue
			}
			v, d := v, d
			src := base
			switch {
			case d > 0:
				src = fmt.Sprintf("%s + %d", base, d)
			case d < 0:
				src = fmt.Sprintf("%s - %d", base, -d)
			}
			cands = append(cands, rgExpr{src, func(st *rgState) int {
				x := st.vars[v.name]
				if v.minus1 {
					x--
				}
				return x + d
			}})
		}
	}
	if len(cands) == 0 || g.chance(0.1) {
		k := g.pick(n)
		return rgExpr{fmt.Sprint(k), func(*rgState) int { return k }}
	}
	return cands[g.pick(len(cands))]
}

// elem は配列 a の要素の読み (式) と、その位置。
func (g *rgGen) elem(a *rgArr, vars []*rgVar) (string, func(st *rgState) int) {
	y, x := g.index(a.r, vars), g.index(a.c, vars)
	src := fmt.Sprintf("%s[%s][%s]", a.name, y.src, x.src)
	return src, func(st *rgState) int { return y.val(st)*a.c + x.val(st) }
}

// value は u8 の値の式。
func (g *rgGen) value(vars []*rgVar) rgExpr {
	switch g.pick(5) {
	case 0:
		k := g.pick(256)
		return rgExpr{fmt.Sprint(k), func(*rgState) int { return k }}
	case 1:
		if len(vars) > 0 {
			v := vars[g.pick(len(vars))]
			k := 1 + g.pick(7)
			return rgExpr{fmt.Sprintf("(%s * %d) as u8", v.name, k), func(st *rgState) int { return st.vars[v.name] * k & 0xff }}
		}
	}
	a := g.arrs[g.pick(len(g.arrs))]
	src, at := g.elem(a, vars)
	if a.wide {
		return rgExpr{"(" + src + " as u8)", func(st *rgState) int { return st.arrs[a.name][at(st)] & 0xff }}
	}
	return rgExpr{src, func(st *rgState) int { return st.arrs[a.name][at(st)] }}
}

// stmt は本体の文 1 つ。bump は本体の途中で 1 進めてよい一番内側のループの変数 (nil なら無し。bumpHi 未満に保つ)。
func (g *rgGen) stmt(vars []*rgVar, depth int, inLoop bool, bump *rgVar, bumpHi int) rgStmt {
	switch k := g.pick(12); {
	case k < 3: // 書き込み
		a := g.arrs[g.pick(len(g.arrs))]
		src, at := g.elem(a, vars)
		v := g.value(vars)
		mask := 0xff
		if a.wide {
			mask = 0xffff
		}
		switch g.pick(3) {
		case 0:
			return rgStmt{fmt.Sprintf("%s = %s;\n", src, v.src), func(st *rgState) rgCtl {
				st.arrs[a.name][at(st)] = v.val(st)
				return rgNext
			}}
		case 1:
			return rgStmt{fmt.Sprintf("%s += %s;\n", src, v.src), func(st *rgState) rgCtl {
				i := at(st)
				st.arrs[a.name][i] = (st.arrs[a.name][i] + v.val(st)) & mask
				return rgNext
			}}
		default:
			return rgStmt{fmt.Sprintf("%s ^= %s;\n", src, v.src), func(st *rgState) rgCtl {
				i := at(st)
				st.arrs[a.name][i] ^= v.val(st)
				return rgNext
			}}
		}
	case k < 6: // 読んで acc に
		v := g.value(vars)
		return rgStmt{fmt.Sprintf("acc += %s as u16;\n", v.src), func(st *rgState) rgCtl {
			st.acc = (st.acc + v.val(st)) & 0xffff
			return rgNext
		}}
	case k < 7 && len(vars) >= 2: // 手で書いた 1 次元の添字 y * C + x (u8 の配列で 256 バイト以内)
		var a *rgArr
		for _, c := range g.arrs {
			if !c.wide && c.r*c.c <= 256 {
				a = c
			}
		}
		if a == nil {
			break
		}
		y, x := g.index(a.r, vars), g.index(a.c, vars)
		t := g.newName("t")
		m := 1 + g.pick(3)
		return rgStmt{fmt.Sprintf("var %s:u8 = (%s) * %d + (%s);\nacc += %s as u16;\nacc += (%s as u16) * %d;\n", t, y.src, a.c, x.src, t, t, m),
			func(st *rgState) rgCtl {
				v := (y.val(st)*a.c + x.val(st)) & 0xff
				st.acc = (st.acc + v + v*m) & 0xffff
				return rgNext
			}}
	case k < 9 && depth > 0: // 条件付き
		c := g.value(vars)
		kk := g.pick(256)
		inner := g.block(vars, depth-1, inLoop, bump, bumpHi)
		return rgStmt{fmt.Sprintf("if (%s > %d) {\n%s}\n", c.src, kk, inner.src), func(st *rgState) rgCtl {
			if c.val(st) > kk {
				return inner.run(st)
			}
			return rgNext
		}}
	case k < 10 && inLoop && depth > 0: // continue / break
		c := g.value(vars)
		kk := g.pick(256)
		if g.chance(0.5) {
			return rgStmt{fmt.Sprintf("if (%s < %d) {\ncontinue;\n}\n", c.src, kk), func(st *rgState) rgCtl {
				if c.val(st) < kk {
					return rgContinue
				}
				return rgNext
			}}
		}
		return rgStmt{fmt.Sprintf("if (%s == %d) {\nbreak;\n}\n", c.src, kk), func(st *rgState) rgCtl {
			if c.val(st) == kk {
				return rgBreak
			}
			return rgNext
		}}
	case k < 11 && bump != nil: // ループの変数を範囲の中で 1 進める
		c := g.value(vars)
		kk := g.pick(256)
		b := bump
		return rgStmt{fmt.Sprintf("if (%s + 1 < %d && %s > %d) {\n%s += 1;\n}\n", b.name, bumpHi, c.src, kk, b.name), func(st *rgState) rgCtl {
			if st.vars[b.name]+1 < bumpHi && c.val(st) > kk {
				st.vars[b.name]++
			}
			return rgNext
		}}
	case depth > 0 && len(vars) < 3: // 内側のループ
		return g.loop(vars, depth-1)
	}
	v := g.value(vars)
	return rgStmt{fmt.Sprintf("acc ^= %s as u16;\n", v.src), func(st *rgState) rgCtl {
		st.acc ^= v.val(st)
		return rgNext
	}}
}

// block は文の並び。
func (g *rgGen) block(vars []*rgVar, depth int, inLoop bool, bump *rgVar, bumpHi int) rgStmt {
	n := 1 + g.pick(4)
	var src strings.Builder
	var runs []func(st *rgState) rgCtl
	for i := 0; i < n; i++ {
		s := g.stmt(vars, depth, inLoop, bump, bumpHi)
		src.WriteString(s.src)
		runs = append(runs, s.run)
	}
	return rgStmt{src.String(), func(st *rgState) rgCtl {
		for _, r := range runs {
			st.work++
			if c := r(st); c != rgNext {
				return c
			}
		}
		return rgNext
	}}
}

// loop はループ 1 つ (変数の範囲は使う配列のどの次元にも 1 つ以上の添字が作れる大きさ)。
func (g *rgGen) loop(outer []*rgVar, depth int) rgStmt {
	name := g.newName("v")
	maxN := 32
	for _, a := range g.arrs {
		maxN = min(maxN, a.r, a.c)
	}
	lo := g.pick(2)
	hi := lo + 1 + g.pick(max(maxN-lo, 1))
	hi = min(hi, maxN)
	if hi <= lo {
		hi = lo + 1
	}
	var vars []*rgVar
	vars = append(vars, outer...)
	switch g.pick(5) {
	case 0: // 減るループ: v は hi..lo+1、添字は v - 1 (lo..hi-1)
		v := &rgVar{name: name, lo: lo, hi: hi - 1, minus1: true}
		body := g.block(append(vars, v), depth, true, nil, 0)
		src := fmt.Sprintf("for (var %s:u8 = %d; %s > %d; %s -= 1) {\n%s}\n", name, hi, name, lo, name, body.src)
		return rgStmt{src, func(st *rgState) rgCtl {
			for st.vars[name] = hi; st.vars[name] > lo; st.vars[name]-- {
				if body.run(st) == rgBreak {
					break
				}
			}
			delete(st.vars, name)
			return rgNext
		}}
	case 1: // while (末尾で進める。continue は使わない)
		v := &rgVar{name: name, lo: lo, hi: hi - 1}
		body := g.block(append(vars, v), depth, false, nil, 0)
		src := fmt.Sprintf("var %s:u8 = %d;\nwhile (%s < %d) {\n%s%s += 1;\n}\n", name, lo, name, hi, body.src, name)
		return rgStmt{src, func(st *rgState) rgCtl {
			for st.vars[name] = lo; st.vars[name] < hi; st.vars[name]++ {
				body.run(st)
			}
			return rgNext
		}}
	default:
		step := 1
		if g.chance(0.3) {
			step = 2 + g.pick(2)
		}
		v := &rgVar{name: name, lo: lo, hi: hi - 1}
		var bump *rgVar
		if step == 1 && g.chance(0.3) {
			bump = v
		}
		body := g.block(append(vars, v), depth, true, bump, hi)
		inc := fmt.Sprintf("%s += %d", name, step)
		src := fmt.Sprintf("for (var %s:u8 = %d; %s < %d; %s) {\n%s}\n", name, lo, name, hi, inc, body.src)
		return rgStmt{src, func(st *rgState) rgCtl {
			for st.vars[name] = lo; st.vars[name] < hi; st.vars[name] += step {
				if body.run(st) == rgBreak {
					break
				}
			}
			delete(st.vars, name)
			return rgNext
		}}
	}
}

// program はソースと期待する出力。
func (g *rgGen) program() (map[string]string, string) {
	r1, c1 := 2+g.pick(15), 2+g.pick(15)
	for r1*c1 > 256 {
		c1--
	}
	r2, c2 := 2+g.pick(20), 2+g.pick(20)
	r3, c3 := 2+g.pick(10), 2+g.pick(10)
	g.arrs = []*rgArr{{name: "A", r: r1, c: c1}, {name: "B", r: r2, c: c2}, {name: "W", r: r3, c: c3, wide: true}}
	var src strings.Builder
	src.WriteString("#fc 4\nuse console;\n")
	for _, a := range g.arrs {
		t := "u8"
		if a.wide {
			t = "u16"
		}
		fmt.Fprintf(&src, "var %s:[%d][%d]%s;\n", a.name, a.r, a.c, t)
	}
	src.WriteString("var acc:u16;\n")
	var fruns []func(st *rgState) rgCtl
	nf := 1 + g.pick(3)
	for f := 0; f < nf; f++ {
		// ループの前の変数 (添字に使う不変の値)
		var pre []*rgVar
		var preSrc strings.Builder
		var preRuns []func(st *rgState)
		if g.chance(0.5) {
			name := g.newName("p")
			k := g.pick(2)
			pre = append(pre, &rgVar{name: name, lo: k, hi: k})
			fmt.Fprintf(&preSrc, "var %s:u8 = %d;\n", name, k)
			preRuns = append(preRuns, func(st *rgState) { st.vars[name] = k })
		}
		body := g.loop(pre, 1+g.pick(2))
		fmt.Fprintf(&src, "function f%d():void\n{\n%s%s}\n", f, preSrc.String(), body.src)
		fruns = append(fruns, func(st *rgState) rgCtl {
			for _, p := range preRuns {
				p(st)
			}
			return body.run(st)
		})
	}
	// main: 配列を決まった値で埋め、関数を呼び、和を出す
	src.WriteString("function main():void\n{\n")
	for _, a := range g.arrs {
		fmt.Fprintf(&src, "for (var i:u8 = 0; i < %d; i += 1) {\nfor (var j:u8 = 0; j < %d; j += 1) {\n%s[i][j] = i * 7 + j * 3;\n}\n}\n", a.r, a.c, a.name)
	}
	for f := 0; f < nf; f++ {
		fmt.Fprintf(&src, "f%d();\n", f)
	}
	src.WriteString("var h:u16 = acc;\n")
	for _, a := range g.arrs {
		fmt.Fprintf(&src, "for (var i:u8 = 0; i < %d; i += 1) {\nfor (var j:u8 = 0; j < %d; j += 1) {\nh = h * 31 + %s[i][j] as u16;\n}\n}\n", a.r, a.c, a.name)
	}
	src.WriteString("@printf(\"{} {}\\n\", acc, h);\nconsole.exit(0);\n}\n")

	st := &rgState{arrs: map[string][]int{}, vars: map[string]int{}}
	for _, a := range g.arrs {
		d := make([]int, a.r*a.c)
		for i := 0; i < a.r; i++ {
			for j := 0; j < a.c; j++ {
				d[i*a.c+j] = (i*7 + j*3) & 0xff
			}
		}
		st.arrs[a.name] = d
	}
	for _, r := range fruns {
		r(st)
	}
	h := st.acc
	for _, a := range g.arrs {
		for _, v := range st.arrs[a.name] {
			h = (h*31 + v) & 0xffff
		}
	}
	return map[string]string{"t.fc": src.String()}, fmt.Sprintf("%d %d\n", st.acc, h)
}

func TestRandomGridV4(t *testing.T) {
	t.Parallel()
	n := max(*randN/4, 5)
	base := *randSeed
	if base == 0 {
		base = 1
	}
	for k := 0; k < n; k++ {
		seed := base + int64(k)
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			t.Parallel()
			g := &rgGen{r: rand.New(rand.NewSource(seed))}
			files, want := g.program()
			for _, level := range []int{-1, 0} {
				out, err := rpRun(t, files, level, rpMaxCycles)
				if err != nil || out != want {
					t.Fatalf("level %d (seed %d): got %q, %v\nwant %q\n%s", level, seed, out, err, want, joinSources(files))
				}
			}
			if *randInterp {
				if out, ok, err := rpInterp(t, files); ok && (err != nil || out != want) {
					t.Fatalf("interp (seed %d): got %q, %v\nwant %q\n%s", seed, out, err, want, joinSources(files))
				}
			}
		})
	}
}
