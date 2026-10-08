package driver

// fc 4 のメソッドと interface のランダムテスト (TestRandomMethodsV4)。fc 4 のソースを直接作り、期待値は生成器が Go で同じ
// プログラムを実行して計算する (メソッドの呼び出しの書き換え・受け取り手の合わせ方・ID の振り分けは sema の仕事で、-O 0 / -O 2 /
// インタプリタが同じように間違えるので、差分でなく期待値と比べる。Agent/wiki/design/methods-interface.md)。
//
// 生成するもの:
//   - struct P (u8 / u16 のフィールド) と、値の self・書き換える self (*P)・読むだけの self (*const P) のメソッド、P を返すメソッド
//     mix。soa Ps:[4]P とハンドルのメソッド (self:*Ps)。受け取り手はグローバル変数・配列の要素・ローカル変数・ポインタ・soa の
//     要素・関数の戻り値、`P.m(x, …)` の形の呼び出しも
//   - interface Task (soa か普通か。共通のフィールド、init と 1〜3 個のメソッド、既定の本体があるものと無いもの) と、2 つの
//     モジュール (ifc・m2) の 2〜4 個の実装 (手動と自動の ID、実装だけのフィールドとヘルパーのメソッド)。要素は実装の値の代入・
//     @set_id と init・.none・実装の無い ID で作り、ID で振り分けて呼ぶ。実装のメソッドから別の要素のメソッドを呼ぶ (番号の小さい
//     メソッドと、深さを限った自分自身の再帰: 振り分けの関数を通した呼び出しの輪)
//
// 式は符号なしの + - ^ | & * と定数の回数の << だけ (環の演算なので、計算の幅 (代入先と式のいちばん広い型) によらず、代入先の幅で
// 切れば同じ値)。`as` の中は中の幅で切る。比較は名前と定数・名前どうしだけ。未定義の動作 (読む前に書かない実装のフィールドなど) は
// 作らない: 要素は実装の ID にしたら必ず init (全部のフィールドを書く) か値の代入をする。

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"
)

// ---- 式と文 (生成器の木。fc のソースにも書き、Go でも実行する) ----

type rmKind int

const (
	rmConst rmKind = iota
	rmFld          // obj.name (obj は self / o)
	rmVar          // 引数・ローカル変数・ループの変数
	rmBin          // x op y
	rmShl          // x << k
	rmAs           // (x as T)
)

// rmE は式。size は型の大きさ (型のない定数は 0)。
type rmE struct {
	kind rmKind
	v    int
	obj  string
	name string
	size int
	op   string
	x, y *rmE
}

// width は式の計算の幅 (`as` の中は数えない。型のない定数は 1 バイトに入らなければ 2)。
func (e *rmE) width() int {
	switch e.kind {
	case rmConst:
		if e.v > 255 {
			return 2
		}
		return 0
	case rmBin:
		return max(e.x.width(), e.y.width())
	case rmShl:
		return e.x.width()
	}
	return e.size
}

func (e *rmE) String() string {
	switch e.kind {
	case rmConst:
		return fmt.Sprint(e.v)
	case rmFld:
		return e.obj + "." + e.name
	case rmVar:
		return e.name
	case rmBin:
		return "(" + e.x.String() + " " + e.op + " " + e.y.String() + ")"
	case rmShl:
		return fmt.Sprintf("(%s << %d)", e.x, e.v)
	case rmAs:
		return fmt.Sprintf("(%s as %s)", e.x, rmTypeName(e.size))
	}
	panic("rmE")
}

func rmTypeName(size int) string {
	if size == 2 {
		return "u16"
	}
	return "u8"
}

func rmMask(v, size int) int {
	if size == 2 {
		return v & 0xffff
	}
	return v & 0xff
}

// rmEnv は実行中のメソッドの環境。
type rmEnv struct {
	objs map[string]*rmObj // self / o
	vars map[string]int
}

// eval は式の値 (切っていない整数。`as` だけはその幅で切る)。
func (e *rmE) eval(env *rmEnv) int {
	switch e.kind {
	case rmConst:
		return e.v
	case rmFld:
		v, ok := env.objs[e.obj].f[e.name]
		if !ok {
			panic("rmethod: 書いていないフィールド " + e.obj + "." + e.name)
		}
		return v
	case rmVar:
		return env.vars[e.name]
	case rmBin:
		a, b := e.x.eval(env), e.y.eval(env)
		switch e.op {
		case "+":
			return a + b
		case "-":
			return a - b
		case "^":
			return a ^ b
		case "|":
			return a | b
		case "&":
			return a & b
		case "*":
			return a * b
		}
	case rmShl:
		return e.x.eval(env) << e.v
	case rmAs:
		return rmMask(rmMask(e.x.eval(env), max(e.x.width(), 1)), e.size)
	}
	panic("rmE.eval")
}

// rmCond は比較 `x op y` (x は名前、y は名前か定数)。
type rmCond struct {
	x, y *rmE
	op   string
}

func (c *rmCond) String() string { return c.x.String() + " " + c.op + " " + c.y.String() }

func (c *rmCond) eval(env *rmEnv) bool {
	a, b := c.x.eval(env), c.y.eval(env)
	switch c.op {
	case "<":
		return a < b
	case ">":
		return a > b
	case "<=":
		return a <= b
	case ">=":
		return a >= b
	case "==":
		return a == b
	}
	return a != b
}

// rmCall はメソッドの呼び出し。recv は受け取り手のソース ("self"・"Tasks[2]" など)、target は実行のときの受け取り手。
type rmCall struct {
	recv   string
	target func(env *rmEnv) *rmObj
	m      *rmMethod // struct・soa・実装のメソッド (iface が nil のとき)
	im     *rmIM     // interface のメソッド (振り分け)
	args   []*rmE
}

func (c *rmCall) String() string {
	var as []string
	for _, a := range c.args {
		as = append(as, a.String())
	}
	name := ""
	if c.m != nil {
		name = c.m.name
	} else {
		name = c.im.name
	}
	return fmt.Sprintf("%s.%s(%s)", c.recv, name, strings.Join(as, ", "))
}

type rmSKind int

const (
	rmAssign    rmSKind = iota // obj.name op= e
	rmLocal                    // var name:T = e
	rmLocalSet                 // name op= e (ローカル変数)
	rmLocalCall                // var name:T = call
	rmCallStmt                 // call;
	rmIf                       // if (cond) { then } else { els }
	rmFor                      // for (var name:u8 = 0; name < v; name++) { then }
	rmRec                      // if (p != 0 && p < 3) { call; } (深さを限った再帰)
)

// rmS は文。
type rmS struct {
	kind      rmSKind
	obj, name string
	op        string // "=" か "+=" など
	size      int
	e         *rmE
	call      *rmCall
	cond      *rmCond
	v         int
	then, els []*rmS
}

