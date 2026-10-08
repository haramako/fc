package driver

// far の interface のランダムテスト (TestRandomFarIfaceNES)。nes ターゲット (UxROM、切替バンク a / b が同じ $8000) で、
// 同じ番地に違う内容の const 表 T を持つ 2 つのバンクに interface の実装を置き、固定バンクの main から ID で振り分けて
// 呼ぶ (Agent/wiki/design/methods-interface.md)。@(far) なら振り分けの表は far の関数 (呼ぶたびに実装のバンクに切り替えて
// 戻す)。実装のメソッドから別の要素のメソッドを呼ぶ (バンクの中から別のバンクへ far call を重ねる) 形も作る。@(far) の
// 無い interface では main が @bank_of_id で切り替えてから呼ぶ (実装のメソッドは別の要素を呼ばない)。期待値は生成器が
// Go で計算し、-O 0 / -O 2 の両方で、呼び出しの後のバンクが元に戻っていることも見る。

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

const farIfaceToml = "[target]\nmapper = \"UxROM\"\nprg = \"64K\"\n[bank.a]\nslot = 0x8000\n[bank.b]\nslot = 0x8000\n"

// fiElem は要素の模型 (共通のフィールド x と、実装のフィールド n)。
type fiElem struct{ id, x, n int }

// fiImpl は実装 1 つ。m[k] は k 番目のメソッドの本体 (nil なら実装しない)。
type fiImpl struct {
	name string
	mod  *fiMod
	id   int
	m    [2]*fiBody
	help bool // ヘルパー h (同じバンクの表を読む) を持つ
	hk   int
}

type fiMod struct {
	name, bank string
	tab        []int
}

// fiBody はメソッドの本体のソースと、Go での評価 (要素 i、引数 d / e)。
type fiBody struct {
	src  string
	eval func(p *fiProg, i, d, e int) int
}

type fiProg struct {
	impls []*fiImpl
	elems []*fiElem
	def0  bool // m0 に既定の本体がある (return self.x + d)
}

func (p *fiProg) implOf(id int) *fiImpl {
	for _, im := range p.impls {
		if im.id == id {
			return im
		}
	}
	return nil
}

// call は要素 i の k 番目のメソッドを ID で振り分けて実行する。
func (p *fiProg) call(k, i, d, e int) int {
	if im := p.implOf(p.elems[i].id); im != nil && im.m[k] != nil {
		return im.m[k].eval(p, i, d, e) & 255
	}
	if k == 0 && p.def0 {
		return (p.elems[i].x + d) & 255
	}
	return 0
}

// fiGenBody は実装 im の k 番目のメソッドの本体をランダムに作る。cross なら別の要素の m0 を呼ぶ形も選ぶ。
func fiGenBody(r *rand.Rand, im *fiImpl, k int, cross bool, n int) *fiBody {
	tab := im.mod.tab
	c := r.Intn(8)
	// 引数の式: m0 は d、m1 は d と e
	arg := "d"
	argv := func(d, e int) int { return d }
	if k == 1 && r.Intn(2) == 0 {
		arg = "(d ^ e)"
		argv = func(d, e int) int { return d ^ e }
	}
	switch x := r.Intn(6); {
	case x == 0:
		return &fiBody{fmt.Sprintf("self.n += %s; return self.n + T[%s & 7];", arg, arg), func(p *fiProg, i, d, e int) int {
			el := p.elems[i]
			a := argv(d, e)
			el.n = (el.n + a) & 255
			return el.n + tab[a&7]
		}}
	case x == 1:
		return &fiBody{fmt.Sprintf("return T[(self.n + %s) & 7] ^ self.x;", arg), func(p *fiProg, i, d, e int) int {
			el := p.elems[i]
			return tab[(el.n+argv(d, e))&7] ^ el.x
		}}
	case x == 2:
		m := 2 + c%5
		return &fiBody{fmt.Sprintf("var s:u8 = %s; for (var j:u8 = 0; j < %d; j++) { s += T[j]; } self.n = s; return s;", arg, m), func(p *fiProg, i, d, e int) int {
			s := argv(d, e)
			for j := 0; j < m; j++ {
				s += tab[j]
			}
			p.elems[i].n = s & 255
			return s
		}}
	case x == 3 && im.help:
		return &fiBody{fmt.Sprintf("return self.h(%s) + %d;", arg, c), func(p *fiProg, i, d, e int) int {
			return tab[(argv(d, e)+im.hk)&7] + p.elems[i].n + c
		}}
	case x >= 4 && cross:
		// 別の要素の m0 (振り分けの表で別のバンクの実装へ far call を重ねる)
		j := r.Intn(n)
		return &fiBody{fmt.Sprintf("var t = task.Tasks[%d].m0(%s); self.x = t; return t + T[e & 7];", j, arg), func(p *fiProg, i, d, e int) int {
			t := p.call(0, j, argv(d, e), 0)
			p.elems[i].x = t
			return t + tab[e&7]
		}}
	}
	return &fiBody{fmt.Sprintf("self.x ^= %s; return T[%d] + self.x;", arg, c), func(p *fiProg, i, d, e int) int {
		el := p.elems[i]
		el.x ^= argv(d, e)
		return tab[c] + el.x
	}}
}

