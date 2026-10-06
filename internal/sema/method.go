package sema

// struct・soa のメソッド (fc 4 の `function T.m(self:…, …)`。Agent/wiki/design/methods-interface.md)。
//
// メソッドはモジュールの名前の表に入れず、受け取り手の型の修飾名 (mod.T。struct も soa も types.Type.Name) → メソッドの名前の
// 表 (Program.methods) に置く。メソッドを足せるのは型を宣言したモジュールだけ。中身は普通の関数 (シンボルは _mod_T__m)。
//
// 呼び出し `x.m(args)` は constEval が `T.m(受け取り手, args)` に書き換える (型を決める段も lval も constEval を通るので、同じ
// 書き換えを見る: methodCall)。受け取り手は self の型に合わせる: 値の self (`self:T`) には値 (ポインタなら `*x`)、ポインタの self
// (`self:*T` / `self:*const T`) には `&x` (ポインタならそのまま)、soa のハンドルの self (`self:*S`) にはハンドル。フィールドと
// 同じ名前のメソッドはエラー (`x.f(...)` が関数ポインタのフィールドの呼び出しと紛れない)。`T.m` は関数の値 (`T.m(p, q)` と呼べる)。

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// methodDecl はメソッド 1 つ。
type methodDecl struct {
	decl   *declaration
	fd     *syntax.FuncDecl
	module *ir.Module
	public bool
	val    *ir.Value // 関数の値 (解決したら)
}

// name はメソッドの表示名 (T.m)。
func (m *methodDecl) name() string { return m.fd.Recv.Name + "." + m.fd.Name.Name }

// collectMethod はメソッドの宣言を表に登録する (受け取り手の型は解決のときに確かめる: 宣言の順によらない)。
func (md *moduleDecls) collectMethod(d *declaration, s *syntax.FuncDecl) {
	m := md.addMethod(d, s)
	d.action = func(h *Hlc) { h.compileMethod(m) }
}

// addMethod はメソッドの宣言 fd (Recv が受け取り手の型の名前) を表に登録する (同じ名前があればエラー)。
func (md *moduleDecls) addMethod(d *declaration, fd *syntax.FuncDecl) *methodDecl {
	p := md.h.prog
	key := md.h.module.Id + "." + fd.Recv.Name
	if p.methods == nil {
		p.methods = map[string]map[string]*methodDecl{}
	}
	ms := p.methods[key]
	if ms == nil {
		ms = map[string]*methodDecl{}
		p.methods[key] = ms
	}
	if _, dup := ms[fd.Name.Name]; dup {
		panic(&diag.Error{Msg: fmt.Sprintf("method %s.%s already defined", fd.Recv.Name, fd.Name.Name), Pos: syntax.At(md.h.module.Path, fd.Name.NamePos)})
	}
	m := &methodDecl{decl: d, fd: fd, module: md.h.module, public: fd.PublicPos.IsValid()}
	ms[fd.Name.Name] = m
	return m
}

// compileMethod はメソッドの本体を関数にする (宣言の解決)。
func (h *Hlc) compileMethod(m *methodDecl) {
	s := m.fd
	recv := s.Recv.Name
	sym := h.scope.declares[recv]
	if sym != nil && sym.Type != nil && sym.Type.Kind == types.Bad {
		panic(&diag.Error{Suppressed: true})
	}
	if sym == nil || sym.Type == nil || !(sym.Type.Kind == types.Struct && !sym.Type.IsSlice() || sym.Type.Kind == types.Soa) || h.prog.ifaces[sym.Type] != nil {
		h.updatePos(s.Recv)
		panic(&diag.Error{Msg: fmt.Sprintf("method %s: %s is not a struct or soa declared in this module (a method can only be added to a type of its own module)", m.name(), recv)})
	}
	t := sym.Type
	elem := t
	if t.Kind == types.Soa {
		elem = h.soaElement(t)
	}
	h.completeType(elem)
	if _, ok := elem.Field(s.Name.Name); ok {
		h.updatePos(s.Name)
		panic(&diag.Error{Msg: fmt.Sprintf("method %s: %s has a field %s (a method cannot have the name of a field)", m.name(), recv, s.Name.Name)})
	}
	want := fmt.Sprintf("self:%s, self:*%s or self:*const %s", recv, recv, recv)
	if t.Kind == types.Soa {
		want = "self:*" + recv
	}
	if len(s.Params) == 0 {
		panic(&diag.Error{Msg: fmt.Sprintf("method %s needs the receiver as the first parameter (%s)", m.name(), want)})
	}
	lam := &cexpr{kind: cLambda, pos: s.Pos(), lam: &lambdaLit{
		name: m.name(), sym: methodSym(h.module.Id, recv, s.Name.Name),
		params: funcParams(s.Params), result: s.Result, body: s.Body, options: parseOptions(s.Options),
	}}
	val := h.constEval(lam).val
	pt := val.Type.Params[0]
	ok := false
	switch t.Kind {
	case types.Struct:
		ok = pt == t || pt.Kind == types.Pointer && pt.Base == t || pt.Kind == types.SoaRef && pt.Base == t && pt.Path == "" // soa の interface・実装はハンドル
	case types.Soa:
		ok = pt.Kind == types.SoaRef && pt.Soa == t && pt.Path == ""
	}
	if !ok {
		h.updatePos(s.Params[0].Name)
		panic(&diag.Error{Msg: fmt.Sprintf("method %s: the first parameter is the receiver (%s), not %s", m.name(), want, pt)})
	}
	m.val = val
}