func (s *rmS) write(b *strings.Builder, ind string) {
	switch s.kind {
	case rmAssign:
		fmt.Fprintf(b, "%s%s.%s %s %s;\n", ind, s.obj, s.name, s.op, s.e)
	case rmLocal:
		fmt.Fprintf(b, "%svar %s:%s = %s;\n", ind, s.name, rmTypeName(s.size), s.e)
	case rmLocalSet:
		fmt.Fprintf(b, "%s%s %s %s;\n", ind, s.name, s.op, s.e)
	case rmLocalCall:
		fmt.Fprintf(b, "%svar %s:%s = %s;\n", ind, s.name, rmTypeName(s.size), s.call)
	case rmCallStmt:
		fmt.Fprintf(b, "%s%s;\n", ind, s.call)
	case rmIf:
		fmt.Fprintf(b, "%sif (%s) {\n", ind, s.cond)
		for _, t := range s.then {
			t.write(b, ind+"\t")
		}
		if len(s.els) > 0 {
			fmt.Fprintf(b, "%s} else {\n", ind)
			for _, t := range s.els {
				t.write(b, ind+"\t")
			}
		}
		fmt.Fprintf(b, "%s}\n", ind)
	case rmFor:
		fmt.Fprintf(b, "%sfor (var %s:u8 = 0; %s < %d; %s++) {\n", ind, s.name, s.name, s.v, s.name)
		for _, t := range s.then {
			t.write(b, ind+"\t")
		}
		fmt.Fprintf(b, "%s}\n", ind)
	case rmRec:
		fmt.Fprintf(b, "%sif (p != 0 && p < 3) {\n%s\t%s;\n%s}\n", ind, ind, s.call, ind)
	}
}

func rmApply(op string, old, v int) int {
	switch op {
	case "+=":
		return old + v
	case "-=":
		return old - v
	case "^=":
		return old ^ v
	case "|=":
		return old | v
	case "&=":
		return old & v
	}
	return v
}

// ---- 実行の模型 ----

// rmObj は struct の値・soa の要素・interface の要素 (id は interface の要素の ID)。
type rmObj struct {
	f  map[string]int
	id int
}

func (o *rmObj) clone() *rmObj {
	c := &rmObj{f: map[string]int{}, id: o.id}
	for k, v := range o.f {
		c.f[k] = v
	}
	return c
}

type rmSelfKind int

const (
	rmSelfValue rmSelfKind = iota // self:P
	rmSelfPtr                     // self:*P (soa のハンドル・実装も)
	rmSelfConst                   // self:*const P
)

// rmMethod は struct・soa・実装のメソッド (と interface の既定の本体)。
type rmMethod struct {
	name   string
	recv   string // 型の名前 (P / Ps / 実装)
	kind   rmSelfKind
	params []rmField
	ret    int // 0 は void、1 / 2 は u8 / u16、-1 は P
	body   []*rmS
	retE   *rmE
	retP   []*rmE // P を返すメソッドの各フィールド
	public bool
}

type rmField struct {
	name string
	size int
}

// rmProg は生成したプログラムと、それを実行する模型。
type rmProg struct {
	pFields []rmField
	out     strings.Builder
	steps   int
	ifc     *rmIface
}

// run は呼び出しを実行して戻り値を返す (void は 0、P を返すメソッドは obj)。
func (p *rmProg) run(m *rmMethod, self *rmObj, args []int, other *rmObj) (int, *rmObj) {
	p.steps++
	if p.steps > 200000 {
		panic("rmethod: 呼び出しが多すぎる")
	}
	if m.kind == rmSelfValue {
		self = self.clone()
	}
	env := &rmEnv{objs: map[string]*rmObj{"self": self}, vars: map[string]int{}}
	if other != nil {
		env.objs["o"] = other.clone()
	}
	for i, a := range args {
		env.vars[m.params[i].name] = rmMask(a, m.params[i].size)
	}
	p.exec(m.body, env)
	switch {
	case m.ret == -1:
		r := &rmObj{f: map[string]int{}}
		for i, f := range p.pFields {
			r.f[f.name] = rmMask(m.retP[i].eval(env), f.size)
		}
		return 0, r
	case m.ret > 0:
		return rmMask(m.retE.eval(env), m.ret), nil
	}
	return 0, nil
}

func (p *rmProg) exec(ss []*rmS, env *rmEnv) {
	for _, s := range ss {
		switch s.kind {
		case rmAssign:
			o := env.objs[s.obj]
			o.f[s.name] = rmMask(rmApply(s.op, o.f[s.name], s.e.eval(env)), s.size)
		case rmLocal:
			env.vars[s.name] = rmMask(s.e.eval(env), s.size)
		case rmLocalSet:
			env.vars[s.name] = rmMask(rmApply(s.op, env.vars[s.name], s.e.eval(env)), s.size)
		case rmLocalCall:
			env.vars[s.name] = rmMask(p.call(s.call, env), s.size)
		case rmCallStmt:
			p.call(s.call, env)
		case rmIf:
			if s.cond.eval(env) {
				p.exec(s.then, env)
			} else {
				p.exec(s.els, env)
			}
		case rmFor:
			for i := 0; i < s.v; i++ {
				env.vars[s.name] = i
				p.exec(s.then, env)
			}
		case rmRec:
			if v := env.vars["p"]; v != 0 && v < 3 {
				p.call(s.call, env)
			}
		}
	}
}

// call は呼び出しを実行する (引数は呼び出しの前に左から評価する)。
func (p *rmProg) call(c *rmCall, env *rmEnv) int {
	var args []int
	for _, a := range c.args {
		args = append(args, a.eval(env))
	}
	self := c.target(env)
	m := c.m
	if c.im != nil {
		m = p.ifc.dispatch(self.id, c.im)
		if m == nil {
			return 0
		}
	}
	r, _ := p.run(m, self, args, nil)
	return r
}

// ---- interface ----

// rmIM は interface のメソッド (def は既定の本体。無ければ nil)。
type rmIM struct {
	name   string
	params []rmField
	ret    int
	def    *rmMethod
}

type rmImpl struct {
	name    string
	mod     string // ifc / m2
	id      int
	manual  bool
	fields  []rmField
	methods map[string]*rmMethod // interface のメソッドの実装
	helpers []*rmMethod
}

type rmIface struct {
	soa     bool
	common  []rmField
	methods []*rmIM // [0] は init
	impls   []*rmImpl
	n       int // 置き場所の要素の数
	maxID   int
	elems   []*rmObj
}

func (f *rmIface) implOf(id int) *rmImpl {
	for _, im := range f.impls {
		if im.id == id {
			return im
		}
	}
	return nil
}

// dispatch は ID id の要素の im の本体 (実装・既定の本体。無ければ nil)。
func (f *rmIface) dispatch(id int, im *rmIM) *rmMethod {
	if impl := f.implOf(id); impl != nil {
		if m := impl.methods[im.name]; m != nil {
			return m
		}
	}
	return im.def
}

// ---- 生成器 ----