// genFarIface は fc.toml・task・ea・eb・main を作り、out[] に入るはずの値を返す。
func genFarIface(r *rand.Rand) (map[string]string, []int) {
	far := r.Intn(4) != 0
	soa := r.Intn(3) != 0
	n := 4
	p := &fiProg{def0: r.Intn(2) == 0}
	mods := []*fiMod{{name: "ea", bank: "a"}, {name: "eb", bank: "b"}}
	for _, m := range mods {
		for i := 0; i < 8; i++ {
			m.tab = append(m.tab, r.Intn(256))
		}
	}
	// 実装 (2〜4 個。手動の ID ときどき)
	used := map[int]bool{}
	nimpl := 2 + r.Intn(3)
	for i := 0; i < nimpl; i++ {
		im := &fiImpl{name: fmt.Sprintf("I%d", i), mod: mods[i%2], help: r.Intn(2) == 0, hk: r.Intn(8)}
		if i >= 2 {
			im.mod = mods[r.Intn(2)]
		}
		if r.Intn(3) == 0 {
			for {
				id := 1 + r.Intn(8)
				if !used[id] {
					im.id, used[id] = id, true
					break
				}
			}
		}
		p.impls = append(p.impls, im)
	}
	// 自動の ID (モジュールのパスの順: ea.fc < eb.fc、その中は宣言の順)
	next := 1
	manual := map[*fiImpl]bool{}
	for _, im := range p.impls {
		manual[im] = im.id != 0
	}
	for _, m := range mods {
		for _, im := range p.impls {
			if im.mod != m || manual[im] {
				continue
			}
			for used[next] {
				next++
			}
			im.id, used[next] = next, true
		}
	}
	for _, im := range p.impls {
		for k := 0; k < 2; k++ {
			if r.Intn(5) == 0 {
				continue // 実装しない (m0 は既定の本体か 0、m1 は 0)
			}
			im.m[k] = fiGenBody(r, im, k, far && k == 1, n)
		}
	}
	files := map[string]string{"fc.toml": farIfaceToml}
	var tk strings.Builder
	tk.WriteString("#fc 4\n")
	if soa {
		tk.WriteString("public soa interface Task")
	} else {
		tk.WriteString("public interface Task")
	}
	if far {
		tk.WriteString(" @(far)")
	}
	tk.WriteString(" {\n\tx:u8;\n")
	if p.def0 {
		tk.WriteString("\tfunction m0(self:*Task, d:u8):u8\n\t{\n\t\treturn self.x + d;\n\t}\n")
	} else {
		tk.WriteString("\tfunction m0(self:*Task, d:u8):u8;\n")
	}
	tk.WriteString("\tfunction m1(self:*Task, d:u8, e:u8):u8;\n}\n")
	if soa {
		fmt.Fprintf(&tk, "public soa Tasks:[%d]Task;\n", n)
	} else {
		fmt.Fprintf(&tk, "public var Tasks:[%d]Task;\n", n)
	}
	files["task.fc"] = tk.String()
	for _, m := range mods {
		var b strings.Builder
		fmt.Fprintf(&b, "#fc 4\n@(bank: %q);\nuse task;\nconst T:[8]u8 = [", m.bank)
		for i, v := range m.tab {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprint(&b, v)
		}
		b.WriteString("];\n")
		for _, im := range p.impls {
			if im.mod != m {
				continue
			}
			fmt.Fprintf(&b, "public struct %s: task.Task", im.name)
			if manual[im] {
				fmt.Fprintf(&b, " = %d", im.id)
			}
			b.WriteString(" {\n\tn:u8;\n}\n")
			if im.help {
				fmt.Fprintf(&b, "function %s.h(self:*%s, d:u8):u8\n{\n\treturn T[(d + %d) & 7] + self.n;\n}\n", im.name, im.name, im.hk)
			}
			for k, body := range im.m {
				if body == nil {
					continue
				}
				params := "d:u8"
				if k == 1 {
					params = "d:u8, e:u8"
				}
				fmt.Fprintf(&b, "public function %s.m%d(self:*%s, %s):u8\n{\n\t%s\n}\n", im.name, k, im.name, params, body.src)
			}
		}
		files[m.name+".fc"] = b.String()
	}

	// main: 要素を作り、呼んだ結果を out に入れる
	p.elems = make([]*fiElem, n)
	var mb strings.Builder
	mb.WriteString("#fc 4\n")
	if far {
		mb.WriteString("@(farcall);\n")
	}
	mb.WriteString("use uxrom;\nuse task;\nuse ea;\nuse eb;\nuse Task from task;\npublic var out:[64]u8;\npublic var done:u8;\n")
	mb.WriteString("function main():void\n{\n\tuxrom.init();\n\tuxrom.prg(@bank(\"a\"));\n")
	for i := 0; i < n; i++ {
		el := &fiElem{x: r.Intn(256)}
		p.elems[i] = el
		if r.Intn(5) == 0 {
			// .none (共通のフィールドだけ)
			fmt.Fprintf(&mb, "\t@set_id(&task.Tasks[%d], .none);\n\ttask.Tasks[%d].x = %d;\n", i, i, el.x)
			continue
		}
		im := p.impls[r.Intn(len(p.impls))]
		el.id, el.n = im.id, r.Intn(256)
		fmt.Fprintf(&mb, "\ttask.Tasks[%d] = %s.%s{x: %d, n: %d};\n", i, im.mod.name, im.name, el.x, el.n)
	}
	var want []int
	slot := 0
	out := func(expr string, v int) {
		fmt.Fprintf(&mb, "\tout[%d] = %s;\n", slot, expr)
		want = append(want, v&255)
		slot++
	}
	for c := 0; c < 6+r.Intn(10) && slot < 60; c++ {
		i := r.Intn(n)
		k := r.Intn(2)
		d, e := r.Intn(256), r.Intn(256)
		args := fmt.Sprint(d)
		if k == 1 {
			args = fmt.Sprintf("%d, %d", d, e)
		}
		call := fmt.Sprintf("task.Tasks[%d].m%d(%s)", i, k, args)
		if !far {
			// near: 実装のバンクに自分で切り替えて呼び、元のバンクに戻す
			fmt.Fprintf(&mb, "\tuxrom.prg(@bank_of_id(@id_of(&task.Tasks[%d])));\n", i)
		}
		out(call, p.call(k, i, d, e))
		if !far {
			mb.WriteString("\tuxrom.prg(@bank(\"a\"));\n")
		}
		if r.Intn(3) == 0 {
			out("uxrom.bank", 0) // 呼び出しの後でバンクが戻っているか (a は 0)
		}
		if r.Intn(4) == 0 {
			j := r.Intn(n)
			out(fmt.Sprintf("task.Tasks[%d].x", j), p.elems[j].x)
		}
	}
	mb.WriteString("\tdone = 1;\n\twhile (true) {\n\t}\n}\n")
	files["main.fc"] = mb.String()
	return files, want
}

// TestRandomFarIfaceNES は far / near の interface のプログラムを NES で -O 0 / -O 2 で走らせ、生成器の期待値と比べる。
func TestRandomFarIfaceNES(t *testing.T) {
	t.Parallel()
	n := 8
	if *randN > 30 {
		n = *randN / 2
	}
	for k := 0; k < n; k++ {
		seed := int64(k + 1)
		if *randSeed != 0 {
			seed = *randSeed + int64(k)
		}
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			t.Parallel()
			files, want := genFarIface(rand.New(rand.NewSource(seed)))
			for _, level := range []int{-1, 0} {
				out, done, _ := runNes(t, files, level, len(want))
				if done != 1 || fmt.Sprint(out) != fmt.Sprint(want) {
					t.Fatalf("-O %d (seed %d): done=%d\ngot  %v\nwant %v\n%s", level, seed, done, out, want, joinSources(files))
				}
			}
		})
	}
}