// lookupMethod は型 t (struct・soa のコンテナ) のメソッド name (無ければ nil)。宣言を解決し、ほかのモジュールからは public だけ。
func (h *Hlc) lookupMethod(t *types.Type, name string) *methodDecl {
	if t == nil || t.Name == "" || (t.Kind != types.Struct && t.Kind != types.Soa) {
		return nil
	}
	m := h.prog.methods[t.Name][name]
	if m == nil {
		return nil
	}
	m.decl.resolve()
	if m.module != h.module && !m.public {
		panic(&diag.Error{Msg: fmt.Sprintf("method %s is private (declare it with `public` in module %s)", m.name(), m.module.Id)})
	}
	return m
}

// methodValue は `T.m` (left が型名・soa のコンテナ) のメソッドの関数の値 (メソッドでなければ nil)。
func (h *Hlc) methodValue(left *cexpr, name string) *cexpr {
	var t *types.Type
	switch {
	case left.kind == cName && left.sym.Type != nil:
		t = left.sym.Type
	case left.kind == cValue && left.val.Type != nil && left.val.Type.Kind == types.Soa:
		t = left.val.Type
	default:
		return nil
	}
	if m := h.lookupMethod(t, name); m != nil {
		return cv(m.val)
	}
	return nil
}

// methodCall は呼び出し c (評価済み: args[0] が `x.m`) がメソッドの呼び出しなら `T.m(受け取り手, args)` (評価済み) にする
// (そうでなければ nil)。x の型にフィールド m があればフィールド (関数ポインタ) の呼び出し。
func (h *Hlc) methodCall(c *cexpr) *cexpr {
	f := c.args[0]
	if f.kind != cOp || f.op != opField {
		return nil
	}
	x := f.args[0]
	info, ok := h.exprType(x)
	if !ok || info.untyped {
		return nil
	}
	xt := info.t
	var cands []*types.Type // メソッドを探す型 (先のものから)
	switch {
	case xt.Kind == types.Struct && !xt.IsSlice():
		if _, has := xt.Field(f.name); has {
			return nil
		}
		cands = []*types.Type{xt}
	case xt.Kind == types.Pointer && xt.Base.Kind == types.Struct && !xt.Base.IsSlice():
		if _, has := xt.Base.Field(f.name); has {
			return nil
		}
		cands = []*types.Type{xt.Base}
	case xt.Kind == types.SoaRef && xt.Path == "":
		if _, has := xt.Base.Field(f.name); has {
			return nil
		}
		cands = []*types.Type{xt.Soa, xt.Base} // soa のメソッド、無ければ要素の struct の値のメソッド
	default:
		return nil
	}
	var m *methodDecl
	for _, t := range cands {
		if m = h.lookupMethod(t, f.name); m != nil {
			break
		}
	}
	if m == nil {
		if h.v4() {
			name := cands[0].Name
			if cands[0].Kind == types.Soa {
				name = cands[1].Name
			}
			panic(&diag.Error{Msg: fmt.Sprintf("struct %s has no field or method %s", name, f.name)})
		}
		return nil
	}
	pt := m.val.Type.Params[0]
	var self *cexpr
	switch {
	case pt.Kind == types.Struct: // 値の self
		switch {
		case xt.Kind == types.Pointer:
			self = cop2(opDeref, x)
		case xt.Kind == types.SoaRef && !h.isSoaElement(x):
			self = cop2(opDeref, x) // ハンドルの指す要素を値として読む
		default:
			self = x // struct の値、soa の要素 (値として読む)
		}
	case pt.Kind == types.Pointer:
		switch xt.Kind {
		case types.Struct:
			if !pt.ReadOnly && !addressable(x) {
				// 書き換えた結果が捨てられる (`f().move(1)`)。読むだけの self:*const T なら一時の値を指してよい
				panic(&diag.Error{Msg: fmt.Sprintf("method %s changes self (self:%s); it cannot be called on a value that is not a variable (store it in a variable first)", m.name(), pt)})
			}
			self = cop2(opRef, x)
		case types.Pointer:
			self = x
		}
	case pt.Kind == types.SoaRef:
		if xt.Kind == types.SoaRef {
			self = x // ハンドル
			if h.isSoaElement(x) {
				self = cop2(opRef, x) // `Enemies[i]` は値として読む式なので、ハンドルは `&Enemies[i]`
			}
		}
	}
	if self == nil {
		panic(&diag.Error{Msg: fmt.Sprintf("method %s takes self:%s; it cannot be called on %s", m.name(), pt, xt)})
	}
	args := append([]*cexpr{cv(m.val), h.constEval(self)}, c.args[1:]...)
	return &cexpr{kind: cOp, op: opCall, args: args, block: c.block, pos: c.pos}
}

// isSoaElement は評価済みの x が soa の要素の式 `S[i]` か (型を決める段の型はハンドルだが、lval は値として読む)。
func (h *Hlc) isSoaElement(x *cexpr) bool {
	if x.kind != cOp || x.op != opIndex {
		return false
	}
	a, ok := h.exprType(x.args[0])
	return ok && a.t.Kind == types.Soa
}

// addressable は評価済みの x が場所 (変数・添字・フィールド・参照はがし) か。呼び出しの結果・リテラル・演算の値は場所でない。
func addressable(x *cexpr) bool {
	switch x.kind {
	case cValue:
		return x.val.Kind == ir.KindGlobal || x.val.Kind == ir.KindLocal
	case cOp:
		return x.op == opIndex || x.op == opField || x.op == opDeref
	}
	return false
}

// methodSym はメソッド T.m の関数のシンボル (_mod_T__m)。
func methodSym(mod, recv, name string) string { return fmt.Sprintf("_%s_%s__%s", mod, recv, name) }