type rmGen struct {
	r    *rand.Rand
	n    int
	prog *rmProg
}

func (g *rmGen) pick(n int) int        { return g.r.Intn(n) }
func (g *rmGen) chance(p float64) bool { return g.r.Float64() < p }
func (g *rmGen) name(p string) string  { g.n++; return fmt.Sprintf("%s%d", p, g.n) }

// rmScope は生成中の式に使える名前。
type rmScope struct {
	objs   map[string][]rmField // self / o のフィールド
	vars   []rmField
	write  bool // self のフィールドに書ける
	calls  func(depth int) *rmCall
	locals int // 作ったローカル変数の数 (多すぎないように)
}

func (sc *rmScope) atoms() []*rmE {
	var r []*rmE
	objs := make([]string, 0, len(sc.objs))
	for o := range sc.objs {
		objs = append(objs, o)
	}
	sort.Strings(objs)
	for _, o := range objs {
		for _, f := range sc.objs[o] {
			r = append(r, &rmE{kind: rmFld, obj: o, name: f.name, size: f.size})
		}
	}
	for _, v := range sc.vars {
		r = append(r, &rmE{kind: rmVar, name: v.name, size: v.size})
	}
	return r
}

// atom は名前 1 つ (dest より広ければ as で切る)。
func (g *rmGen) atom(sc *rmScope, dest int) *rmE {
	as := sc.atoms()
	if len(as) == 0 {
		return &rmE{kind: rmConst, v: g.pick(256)}
	}
	a := as[g.pick(len(as))]
	if a.size > dest {
		return &rmE{kind: rmAs, x: a, size: dest}
	}
	return a
}

// expr は代入先の大きさ dest の式 (計算の幅が dest を超えない)。
func (g *rmGen) expr(sc *rmScope, dest, depth int) *rmE {
	switch x := g.pick(10); {
	case depth >= 2 || x < 4:
		return g.atom(sc, dest)
	case x < 5:
		v := g.pick(256)
		if dest == 2 && g.chance(0.3) {
			v = g.pick(65536)
		}
		return &rmE{kind: rmConst, v: v}
	case x < 8:
		ops := []string{"+", "-", "^", "|", "&", "*", "+", "-"}
		e := &rmE{kind: rmBin, op: ops[g.pick(len(ops))], x: g.expr(sc, dest, depth+1), y: g.expr(sc, dest, depth+1)}
		if e.x.kind == rmConst && e.y.kind == rmConst {
			e.x = g.atom(sc, dest) // 定数どうしは畳み込まれるだけ
		}
		return e
	case x < 9:
		return &rmE{kind: rmShl, x: g.atom(sc, dest), v: 1 + g.pick(3)}
	default:
		// as の中は自分の幅で計算する (u8 の a + b を u16 にすると折り返した値)
		inner := g.pick(2) + 1
		in := g.expr(sc, inner, depth+1)
		if in.width() == 0 {
			in = &rmE{kind: rmBin, op: "+", x: g.atom(sc, inner), y: in}
		}
		return &rmE{kind: rmAs, x: in, size: dest}
	}
}

// cond は比較。
func (g *rmGen) cond(sc *rmScope) *rmCond {
	ops := []string{"<", ">", "<=", ">=", "==", "!="}
	x := g.atom(sc, 2)
	if x.kind != rmFld && x.kind != rmVar {
		x = &rmE{kind: rmConst, v: 1}
		return &rmCond{x: x, op: "==", y: &rmE{kind: rmConst, v: 1}} // 名前が無い (起きない)
	}
	var y *rmE
	if g.chance(0.6) {
		lim := 256
		if x.size == 2 {
			lim = 1024
		}
		y = &rmE{kind: rmConst, v: g.pick(lim)}
	} else {
		y = g.atom(sc, 2)
		if y.kind == rmAs {
			y = y.x
		}
	}
	return &rmCond{x: x, op: ops[g.pick(len(ops))], y: y}
}

// stmts は文の並び (sc.write なら self のフィールドに書く)。
func (g *rmGen) stmts(sc *rmScope, n, depth int) []*rmS {
	var r []*rmS
	nvars := len(sc.vars)
	for i := 0; i < n; i++ {
		r = append(r, g.stmt(sc, depth))
	}
	sc.vars = sc.vars[:nvars] // ブロックの中の変数は外から見えない
	return r
}

func (g *rmGen) stmt(sc *rmScope, depth int) *rmS {
	fields := sc.objs["self"]
	for {
		switch x := g.pick(20); {
		case x < 7 && sc.write && len(fields) > 0:
			f := fields[g.pick(len(fields))]
			ops := []string{"=", "+=", "-=", "^=", "|=", "&=", "=", "+="}
			return &rmS{kind: rmAssign, obj: "self", name: f.name, size: f.size, op: ops[g.pick(len(ops))], e: g.expr(sc, f.size, 0)}
		case x < 10 && sc.locals < 6:
			size := g.pick(2) + 1
			s := &rmS{kind: rmLocal, name: g.name("t"), size: size, e: g.expr(sc, size, 0)}
			sc.vars = append(sc.vars, rmField{s.name, size})
			sc.locals++
			return s
		case x < 11:
			var ls []rmField
			for _, v := range sc.vars {
				if strings.HasPrefix(v.name, "t") {
					ls = append(ls, v)
				}
			}
			if len(ls) == 0 {
				continue
			}
			v := ls[g.pick(len(ls))]
			ops := []string{"=", "+=", "-=", "^="}
			return &rmS{kind: rmLocalSet, name: v.name, size: v.size, op: ops[g.pick(len(ops))], e: g.expr(sc, v.size, 0)}
		case x < 14 && sc.calls != nil:
			c := sc.calls(depth)
			if c == nil {
				continue
			}
			ret := c.retSize()
			if ret > 0 && sc.locals < 6 {
				s := &rmS{kind: rmLocalCall, name: g.name("t"), size: ret, call: c}
				sc.vars = append(sc.vars, rmField{s.name, ret})
				sc.locals++
				return s
			}
			if ret != 0 {
				continue
			}
			return &rmS{kind: rmCallStmt, call: c}
		case x < 17 && depth < 2:
			s := &rmS{kind: rmIf, cond: g.cond(sc)}
			s.then = g.stmts(sc, 1+g.pick(2), depth+1)
			if g.chance(0.5) {
				s.els = g.stmts(sc, 1+g.pick(2), depth+1)
			}
			return s
		case x < 19 && depth < 1:
			s := &rmS{kind: rmFor, name: g.name("w"), v: 1 + g.pick(4)}
			sc.vars = append(sc.vars, rmField{s.name, 1})
			s.then = g.stmts(sc, 1+g.pick(2), depth+1)
			sc.vars = sc.vars[:len(sc.vars)-1]
			return s
		default:
			if !sc.write {
				size := g.pick(2) + 1
				if sc.locals >= 6 {
					return &rmS{kind: rmIf, cond: g.cond(sc)} // 空の if
				}
				s := &rmS{kind: rmLocal, name: g.name("t"), size: size, e: g.expr(sc, size, 0)}
				sc.vars = append(sc.vars, rmField{s.name, size})
				sc.locals++
				return s
			}
		}
	}
}

func (c *rmCall) retSize() int {
	if c.m != nil {
		return c.m.ret
	}
	return c.im.ret
}

// params は引数 (p:u8 と、ときどき q:u16)。
func (g *rmGen) params() []rmField {
	ps := []rmField{{"p", 1}}
	if g.chance(0.4) {
		ps = append(ps, rmField{"q", 2})
	}
	return ps
}

func (g *rmGen) args(sc *rmScope, params []rmField) []*rmE {
	var r []*rmE
	for _, p := range params {
		r = append(r, g.expr(sc, p.size, 1))
	}
	return r
}

func rmParamList(self string, ps []rmField) string {
	s := self
	for _, p := range ps {
		s += ", " + p.name + ":" + rmTypeName(p.size)
	}
	return s
}

func rmRetName(ret int) string {
	switch ret {
	case 0:
		return "void"
	case -1:
		return "P"
	}
	return rmTypeName(ret)
}

// writeMethod はメソッドの宣言。
func (m *rmMethod) write(b *strings.Builder, selfType string) {
	if m.public {
		b.WriteString("public ")
	}
	if m.name == "mix" {
		fmt.Fprintf(b, "function %s.mix(self:P, o:P):P\n{\n", m.recv)
	} else {
		fmt.Fprintf(b, "function %s.%s(%s):%s\n{\n", m.recv, m.name, rmParamList("self:"+selfType, m.params), rmRetName(m.ret))
	}
	for _, s := range m.body {
		s.write(b, "\t")
	}
	switch {
	case m.ret == -1:
		var es []string
		for _, e := range m.retP {
			es = append(es, e.String())
		}
		fmt.Fprintf(b, "\treturn {%s};\n", strings.Join(es, ", "))
	case m.ret > 0:
		fmt.Fprintf(b, "\treturn %s;\n", m.retE)
	}
	b.WriteString("}\n")
}

// ---- struct P とメソッド ----

type rmStruct struct {
	fields []rmField
	ms     []*rmMethod // P のメソッド (mix を含む)
	hs     []*rmMethod // Ps のメソッド
	soa    bool
}

func (g *rmGen) structP() *rmStruct {
	st := &rmStruct{}
	for i := 0; i < 2+g.pick(3); i++ {
		st.fields = append(st.fields, rmField{fmt.Sprintf("f%d", i), 1 + g.pick(2)})
	}
	g.prog.pFields = st.fields
	for k := 0; k < 2+g.pick(4); k++ {
		m := &rmMethod{name: fmt.Sprintf("m%d", k), recv: "P", kind: rmSelfKind(g.pick(3)), params: g.params()}
		m.ret = g.pick(3)
		if m.kind != rmSelfPtr && m.ret == 0 {
			m.ret = 1 + g.pick(2) // 書き換えないメソッドは値を返す
		}
		lower := st.ms
		sc := &rmScope{objs: map[string][]rmField{"self": st.fields}, vars: append([]rmField(nil), m.params...), write: m.kind != rmSelfConst}
		sc.calls = func(depth int) *rmCall {
			var cands []*rmMethod
			for _, l := range lower {
				if m.kind == rmSelfPtr || l.kind != rmSelfPtr {
					cands = append(cands, l)
				}
			}
			if len(cands) == 0 {
				return nil
			}
			l := cands[g.pick(len(cands))]
			return &rmCall{recv: "self", target: func(env *rmEnv) *rmObj { return env.objs["self"] }, m: l, args: g.args(sc, l.params)}
		}
		m.body = g.stmts(sc, g.pick(4), 0)
		if m.ret > 0 {
			m.retE = g.expr(sc, m.ret, 0)
		}
		st.ms = append(st.ms, m)
	}
	if g.chance(0.6) {
		m := &rmMethod{name: "mix", recv: "P", kind: rmSelfValue, ret: -1}
		sc := &rmScope{objs: map[string][]rmField{"self": st.fields, "o": st.fields}}
		for _, f := range st.fields {
			m.retP = append(m.retP, g.expr(sc, f.size, 0))
		}
		st.ms = append(st.ms, m)
	}
	if g.chance(0.6) {
		st.soa = true
		for k := 0; k < 1+g.pick(3); k++ {
			m := &rmMethod{name: fmt.Sprintf("h%d", k), recv: "Ps", kind: rmSelfPtr, params: g.params(), ret: g.pick(3)}
			lower := st.hs
			sc := &rmScope{objs: map[string][]rmField{"self": st.fields}, vars: append([]rmField(nil), m.params...), write: true}
			sc.calls = func(depth int) *rmCall {
				if len(lower) == 0 {
					return nil
				}
				l := lower[g.pick(len(lower))]
				return &rmCall{recv: "self", target: func(env *rmEnv) *rmObj { return env.objs["self"] }, m: l, args: g.args(sc, l.params)}
			}
			m.body = g.stmts(sc, 1+g.pick(3), 0)
			if m.ret > 0 {
				m.retE = g.expr(sc, m.ret, 0)
			}
			st.hs = append(st.hs, m)
		}
	}
	return st
}

// ---- interface の生成 ----

func (g *rmGen) iface() *rmIface {
	f := &rmIface{soa: g.chance(0.6), n: 4 + g.pick(5)}
	for i := 0; i < g.pick(3); i++ {
		f.common = append(f.common, rmField{fmt.Sprintf("c%d", i), 1 + g.pick(2)})
	}
	g.prog.ifc = f
	f.methods = append(f.methods, &rmIM{name: "init", params: g.params()})
	for k, n := 1, 1+g.pick(3); k <= n; k++ {
		f.methods = append(f.methods, &rmIM{name: fmt.Sprintf("k%d", k), params: g.params(), ret: g.pick(3)})
	}
	// 既定の本体 (共通のフィールドだけを使う)
	for _, im := range f.methods {
		if !g.chance(0.5) {
			continue
		}
		m := &rmMethod{name: im.name, recv: "Task", kind: rmSelfPtr, params: im.params, ret: im.ret}
		sc := &rmScope{objs: map[string][]rmField{"self": f.common}, vars: append([]rmField(nil), im.params...), write: true}
		m.body = g.stmts(sc, g.pick(3), 1)
		if m.ret > 0 {
			m.retE = g.expr(sc, m.ret, 0)
		}
		im.def = m
	}
	used := map[int]bool{}
	nimpl := 2 + g.pick(3)
	for i := 0; i < nimpl; i++ {
		impl := &rmImpl{name: fmt.Sprintf("I%d", i), mod: []string{"ifc", "m2"}[g.pick(2)], methods: map[string]*rmMethod{}}
		if g.chance(0.4) {
			for {
				id := 1 + g.pick(10)
				if !used[id] {
					impl.id, impl.manual, used[id] = id, true, true
					break
				}
			}
		}
		for j := 0; j < 1+g.pick(3); j++ {
			impl.fields = append(impl.fields, rmField{fmt.Sprintf("%s_%d", strings.ToLower(impl.name), j), 1 + g.pick(2)})
		}
		f.impls = append(f.impls, impl)
	}
	// 自動の ID: モジュールのパスの順 (ifc.fc < m2.fc) と宣言の順で、空いている番号を 1 から
	next := 1
	for _, mod := range []string{"ifc", "m2"} {
		for _, impl := range f.impls {
			if impl.mod != mod || impl.manual {
				continue
			}
			for used[next] {
				next++
			}
			impl.id = next
			used[next] = true
		}
	}
	for id := range used {
		f.maxID = max(f.maxID, id)
	}
	for _, impl := range f.impls {
		all := append(append([]rmField(nil), f.common...), impl.fields...)
		pub := true // main が実装のハンドルで直接呼ぶ
		// ヘルパー (実装だけのメソッド)
		for j := 0; j < g.pick(3); j++ {
			h := &rmMethod{name: fmt.Sprintf("x%d", j), recv: impl.name, kind: rmSelfPtr, params: g.params(), ret: g.pick(3), public: pub}
			lower := impl.helpers
			sc := &rmScope{objs: map[string][]rmField{"self": all}, vars: append([]rmField(nil), h.params...), write: true}
			sc.calls = g.selfCalls(sc, lower)
			h.body = g.stmts(sc, 1+g.pick(3), 0)
			if h.ret > 0 {
				h.retE = g.expr(sc, h.ret, 0)
			}
			impl.helpers = append(impl.helpers, h)
		}
		for k, im := range f.methods {
			if k > 0 && g.chance(0.25) {
				continue // 実装しない (既定の本体か、何もしない)
			}
			m := &rmMethod{name: im.name, recv: impl.name, kind: rmSelfPtr, params: im.params, ret: im.ret, public: pub}
			sc := &rmScope{objs: map[string][]rmField{"self": all}, vars: append([]rmField(nil), im.params...), write: true}
			if k == 0 {
				// init は全部のフィールドを書く (式は書いた後のフィールドだけを読む)
				for i, fl := range all {
					isc := &rmScope{objs: map[string][]rmField{"self": all[:i]}, vars: sc.vars}
					m.body = append(m.body, &rmS{kind: rmAssign, obj: "self", name: fl.name, size: fl.size, op: "=", e: g.expr(isc, fl.size, 0)})
				}
			}
			helperCalls := g.selfCalls(sc, impl.helpers)
			lowerIM := f.methods[:k]
			sc.calls = func(depth int) *rmCall {
				if k > 0 && len(lowerIM) > 0 && g.chance(0.4) {
					// 別の要素の、番号の小さいメソッド (振り分け)
					l := lowerIM[g.pick(len(lowerIM))]
					return g.elemCall(sc, impl.mod, l)
				}
				return helperCalls(depth)
			}
			m.body = append(m.body, g.stmts(sc, g.pick(3), 0)...)
			if k > 0 && g.chance(0.3) {
				// 深さを限った自分自身の再帰 (振り分けの関数を通した輪)
				c := g.elemCall(sc, impl.mod, im)
				c.args[0] = &rmE{kind: rmBin, op: "-", x: &rmE{kind: rmVar, name: "p", size: 1}, y: &rmE{kind: rmConst, v: 1}}
				m.body = append(m.body, &rmS{kind: rmRec, call: c})
			}
			if m.ret > 0 {
				m.retE = g.expr(sc, m.ret, 0)
			}
			impl.methods[im.name] = m
		}
	}
	f.elems = make([]*rmObj, f.n)
	for i := range f.elems {
		f.elems[i] = &rmObj{f: map[string]int{}}
	}
	return f
}

// selfCalls は self のメソッド (helpers の中から) を呼ぶ式を作る関数。
func (g *rmGen) selfCalls(sc *rmScope, helpers []*rmMethod) func(int) *rmCall {
	return func(depth int) *rmCall {
		if len(helpers) == 0 {
			return nil
		}
		h := helpers[g.pick(len(helpers))]
		return &rmCall{recv: "self", target: func(env *rmEnv) *rmObj { return env.objs["self"] }, m: h, args: g.args(sc, h.params)}
	}
}

// elemCall はモジュール mod の中から、置き場所の要素 k の interface のメソッド im を呼ぶ式。
func (g *rmGen) elemCall(sc *rmScope, mod string, im *rmIM) *rmCall {
	f := g.prog.ifc
	k := g.pick(f.n)
	recv := fmt.Sprintf("Tasks[%d]", k)
	if mod != "ifc" {
		recv = "ifc." + recv
	}
	return &rmCall{recv: recv, target: func(*rmEnv) *rmObj { return f.elems[k] }, im: im, args: g.args(sc, im.params)}
}

// sources は ifc.fc と m2.fc。
func (f *rmIface) sources() (ifc, m2 string) {
	var a, b strings.Builder
	a.WriteString("#fc 4\n")
	b.WriteString("#fc 4\nuse ifc;\n")
	if f.soa {
		a.WriteString("public soa interface Task {\n")
	} else {
		a.WriteString("public interface Task {\n")
	}
	for _, c := range f.common {
		fmt.Fprintf(&a, "\t%s:%s;\n", c.name, rmTypeName(c.size))
	}
	for _, im := range f.methods {
		head := fmt.Sprintf("function %s(%s):%s", im.name, rmParamList("self:*Task", im.params), rmRetName(im.ret))
		if im.def == nil {
			fmt.Fprintf(&a, "\t%s;\n", head)
			continue
		}
		var body strings.Builder
		im.def.write(&body, "*Task")
		// "function Task.name(...)" の宣言を interface の中の "function name(...)" にする
		s := strings.Replace(body.String(), "function Task."+im.name+"(", "function "+im.name+"(", 1)
		for _, l := range strings.Split(strings.TrimSuffix(s, "\n"), "\n") {
			a.WriteString("\t" + l + "\n")
		}
	}
	a.WriteString("}\n")
	if f.soa {
		fmt.Fprintf(&a, "public soa Tasks:[%d]Task;\n", f.n)
	} else {
		fmt.Fprintf(&a, "public var Tasks:[%d]Task;\n", f.n)
	}
	for _, impl := range f.impls {
		w := &a
		iface := "Task"
		if impl.mod == "m2" {
			w, iface = &b, "ifc.Task"
		}
		fmt.Fprintf(w, "public struct %s: %s", impl.name, iface)
		if impl.manual {
			fmt.Fprintf(w, " = %d", impl.id)
		}
		w.WriteString(" {\n")
		for _, fl := range impl.fields {
			fmt.Fprintf(w, "\t%s:%s;\n", fl.name, rmTypeName(fl.size))
		}
		w.WriteString("}\n")
		for _, h := range impl.helpers {
			h.write(w, "*"+impl.name)
		}
		for _, im := range f.methods {
			if m := impl.methods[im.name]; m != nil {
				m.write(w, "*"+impl.name)
			}
		}
	}
	return a.String(), b.String()
}

// ---- main ----

// rmMain は main の文を作りながら模型で実行する。
type rmMain struct {
	g    *rmGen
	st   *rmStruct
	b    strings.Builder
	env  *rmEnv
	gp   *rmObj
	pa   []*rmObj
	ps   []*rmObj
	lp   *rmObj
	lvar []rmField // main のローカル変数 (引数の式に使う)
}

func (mn *rmMain) line(s string) { mn.b.WriteString("\t" + s + "\n") }

func (mn *rmMain) print(expr string, v int) {
	mn.line(fmt.Sprintf(`@printf("{}\n", %s);`, expr))
	fmt.Fprintf(&mn.g.prog.out, "%d\n", v)
}

func (mn *rmMain) printBool(expr string, v bool) {
	mn.line(fmt.Sprintf(`@printf("{}\n", %s);`, expr))
	fmt.Fprintf(&mn.g.prog.out, "%v\n", v)
}

func (mn *rmMain) argScope() *rmScope {
	return &rmScope{objs: map[string][]rmField{}, vars: mn.lvar}
}

// args は引数の式と値。
func (mn *rmMain) args(params []rmField) ([]*rmE, []int) {
	es := mn.g.args(mn.argScope(), params)
	var vs []int
	for _, e := range es {
		vs = append(vs, e.eval(mn.env))
	}
	return es, vs
}

func rmArgList(es []*rmE) string {
	var s []string
	for _, e := range es {
		s = append(s, e.String())
	}
	return strings.Join(s, ", ")
}

// pLiteral は P の値のリテラル (名前付きか順番) と、その値。
func (mn *rmMain) pLiteral(typed bool) (string, *rmObj) {
	g := mn.g
	o := &rmObj{f: map[string]int{}}
	var parts []string
	named := g.chance(0.5)
	for _, f := range mn.st.fields {
		e := g.expr(mn.argScope(), f.size, 1)
		o.f[f.name] = rmMask(e.eval(mn.env), f.size)
		if named {
			parts = append(parts, f.name+": "+e.String())
		} else {
			parts = append(parts, e.String())
		}
	}
	s := "{" + strings.Join(parts, ", ") + "}"
	if typed {
		s = "P" + s
	}
	return s, o
}

// pRecv は P の受け取り手 (ソース・値・書き換えられるか)。
func (mn *rmMain) pRecv(allowQ bool) (string, *rmObj, bool) {
	k := mn.g.pick(5)
	if k == 4 && !allowQ {
		k = 0
	}
	switch k {
	case 0:
		return "gp", mn.gp, true
	case 1, 2:
		i := mn.g.pick(len(mn.pa))
		return fmt.Sprintf("pa[%d]", i), mn.pa[i], true
	case 3:
		return "lp", mn.lp, true
	default:
		i := mn.g.pick(len(mn.pa))
		mn.line(fmt.Sprintf("q = &pa[%d];", i))
		return "q", mn.pa[i], true
	}
}

func (mn *rmMain) structStmt() {
	g, st := mn.g, mn.st
	var plain []*rmMethod
	var mix *rmMethod
	for _, m := range st.ms {
		if m.name == "mix" {
			mix = m
		} else {
			plain = append(plain, m)
		}
	}
	switch x := g.pick(12); {
	case x < 5:
		m := plain[g.pick(len(plain))]
		recv, obj, _ := mn.pRecv(true)
		es, vs := mn.args(m.params)
		call := fmt.Sprintf("%s.%s(%s)", recv, m.name, rmArgList(es))
		if g.chance(0.2) {
			// T.m(x, …) の形 (self の形に合わせて & を付ける)
			arg := recv
			if m.kind != rmSelfValue && recv != "q" {
				arg = "&" + recv
			} else if m.kind == rmSelfValue && recv == "q" {
				arg = "*q"
			}
			call = fmt.Sprintf("P.%s(%s)", m.name, rmArgList(append([]*rmE{{kind: rmVar, name: arg}}, es...)))
		}
		r, _ := g.prog.run(m, obj, vs, nil)
		if m.ret == 0 {
			mn.line(call + ";")
		} else {
			mn.print(call, r)
		}
	case x < 6:
		// 関数の戻り値 (値の self のメソッドだけ)
		var vals []*rmMethod
		for _, m := range plain {
			if m.kind == rmSelfValue {
				vals = append(vals, m)
			}
		}
		if len(vals) == 0 {
			return
		}
		m := vals[g.pick(len(vals))]
		k := g.pick(256)
		o := &rmObj{f: map[string]int{}}
		for i, f := range st.fields {
			o.f[f.name] = rmMask(k*(i+3)+i, f.size)
		}
		es, vs := mn.args(m.params)
		r, _ := g.prog.run(m, o, vs, nil)
		mn.print(fmt.Sprintf("mk(%d).%s(%s)", k, m.name, rmArgList(es)), r)
	case x < 8 && mix != nil:
		recv, obj, _ := mn.pRecv(false)
		recv2, obj2, _ := mn.pRecv(false)
		_, r := g.prog.run(mix, obj, nil, obj2)
		dst, dobj, _ := mn.pRecv(true)
		if dst == "q" {
			dst = "*q"
		}
		mn.line(fmt.Sprintf("%s = %s.mix(%s);", dst, recv, recv2))
		for k, v := range r.f {
			dobj.f[k] = v
		}
	case x < 9:
		recv, obj, _ := mn.pRecv(true)
		if recv == "q" {
			recv = "*q"
		}
		s, o := mn.pLiteral(g.chance(0.5))
		mn.line(fmt.Sprintf("%s = %s;", recv, s))
		for k, v := range o.f {
			obj.f[k] = v
		}
	case x < 11 && st.soa:
		i := g.pick(4)
		if g.chance(0.15) {
			// soa を @len(Ps) で回して、あるフィールドを足す (u16 で)
			f := st.fields[g.pick(len(st.fields))]
			lv := g.name("j")
			mn.line(fmt.Sprintf("{ var sum:u16 = 0; for (var %s:u8 = 0; %s < @len(Ps); %s++) { sum += Ps[%s].%s; } @printf(\"{}\\n\", sum); }", lv, lv, lv, lv, f.name))
			sum := 0
			for _, o := range mn.ps {
				sum += o.f[f.name]
			}
			fmt.Fprintf(&g.prog.out, "%d\n", sum&0xffff)
			return
		}
		if len(st.hs) > 0 && g.chance(0.6) {
			m := st.hs[g.pick(len(st.hs))]
			es, vs := mn.args(m.params)
			r, _ := g.prog.run(m, mn.ps[i], vs, nil)
			recv := fmt.Sprintf("Ps[%d]", i)
			if g.chance(0.3) {
				mn.line(fmt.Sprintf("hp = &Ps[%d];", i))
				recv = "hp"
			}
			call := fmt.Sprintf("%s.%s(%s)", recv, m.name, rmArgList(es))
			if m.ret == 0 {
				mn.line(call + ";")
			} else {
				mn.print(call, r)
			}
			return
		}
		// soa の要素を値として読む (値の self のメソッド)
		for _, m := range plain {
			if m.kind == rmSelfValue {
				es, vs := mn.args(m.params)
				r, _ := g.prog.run(m, mn.ps[i], vs, nil)
				mn.print(fmt.Sprintf("Ps[%d].%s(%s)", i, m.name, rmArgList(es)), r)
				return
			}
		}
	default:
		recv, obj, _ := mn.pRecv(true)
		if st.soa && g.chance(0.4) {
			i := g.pick(4)
			recv, obj = fmt.Sprintf("Ps[%d]", i), mn.ps[i]
		}
		f := st.fields[g.pick(len(st.fields))]
		mn.print(recv+"."+f.name, obj.f[f.name])
	}
}

// ---- main の interface の文 ----

// implLiteral は実装の値のリテラル。
func (mn *rmMain) implLiteral(impl *rmImpl) (string, *rmObj) {
	g, f := mn.g, mn.g.prog.ifc
	o := &rmObj{f: map[string]int{}, id: impl.id}
	all := append(append([]rmField(nil), f.common...), impl.fields...)
	var parts []string
	named := g.chance(0.5)
	for _, fl := range all {
		if named && g.chance(0.2) {
			o.f[fl.name] = 0 // 書かなかったフィールドは 0
			continue
		}
		e := g.expr(mn.argScope(), fl.size, 1)
		o.f[fl.name] = rmMask(e.eval(mn.env), fl.size)
		if named {
			parts = append(parts, fl.name+": "+e.String())
		} else {
			parts = append(parts, e.String())
		}
	}
	return fmt.Sprintf("%s.%s{%s}", impl.mod, impl.name, strings.Join(parts, ", ")), o
}

// setElem は要素 i を作り直す。
func (mn *rmMain) setElem(i int) {
	g, f := mn.g, mn.g.prog.ifc
	e := f.elems[i]
	elem := fmt.Sprintf("ifc.Tasks[%d]", i)
	switch x := g.pick(10); {
	case x < 4:
		impl := f.impls[g.pick(len(f.impls))]
		s, o := mn.implLiteral(impl)
		mn.line(fmt.Sprintf("%s = %s;", elem, s))
		*e = *o
	case x < 8:
		impl := f.impls[g.pick(len(f.impls))]
		idText := []string{"." + impl.name, "Task.Id." + impl.name, fmt.Sprintf("%d as Task.Id", impl.id)}[g.pick(3)]
		mn.line(fmt.Sprintf("@set_id(&%s, %s);", elem, idText))
		if e.id != impl.id {
			// 実装のフィールドは読めなくなる (init が全部書く)
			kept := map[string]int{}
			for _, fl := range f.common {
				if v, ok := e.f[fl.name]; ok {
					kept[fl.name] = v
				}
			}
			e.f = kept
		}
		e.id = impl.id
		mn.dispatch(i, f.methods[0], false)
	default:
		// .none か実装の無い ID: 共通のフィールドを書く
		id := 0
		var holes []int
		for k := 1; k <= f.maxID; k++ {
			if f.implOf(k) == nil {
				holes = append(holes, k)
			}
		}
		idText := ".none"
		if len(holes) > 0 && g.chance(0.5) {
			id = holes[g.pick(len(holes))]
			idText = fmt.Sprintf("%d as Task.Id", id)
		}
		mn.line(fmt.Sprintf("@set_id(&%s, %s);", elem, idText))
		e.f = map[string]int{}
		e.id = id
		for _, fl := range f.common {
			v := g.pick(256)
			mn.line(fmt.Sprintf("%s.%s = %d;", elem, fl.name, v))
			e.f[fl.name] = v
		}
		if g.chance(0.5) {
			mn.dispatch(i, f.methods[0], false)
		}
	}
}

// dispatch は要素 i の im を呼ぶ文 (値を返すなら出力する)。viaHandle なら変数のハンドル・ポインタで呼ぶ。
func (mn *rmMain) dispatch(i int, im *rmIM, viaHandle bool) {
	f := mn.g.prog.ifc
	es, vs := mn.args(im.params)
	recv := fmt.Sprintf("ifc.Tasks[%d]", i)
	if viaHandle {
		mn.line(fmt.Sprintf("e = &ifc.Tasks[%d];", i))
		recv = "e"
	}
	call := fmt.Sprintf("%s.%s(%s)", recv, im.name, rmArgList(es))
	r := 0
	if m := f.dispatch(f.elems[i].id, im); m != nil {
		r, _ = mn.g.prog.run(m, f.elems[i], vs, nil)
	}
	if im.ret == 0 {
		mn.line(call + ";")
	} else {
		mn.print(call, r)
	}
}

func (mn *rmMain) ifaceStmt() {
	g, f := mn.g, mn.g.prog.ifc
	i := g.pick(f.n)
	elem := fmt.Sprintf("ifc.Tasks[%d]", i)
	switch x := g.pick(14); {
	case x < 5:
		mn.dispatch(i, f.methods[1+g.pick(len(f.methods)-1)], g.chance(0.25))
	case x < 6:
		// 全部の要素を回して呼ぶ (void のメソッド)
		var voids []*rmIM
		for _, im := range f.methods[1:] {
			if im.ret == 0 {
				voids = append(voids, im)
			}
		}
		if len(voids) == 0 {
			return
		}
		im := voids[g.pick(len(voids))]
		lv := g.name("j")
		sc := mn.argScope()
		sc.vars = append(sc.vars, rmField{lv, 1})
		es := g.args(sc, im.params)
		mn.line(fmt.Sprintf("for (var %s:u8 = 0; %s < %d; %s++) {", lv, lv, f.n, lv))
		mn.line(fmt.Sprintf("\tifc.Tasks[%s].%s(%s);", lv, im.name, rmArgList(es)))
		mn.line("}")
		for k := 0; k < f.n; k++ {
			mn.env.vars[lv] = k
			var vs []int
			for _, e := range es {
				vs = append(vs, e.eval(mn.env))
			}
			if m := f.dispatch(f.elems[k].id, im); m != nil {
				g.prog.run(m, f.elems[k], vs, nil)
			}
		}
		delete(mn.env.vars, lv)
	case x < 8:
		mn.setElem(i)
	case x < 9 && len(f.common) > 0:
		c := f.common[g.pick(len(f.common))]
		mn.print(elem+"."+c.name, f.elems[i].f[c.name])
	case x < 10:
		if g.chance(0.5) {
			mn.print(fmt.Sprintf("@id_of(&%s) as u8", elem), f.elems[i].id)
		} else {
			impl := f.impls[g.pick(len(f.impls))]
			mn.printBool(fmt.Sprintf("@id_of(&%s) == .%s", elem, impl.name), f.elems[i].id == impl.id)
		}
	case x < 12:
		// 実装のハンドルで、実装のフィールドを読む・実装のメソッドを直接呼ぶ
		impl := f.implOf(f.elems[i].id)
		if impl == nil {
			return
		}
		h := fmt.Sprintf("@bitcast(*%s.%s, &%s)", impl.mod, impl.name, elem)
		var ms []*rmMethod
		for _, im := range f.methods[1:] {
			if m := impl.methods[im.name]; m != nil {
				ms = append(ms, m)
			}
		}
		ms = append(ms, impl.helpers...)
		if len(ms) > 0 && g.chance(0.5) {
			m := ms[g.pick(len(ms))]
			es, vs := mn.args(m.params)
			r, _ := g.prog.run(m, f.elems[i], vs, nil)
			call := fmt.Sprintf("%s.%s(%s)", h, m.name, rmArgList(es))
			if m.ret == 0 {
				mn.line(call + ";")
			} else {
				mn.print(call, r)
			}
			return
		}
		fl := impl.fields[g.pick(len(impl.fields))]
		mn.print(h+"."+fl.name, f.elems[i].f[fl.name])
	default:
		if f.soa {
			mn.dispatch(i, f.methods[g.pick(len(f.methods))], true)
			return
		}
		// 普通の interface: 要素の値を写す (ローカル変数・別の要素)
		if g.chance(0.5) {
			j := g.pick(f.n)
			mn.line(fmt.Sprintf("ifc.Tasks[%d] = %s;", j, elem))
			*f.elems[j] = *f.elems[i].clone()
			return
		}
		mn.line(fmt.Sprintf("lt = %s;", elem))
		saved := f.elems[i]
		lt := saved.clone()
		var im *rmIM
		for k := 0; k < 1+g.pick(2); k++ {
			im = f.methods[1+g.pick(len(f.methods)-1)]
			es, vs := mn.args(im.params)
			r := 0
			if m := f.dispatch(lt.id, im); m != nil {
				r, _ = g.prog.run(m, lt, vs, nil)
			}
			call := fmt.Sprintf("lt.%s(%s)", im.name, rmArgList(es))
			if im.ret == 0 {
				mn.line(call + ";")
			} else {
				mn.print(call, r)
			}
		}
	}
}

// program はプログラム (t.fc・ifc.fc・m2.fc) と期待する出力。
func (g *rmGen) program() (map[string]string, string) {
	g.prog = &rmProg{}
	st := g.structP()
	var f *rmIface
	if g.chance(0.85) {
		f = g.iface()
		g.prog.ifc = f
	}
	var b strings.Builder
	b.WriteString("#fc 4\nuse console;\n")
	if f != nil {
		b.WriteString("use ifc;\nuse m2;\nuse Task from ifc;\n")
	}
	b.WriteString("struct P {\n")
	for _, fl := range st.fields {
		fmt.Fprintf(&b, "\t%s:%s;\n", fl.name, rmTypeName(fl.size))
	}
	b.WriteString("}\n")
	if st.soa {
		b.WriteString("soa Ps:[4]P;\n")
	}
	b.WriteString("var gp:P;\nvar pa:[4]P;\n")
	for _, m := range st.ms {
		self := map[rmSelfKind]string{rmSelfValue: "P", rmSelfPtr: "*P", rmSelfConst: "*const P"}[m.kind]
		m.write(&b, self)
	}
	for _, m := range st.hs {
		m.write(&b, "*Ps")
	}
	b.WriteString("function mk(k:u8):P\n{\n\tvar r:P;\n")
	for i, fl := range st.fields {
		fmt.Fprintf(&b, "\tr.%s = (k * %d) + %d;\n", fl.name, i+3, i)
	}
	b.WriteString("\treturn r;\n}\n")

	mn := &rmMain{g: g, st: st, env: &rmEnv{objs: map[string]*rmObj{}, vars: map[string]int{}}}
	newP := func() *rmObj {
		o := &rmObj{f: map[string]int{}}
		for _, fl := range st.fields {
			o.f[fl.name] = 0
		}
		return o
	}
	mn.gp, mn.lp = newP(), newP()
	for i := 0; i < 4; i++ {
		mn.pa = append(mn.pa, newP())
		mn.ps = append(mn.ps, newP())
	}
	// main のローカル変数 (引数の式に使う)
	for i := 0; i < 2; i++ {
		size := 1 + g.pick(2)
		v := g.pick(256)
		if size == 2 {
			v = g.pick(65536)
		}
		name := fmt.Sprintf("v%d", i)
		mn.line(fmt.Sprintf("var %s:%s = %d;", name, rmTypeName(size), v))
		mn.lvar = append(mn.lvar, rmField{name, size})
		mn.env.vars[name] = v
	}
	mn.line("var lp:P;")
	mn.line("var q:*P = &gp;")
	if st.soa {
		mn.line("var hp = &Ps[0];")
	}
	// 全部の値を書いておく (グローバル変数は 0 で始まるが、ローカル変数 lp は書くまで決まらない)
	for i := 0; i < 4; i++ {
		s, o := mn.pLiteral(true)
		mn.line(fmt.Sprintf("pa[%d] = %s;", i, s))
		mn.pa[i] = o
		if st.soa {
			for _, fl := range st.fields {
				v := g.pick(256)
				mn.line(fmt.Sprintf("Ps[%d].%s = %d;", i, fl.name, v))
				mn.ps[i].f[fl.name] = v
			}
		}
	}
	s, o := mn.pLiteral(false)
	mn.line("lp = " + s + ";")
	mn.lp = o
	s, o = mn.pLiteral(true)
	mn.line("gp = " + s + ";")
	mn.gp = o
	if f != nil {
		mn.line("var e = &ifc.Tasks[0];")
		if !f.soa {
			mn.line("var lt:Task;")
		}
		for i := 0; i < f.n; i++ {
			mn.setElem(i)
		}
	}
	for k := 0; k < 15+g.pick(15); k++ {
		if f != nil && g.chance(0.6) {
			mn.ifaceStmt()
		} else {
			mn.structStmt()
		}
	}
	b.WriteString("function main():void\n{\n" + mn.b.String() + "\tconsole.exit(0);\n}\n")
	files := map[string]string{"t.fc": b.String()}
	if f != nil {
		files["ifc.fc"], files["m2.fc"] = f.sources()
	}
	return files, g.prog.out.String()
}

// TestRandomMethodsV4 は fc 4 のメソッドと interface のプログラムを -O 0 / -O 2 / インタプリタで走らせ、生成器の期待値と比べる。
func TestRandomMethodsV4(t *testing.T) {
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
			g := &rmGen{r: rand.New(rand.NewSource(seed))}
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
