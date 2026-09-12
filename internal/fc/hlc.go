package fc

// HLC: 構文木 (internal/syntax) → 中間コード (Lambda.Ops)。lib/fc/hlc.rb 由来。
//
// R1-c (doc/v2_plan.md §4.4) で入力を型付き構文木にし、定数評価を純関数化した
// (構文木は変異しない。評価結果は cexpr に持つ)。tmp_count の採番順・エラーメッセージ文言は
// 旧実装と同一で、生成される IR はバイト単位で一致する。
// IR (Lambda.Ops の [][]any、Sym opcode) はまだ旧形式で、R1-d で型付き化する。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/haramako/fc/internal/syntax"
)

type Hlc struct {
	Modules *OMap // key: Sym(モジュールid), val: *Module
	Options *OMap

	LibPath []string // Fc::LIB_PATH 相当

	curTmpCount int
	globalScope *Scope
	scope       *Scope
	loops       [][]string // [ [continueラベル, breakラベル], ... ]
	fastCalling bool

	module      *Module
	lmd         *Lambda
	curFilename string
	curLineNo   int

	// constEval のメモ。同一の未評価ノードが複数箇所から共有されるとき (`+=` の脱糖)、
	// 2 回目以降は 1 回目の評価結果を返す (旧実装の破壊的評価と同じ挙動)。文ごとにリセットする
	cmemo map[*cexpr]*cexpr
}

// MacroFn は Go 組み込みマクロ (Ruby 版 fclib/*.rb の defmacro 相当)。
// args は評価済みの実引数、block は呼び出しの後置ブロック。
type MacroFn func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult

// macroResult はマクロの展開結果。expr (式として展開) と stmts (式文の列として展開) は排他。
// どちらも nil なら何も展開しない (asm マクロのように副作用のみ)。
type macroResult struct {
	expr  *cexpr
	stmts []*cexpr
}

func NewHlc(libPath []string) *Hlc {
	h := &Hlc{
		Modules: NewOMap(),
		Options: NewOMap(),
		LibPath: libPath,
	}
	h.globalScope = NewScope(nil)
	h.scope = h.globalScope

	h.defmacro(Sym("asm"), func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		for _, line := range args {
			h.emit(Sym("asm"), mustValue(line).BaseString)
		}
		return macroResult{}
	})
	return h
}

// Compile は Hlc#compile 相当。CompileError には filename/line_no を付与する。
func (h *Hlc) Compile(filename string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if ce, ok := r.(*CompileError); ok {
				if ce.Filename == "" {
					ce.Filename = h.curFilename
				}
				if ce.LineNo == 0 {
					ce.LineNo = h.curLineNo
				}
				err = ce
				return
			}
			panic(r)
		}
	}()

	h.compileModule(filename)

	// 関数はあとからコンパイル
	for _, e := range h.Modules.Entries() {
		mod := e.Val.(*Module)
		if mod.FromFcm {
			continue
		}
		h.module = mod
		h.attachScope(mod.Scope, func() {
			// コンパイル中にネストしたlambdaが追加されることがあるため index ループ
			for i := 0; i < len(mod.Lambdas); i++ {
				h.compileLambda(mod.Lambdas[i])
			}
		})
	}
	return nil
}

// ---------------------------------------------------------------
// ユーティリティ
// ---------------------------------------------------------------

// updatePos は現在処理中の文の位置を記録する (CompileError に付与するため)。
// 位置を持たない合成ノード (while/for の脱糖) では更新しない。
func (h *Hlc) updatePos(s syntax.Stmt) {
	if p := s.Pos(); p.IsValid() {
		h.curFilename = h.module.Path
		h.curLineNo = p.Line
	}
}

func (h *Hlc) tmpCount() int {
	h.curTmpCount++
	return h.curTmpCount
}

func (h *Hlc) defmacro(name Sym, fn MacroFn) *Value {
	v := h.addVar(NewValue("global", name, TypeOf(Sym("macro")), fn, NewOMap()))
	v.Public = true
	return v
}

// addVar は変数/定数を追加する。
func (h *Hlc) addVar(v *Value) *Value {
	if h.lmd != nil {
		h.lmd.Vars = append(h.lmd.Vars, v)
	} else if h.module != nil {
		h.module.Vars = append(h.module.Vars, v)
	}
	h.scope.Declare(v)
	return v
}

func defsFind(defs []*Def, symbol any) bool {
	for _, d := range defs {
		if d.Sym == symbol {
			return true
		}
	}
	return false
}

func (h *Hlc) addDef(id any, kind Sym, typ *Type, val any) any {
	var symbol any
	if h.lmd != nil {
		symbol = id
		if !defsFind(h.lmd.Defs, symbol) {
			h.lmd.Defs = append(h.lmd.Defs, &Def{symbol, kind, typ, val})
		}
	} else {
		symbol = Sym(fmt.Sprintf("_%s_%s", ToS(h.module.Id), ToS(id)))
		if !defsFind(h.module.Defs, symbol) {
			h.module.Defs = append(h.module.Defs, &Def{symbol, kind, typ, val})
		}
	}
	return symbol
}

func (h *Hlc) addDefModule(symbol any, kind Sym, typ *Type, val any) {
	if !defsFind(h.module.Defs, symbol) {
		h.module.Defs = append(h.module.Defs, &Def{symbol, kind, typ, val})
	}
}

func (h *Hlc) inScope(f func()) {
	old := h.scope
	h.scope = NewScope(old)
	f()
	h.scope = old
}

func (h *Hlc) attachScope(newScope *Scope, f func()) {
	old := h.scope
	h.scope = newScope
	f()
	h.scope = old
}

// findModule は Fc.find_module 相当。
func (h *Hlc) findModule(file string) string {
	for _, p := range h.LibPath {
		cand := joinRubyPath(p, file)
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	panic(&CompileError{Msg: fmt.Sprintf("file %s not found", file)})
}

// joinRubyPath は Ruby の Pathname#+ 相当 ('.' + f は f になる)。
func joinRubyPath(p, f string) string {
	if p == "." {
		return f
	}
	return p + "/" + f
}

// mustValue は評価済みの値であることを要求する (マクロ引数など)。
func mustValue(c *cexpr) *Value {
	if c.kind != cValue {
		panic(&CompileError{Msg: "constant value required"})
	}
	return c.val
}

// ---------------------------------------------------------------
// モジュールのコンパイル
// ---------------------------------------------------------------

func (h *Hlc) compileModule(filename string) *Module {
	path := h.findModule(filename)
	id := Sym(strings.TrimSuffix(filepath.Base(filename), ".fc"))
	if m, ok := h.Modules.Get(id); ok {
		return m.(*Module)
	}

	oldModule := h.module
	h.module = NewModule(h.globalScope)
	h.module.Path = path

	h.module.Id = id
	h.Modules.Set(id, h.module)
	src, err := ReadSource(path)
	if err != nil {
		panic(&CompileError{Msg: err.Error()})
	}
	file, perr := syntax.Parse(src, path)
	if perr != nil {
		se := perr.(*syntax.Error)
		panic(&CompileError{Msg: se.Msg, Filename: se.Filename, LineNo: se.Pos.Line})
	}

	h.attachScope(h.module.Scope, func() {
		h.compileStmts(file.Stmts)
	})

	newModule := h.module
	h.module = oldModule
	return newModule
}

// ---------------------------------------------------------------
// Lambdaのコンパイル
// ---------------------------------------------------------------

func (h *Hlc) compileLambda(lmd *Lambda) {
	oldLmd := h.lmd
	h.lmd = lmd
	if len(h.loops) != 0 {
		panic("loops not empty")
	}
	h.inScope(func() {
		// 帰り値の追加
		if lmd.Type.Base.Kind != "void" {
			lmd.Result = NewValue("local", Sym("$result"), lmd.Type.Base, nil, omap1("local_type", Sym("result")))
			lmd.Vars = append([]*Value{lmd.Result}, lmd.Vars...)
		}

		// 引数の追加
		for i, a := range lmd.Args {
			pair := a.([]any)
			lmd.Args[i] = h.addVar(NewValue("local", pair[0], pair[1].(*Type), nil, omap1("local_type", Sym("arg"))))
		}

		if lmd.Body != nil {
			h.compileStmts(lmd.Body.Stmts)
		}

		// returnを追加する
		last := []any(nil)
		if len(lmd.Ops) > 0 {
			last = lmd.Ops[len(lmd.Ops)-1]
		}
		if last == nil || !eqAny(last[0], Sym("return")) {
			if lmd.Type.Base == TypeOf(Sym("void")) {
				h.emit(Sym("return"))
			}
		}
	})
	h.lmd = oldLmd
}

// ---------------------------------------------------------------
// 文のコンパイル
// ---------------------------------------------------------------

func (h *Hlc) compileStmts(stmts []syntax.Stmt) {
	for _, s := range stmts {
		h.compileStatement(s)
	}
}

func (h *Hlc) mustInModule() {
	if h.lmd != nil {
		panic("not in module")
	}
}

func eqAny(a, b any) bool { return a == b }

func omap1(k string, v any) *OMap {
	m := NewOMap()
	m.Set(Sym(k), v)
	return m
}

// scopeIsPublic は宣言の可視性を決める (文に public が付いていればそれ、なければモジュールの現在値)。
func (h *Hlc) scopeIsPublic(publicPos syntax.Pos) bool {
	if publicPos.IsValid() {
		return true
	}
	return h.module.CurrentScope == Sym("public")
}

func (h *Hlc) compileStatement(s syntax.Stmt) {
	h.updatePos(s)
	h.cmemo = nil

	switch s := s.(type) {

	case *syntax.Block:
		h.compileStmts(s.Stmts)

	case *syntax.EmptyStmt:
		// DO NOTHING

	case *syntax.OptionsStmt:
		h.mustInModule()
		// 重複キーは後勝ち (位置は最初のもの) なので、一度 OMap にしてから評価する
		raw := NewOMap()
		for _, e := range s.Options.Entries {
			raw.Set(Sym(e.Key.Name), e.Value)
		}
		for _, e := range raw.Entries() {
			var val any
			cv := mustValue(h.constEval(toC(e.Val.(syntax.Expr))))
			if cv.BaseString != nil {
				val = cv.BaseString
			} else {
				val = cv.Val
			}
			h.Options.Set(e.Key, val)
			h.module.Options.Set(e.Key, val)
		}

	case *syntax.IncludeDecl:
		h.mustInModule()
		filename := s.Path.Value
		var kind string
		if s.Kind != nil {
			kind = s.Kind.Name
		} else {
			switch filepath.Ext(filename) {
			case ".asm", ".inc":
				kind = "asm"
			case ".chr":
				kind = "chr"
			case ".rb":
				kind = "macro"
			default:
				panic(fmt.Sprintf("unknown include extension %s", filename))
			}
		}
		switch kind {
		case "asm":
			h.module.IncludeAsms = append(h.module.IncludeAsms, filename)
		case "macro":
			path := h.findModule(filename)
			reg, ok := macroFiles[filename]
			if !ok {
				panic(&CompileError{Msg: fmt.Sprintf("macro file %s is not supported by go port", path)})
			}
			reg(h)
		case "chr":
			h.module.IncludeChrs = append(h.module.IncludeChrs, h.findModule(filename))
		default:
			panic(&CompileError{Msg: fmt.Sprintf("invalid keyword %s", kind)})
		}
		h.module.Depends = append(h.module.Depends, filename)

	case *syntax.UseDecl:
		h.mustInModule()
		id := Sym(s.Module.Name)
		m := h.compileModule(string(id) + ".fc")
		h.module.Modules.Set(id, m)
		if s.FromAll {
			h.scope.Use(m.Scope)
		} else {
			if s.As != nil {
				id = Sym(s.As.Name)
			}
			v := h.addVar(NewValue("global", id, TypeOf(Sym("module")), m, NewOMap()))
			v.Public = true
		}

	case *syntax.FuncDecl:
		// const <name> = <lambda> に脱糖する (旧実装と同じ)
		id := Sym(s.Name.Name)
		lambdaOpt := omap1("id", id)
		if s.Options != nil {
			for _, e := range rawOptions(s.Options).Entries() {
				lambdaOpt.Set(e.Key, e.Val)
			}
		}
		params := make([]lambdaParam, len(s.Params))
		for i, p := range s.Params {
			if p.Type == nil {
				panic(&CompileError{Msg: fmt.Sprintf("parameter %s requires type", p.Name.Name)})
			}
			params[i] = lambdaParam{name: p.Name.Name, typ: p.Type}
		}
		lam := &cexpr{kind: cLambda, lam: &lambdaLit{params: params, result: s.Result, body: s.Body, opt: lambdaOpt}, pos: s.Pos()}
		h.compileConstSpec(id, nil, lam, nil, s.PublicPos)

	case *syntax.VarDecl:
		if s.Const {
			for _, sp := range s.Specs {
				var init *cexpr
				if sp.Init != nil {
					init = toC(sp.Init)
				}
				h.compileConstSpec(Sym(sp.Name.Name), sp.Type, init, rawOptions(sp.Options), s.PublicPos)
			}
		} else {
			for _, sp := range s.Specs {
				h.compileVarSpec(sp, s.PublicPos)
			}
		}

	case *syntax.IfStmt:
		labels := h.newLabels("then", "else", "end")
		thenLabel, elseLabel, endLabel := labels[0], labels[1], labels[2]
		cond := h.rval(toC(s.Cond))
		h.emit(Sym("if"), cond, elseLabel)
		h.emit(Sym("label"), thenLabel)
		h.inScope(func() { h.compileStatement(s.Then) })
		h.emit(Sym("jump"), endLabel)
		h.emit(Sym("label"), elseLabel)
		if s.Else != nil {
			h.inScope(func() { h.compileStatement(s.Else) })
		}
		h.emit(Sym("label"), endLabel)

	case *syntax.LoopStmt:
		h.inScope(func() {
			labels := h.newLabels("begin", "end")
			h.loops = append(h.loops, labels)
			h.emit(Sym("label"), labels[0])
			h.compileStatement(s.Body)
			h.emit(Sym("jump"), labels[0])
			h.emit(Sym("label"), labels[1])
			h.loops = h.loops[:len(h.loops)-1]
		})

	case *syntax.WhileStmt:
		// loop() { if (cond) body else break; }
		h.compileStatement(&syntax.LoopStmt{Body: &syntax.IfStmt{Cond: s.Cond, Then: s.Body, Else: &syntax.BreakStmt{}}})

	case *syntax.ForStmt:
		// var = from; while (var < to) { body...; var = var + 1; }
		body := make([]syntax.Stmt, 0, len(s.Body.Stmts)+1)
		body = append(body, s.Body.Stmts...)
		body = append(body, &syntax.ExprStmt{X: &syntax.AssignExpr{Lhs: s.Var, Op: syntax.Assign,
			Rhs: &syntax.BinaryExpr{X: s.Var, Op: syntax.Plus, Y: &syntax.IntLit{Value: 1, Text: "1"}}}})
		h.compileStmts([]syntax.Stmt{
			&syntax.ExprStmt{X: &syntax.AssignExpr{Lhs: s.Var, Op: syntax.Assign, Rhs: s.From}},
			&syntax.WhileStmt{Cond: &syntax.BinaryExpr{X: s.Var, Op: syntax.Lt, Y: s.To}, Body: &syntax.Block{Stmts: body}},
		})

	case *syntax.BreakStmt:
		if len(h.loops) == 0 {
			panic(&CompileError{Msg: "cannot break without loop"})
		}
		h.emit(Sym("jump"), h.loops[len(h.loops)-1][1])

	case *syntax.ContinueStmt:
		if len(h.loops) == 0 {
			panic(&CompileError{Msg: "cannot break without loop"})
		}
		h.emit(Sym("jump"), h.loops[len(h.loops)-1][0])

	case *syntax.ReturnStmt:
		if h.lmd.Type.Base != TypeOf(Sym("void")) {
			// 非void関数
			if s.Value == nil {
				panic(&CompileError{Msg: "can't return without value"})
			}
			h.emit(Sym("return"), h.rval(toC(s.Value)))
		} else {
			// void関数
			if s.Value != nil {
				panic(&CompileError{Msg: "can't return with value from void function"})
			}
			h.emit(Sym("return"))
		}

	case *syntax.ExprStmt:
		h.lval(toC(s.X))

	case *syntax.SwitchStmt:
		// TODO: jumptableを使った実装をいれる
		cond := h.rval(toC(s.Tag))
		tmp := h.newTmp(TypeOf(Sym("int")))
		endLabel := h.newLabel("end")
		for _, c := range s.Cases {
			labels := h.newLabels("then", "else")
			thenLabel, elseLabel := labels[0], labels[1]
			for _, v := range c.Values {
				h.emit(Sym("eq"), tmp, cond, h.constEvalOperand(toC(v)))
				h.emit(Sym("not"), tmp, tmp)
				h.emit(Sym("if"), tmp, thenLabel)
			}
			h.emit(Sym("jump"), elseLabel)
			h.emit(Sym("label"), thenLabel)
			h.compileStmts(c.Body)
			h.emit(Sym("jump"), endLabel)
			h.emit(Sym("label"), elseLabel)
		}
		if s.Default != nil {
			h.compileStmts(s.Default.Body)
		}
		h.emit(Sym("label"), endLabel)

		// TODO: 一時的に、public/privateの切り替えを可能にしている。そのうち消すこと
	case *syntax.ScopeLabel:
		if s.Public {
			h.module.CurrentScope = "public"
		} else {
			h.module.CurrentScope = "private"
		}

	default:
		panic(fmt.Sprintf("unknown statement %T", s))
	}
}

// compileVarSpec は var 宣言の 1 変数分。
func (h *Hlc) compileVarSpec(sp *syntax.VarSpec, publicPos syntax.Pos) {
	id := Sym(sp.Name.Name)
	opt := rawOptions(sp.Options)
	if opt == nil {
		opt = NewOMap()
	}
	var init any
	if sp.Init != nil {
		init = h.rval(toC(sp.Init))
	}
	if init != nil && h.lmd == nil {
		panic(&CompileError{Msg: "can't init global variable"})
	}
	typ := h.typeEval(sp.Type)
	if typ == nil {
		typ = GuessType(nil, init)
	}
	if typ != nil && init != nil {
		CompatibleType(typ, ValType(init))
	}
	var val any
	if h.lmd == nil {
		if opt.GetOr(Sym("address")) != nil {
			val = h.addDef(id, "equ", typ, opt.GetOr(Sym("address")))
		} else {
			val = h.addDef(id, "bss", typ, omap1("segment", opt.GetOr(Sym("segment"))))
		}
	}
	kind := Sym("global")
	if h.lmd != nil {
		kind = "local"
	}
	vv := h.addVar(NewValue(kind, id, typ, val, opt))
	if h.scopeIsPublic(publicPos) {
		vv.Public = true
	}
	if init != nil {
		h.emit(Sym("load"), vv, init)
	}
}

// compileConstSpec は const 宣言の 1 定数分 (関数宣言の脱糖にも使う)。
// typ / val / opt はそれぞれ省略可 (nil)。
func (h *Hlc) compileConstSpec(id Sym, typ syntax.TypeExpr, val *cexpr, opt *OMap, publicPos syntax.Pos) {
	var newVal *Value
	if val != nil {
		cv := h.constEval(val)
		if cv.kind != cValue {
			panic(&CompileError{Msg: fmt.Sprintf("const %s must be constant", id)})
		}
		v := cv.val
		t := GuessType(h.typeEval(typ), v)
		if _, isArr := v.Val.([]any); isArr {
			symbol := h.addDef(id, "block", t, v.Val)
			newVal = h.addVar(NewValue("global", id, t, symbol, opt))
		} else {
			newVal = h.addVar(NewValue("literal", id, t, v.Val, opt))
			if h.lmd == nil {
				h.addDef(id, "equ", t, v.Val)
			}
		}
	} else {
		addr, addrOk := "", false
		if opt != nil {
			addr, addrOk = opt.GetOr(Sym("address")).(string)
		}
		if addrOk {
			t := h.typeEval(typ)
			newVal = h.addVar(NewValue("global", id, t, addr, opt))
		} else {
			panic(&CompileError{Msg: fmt.Sprintf("cannot define const without value %s", id)})
		}
	}
	if h.scopeIsPublic(publicPos) {
		newVal.Public = true
	}
}

// ---------------------------------------------------------------
// 定数式の評価
// ---------------------------------------------------------------

// constEval は const_eval 相当。未評価の cexpr を受け、評価済みの cexpr を返す
// (値が確定すれば cValue、そうでなければ子が評価済みの演算ノード)。入力は変異しない。
// 評価済みの木に再適用しても結果は変わらない。
func (h *Hlc) constEval(c *cexpr) *cexpr {
	if c.kind == cValue {
		return c
	}
	if h.cmemo == nil {
		h.cmemo = map[*cexpr]*cexpr{}
	}
	if r, ok := h.cmemo[c]; ok {
		return r
	}
	r := h.constEval0(c)
	r.pos = c.pos
	h.cmemo[c] = r
	return r
}

// constEvalOperand は評価結果を IR のオペランド (*Value) として取り出す。
// 旧実装では未確定の演算ノード ([]any) がそのままオペランドになっていた箇所があり
// (switch の case 値)、その場合はダンプ不能で落ちていた。同じく定数を要求する。
func (h *Hlc) constEvalOperand(c *cexpr) any {
	r := h.constEval(c)
	if r.kind != cValue {
		panic(&CompileError{Msg: "constant value required"})
	}
	return r.val
}

func (h *Hlc) constEval0(c *cexpr) *cexpr {
	switch c.kind {

	case cInt:
		return cv(NewIntValue(c.n))

	case cIdent:
		return cv(h.scope.FindMust(Sym(c.name), true))

	case cStr:
		// String#unpack('c*') は符号付きバイト
		elems := make([]*cexpr, 0, len(c.s)+1)
		for i := 0; i < len(c.s); i++ {
			elems = append(elems, cint(int(int8(c.s[i]))))
		}
		elems = append(elems, cint(0))
		rv := h.constEval(carray(elems)).val
		rv.BaseString = c.s
		return cv(rv)

	case cArray:
		vals := make([]any, len(c.args))
		for i, e := range c.args {
			vals[i] = h.constEvalOperand(e)
		}
		var typ *Type
		for i, v := range vals {
			if i == 0 {
				typ = ValType(v)
			} else {
				typ = CompatibleType(typ, ValType(v))
			}
		}
		return cv(NewValue("array_literal", Sym(fmt.Sprintf("$%d", h.tmpCount())), TypeOf([]any{Sym("array"), len(vals), typ}), vals, nil))

	case cIncbin:
		data, err := os.ReadFile(h.findModule(c.s))
		if err != nil {
			panic(&CompileError{Msg: err.Error()})
		}
		// unpack('C*') は符号なしバイト
		elems := make([]*cexpr, len(data))
		for i, b := range data {
			elems[i] = cint(int(b))
		}
		return h.constEval(carray(elems))

	case cLambda:
		lam := c.lam
		opt := lam.opt
		args := make([]any, len(lam.params))
		for i, p := range lam.params {
			args[i] = []any{Sym(p.name), TypeOf(typeAST(p.typ))}
		}
		baseType := TypeOf(typeAST(lam.result))
		var id any
		if s := opt.GetOr(Sym("symbol")); s != nil {
			id = s
		} else if eqAny(opt.GetOr(Sym("id")), Sym("main")) {
			id = Sym("_main")
		} else if opt.GetOr(Sym("id")) != nil {
			id = Sym(fmt.Sprintf("_%s_%s", ToS(h.module.Id), ToS(opt.GetOr(Sym("id")))))
		} else {
			id = Sym(fmt.Sprintf("$%d", h.tmpCount()))
		}
		if lam.body == nil {
			opt.Set(Sym("extern"), true)
		}
		lmd := NewLambda(id, args, baseType, opt, lam.body)
		h.module.Lambdas = append(h.module.Lambdas, lmd)
		h.addDefModule(id, "code", lmd.Type, lmd)
		return cv(NewValue("literal", nil, lmd.Type, id, nil))

	case cDot:
		left := h.constEval(c.args[0])
		var mod *Module
		if left.kind == cValue {
			mod, _ = left.val.Val.(*Module)
		}
		if mod == nil {
			panic("dot: not a module")
		}
		return cv(mod.Scope.FindMust(Sym(c.name), true))

	case cCast:
		x := h.constEval(c.args[0])
		ty := c.ty // 評価済みノードの再評価では型を再計算しない
		if ty == nil {
			ty = h.typeEval(c.typ)
		}
		if x.isLiteralInt() {
			return cv(NewValue("literal", nil, ty, x.val.Val, nil))
		}
		return &cexpr{kind: cCast, args: []*cexpr{x}, typ: c.typ, ty: ty}

	case cOp:
		switch c.op {
		case opAdd, opSub, opMul, opDiv, opMod,
			opEq, opNe, opLt, opGt, opLe, opGe,
			opAnd, opOr, opXor, opLand, opLor, opNot, opUminus,
			opShiftLeft, opShiftRight:
			args := make([]*cexpr, len(c.args))
			args[0] = h.constEval(c.args[0])
			if len(c.args) > 1 {
				args[1] = h.constEval(c.args[1])
			}
			if args[0].isLiteralInt() && (len(args) == 1 || args[1].isLiteralInt()) {
				v1 := args[0].val.Val.(int)
				v2 := 0
				if len(args) > 1 {
					v2 = args[1].val.Val.(int)
				}
				return cv(NewIntValue(foldIntOp(c.op, v1, v2)))
			}
			return &cexpr{kind: cOp, op: c.op, args: args}

		case opCall:
			args := make([]*cexpr, len(c.args))
			for i, a := range c.args {
				args[i] = h.constEval(a)
			}
			return &cexpr{kind: cOp, op: opCall, args: args, block: c.block}

		case opLoad, opIndex, opRef, opDeref:
			args := make([]*cexpr, len(c.args))
			for i, a := range c.args {
				args[i] = h.constEval(a)
			}
			return &cexpr{kind: cOp, op: c.op, args: args}
		}
	}
	panic(fmt.Sprintf("invalid op %v", c.op))
}

// foldIntOp は整数リテラル同士の演算を畳み込む (Ruby の整数演算と真偽値→0/1 に準拠)。
func foldIntOp(op cop, v1, v2 int) int {
	b2i := func(b bool) int {
		if b {
			return 1
		}
		return 0
	}
	switch op {
	case opAdd:
		return v1 + v2
	case opSub:
		return v1 - v2
	case opMul:
		return v1 * v2
	case opDiv:
		return rubyDiv(v1, v2)
	case opMod:
		return rubyMod(v1, v2)
	case opEq:
		return b2i(v1 == v2)
	case opNe:
		return b2i(v1 != v2)
	case opLt:
		return b2i(v1 < v2)
	case opGt:
		return b2i(v1 > v2)
	case opLe:
		return b2i(v1 <= v2)
	case opGe:
		return b2i(v1 >= v2)
	case opAnd:
		return v1 & v2
	case opOr:
		return v1 | v2
	case opXor:
		return v1 ^ v2
	case opLand:
		return b2i(v1 != 0 && v2 != 0)
	case opLor:
		return b2i(v1 != 0 || v2 != 0)
	case opNot:
		return b2i(v1 == 0)
	case opUminus:
		return -v1
	case opShiftLeft:
		return rubyShl(v1, v2)
	case opShiftRight:
		return rubyShr(v1, v2)
	}
	panic("unreachable")
}

func isInt(v any) bool {
	_, ok := v.(int)
	return ok
}

// Ruby の整数演算 (floor除算・floor剰余)
func rubyDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

func rubyMod(a, b int) int {
	m := a % b
	if m != 0 && (m < 0) != (b < 0) {
		m += b
	}
	return m
}

func rubyShl(a, b int) int {
	if b < 0 {
		return rubyShr(a, -b)
	}
	return a << uint(b)
}

func rubyShr(a, b int) int {
	if b < 0 {
		return rubyShl(a, -b)
	}
	return a >> uint(b)
}

// typeEval は type_eval 相当。最外の配列長だけを定数評価する (内側の次元は整数リテラルのみ有効)。
// nil (型省略) なら nil。
func (h *Hlc) typeEval(t syntax.TypeExpr) *Type {
	if t == nil {
		return nil
	}
	if at, ok := t.(*syntax.ArrayType); ok {
		var size any
		if at.Len != nil {
			sv := h.constEval(toC(at.Len))
			if sv.kind != cValue {
				panic(&CompileError{Msg: "array size must be constant"})
			}
			size = sv.val.Val
		}
		return TypeOf([]any{Sym("array"), size, typeAST(at.Elem)})
	}
	return TypeOf(typeAST(t))
}

// ---------------------------------------------------------------
// 式のコンパイル
// ---------------------------------------------------------------

// rval は右辺値として評価し、値を返す。
func (h *Hlc) rval(c *cexpr) any {
	v, left := h.lval(c)
	if left {
		r := h.newTmp(ValType(v).Base)
		h.emit(Sym("pget"), r, v)
		return r
	}
	return v
}

// lval は左辺値として評価し、(値, 左辺値かどうか) を返す。
func (h *Hlc) lval(c *cexpr) (any, bool) {
	leftValue := false
	e := h.constEval(c)
	var r any

	switch e.kind {

	case cValue:
		if e.val.Kind == "array_literal" {
			symbol := h.addDef(Sym(fmt.Sprintf("_%d", h.tmpCount())), "block", e.val.Type, e.val.Val)
			r = NewValue("global", fmt.Sprintf("$%d", h.tmpCount()), e.val.Type, symbol, nil)
		} else {
			r = e.val
		}

	case cCast:
		r = NewCastedValue(h.rval(e.args[0]), e.ty, 0)

	case cOp:
		switch e.op {

		case opLoad:
			left, lv := h.lval(e.args[0])
			right := h.rval(e.args[1])
			if lv {
				CompatibleType(ValType(left).Base, ValType(right))
				right = h.cast(right, ValType(left).Base)
				h.emit(Sym("pset"), left, right)
				r = left
				leftValue = true
			} else {
				CompatibleType(ValType(left), ValType(right))
				if !ValAssignable(left) {
					panic(&CompileError{Msg: fmt.Sprintf("%s is not left value", valToS(left))})
				}
				right = h.cast(right, ValType(left))
				h.emit(Sym("load"), left, right)
				r = left
			}

		case opNot, opUminus:
			left := h.rval(e.args[0])
			r = h.newTmp(ValType(left))
			h.emit(Sym(e.op), r, left)

		case opAdd, opSub, opMul, opDiv, opMod,
			opAnd, opOr, opXor, opShiftLeft, opShiftRight:
			left := h.rval(e.args[0])
			right := h.rval(e.args[1])
			typ, l2, r2, cerr := h.tryMakeCompatible(left, right)
			if cerr != nil {
				if (e.op == opAdd || e.op == opSub) &&
					ValType(left).Kind == "pointer" && ValType(right).Kind == "int" {
					typ = ValType(left)
				} else {
					panic(cerr)
				}
			} else {
				left, right = l2, r2
			}
			r = h.newTmp(typ)
			h.emit(Sym(e.op), r, left, right)

		case opEq, opLt:
			left := h.rval(e.args[0])
			right := h.rval(e.args[1])
			_, left, right = h.makeCompatible(left, right)
			r = h.newTmp(TypeOf(Sym("int")))
			h.emit(Sym(e.op), r, left, right)

		case opNe, opGt, opLe, opGe:
			// これらは、eq,lt の引数の順番とnotを組合せて合成する
			left := e.args[0]
			right := e.args[1]
			switch e.op {
			case opNe:
				r = h.rval(cop2(opNot, cop2(opEq, left, right)))
			case opGt:
				r = h.rval(cop2(opLt, right, left))
			case opLe:
				r = h.rval(cop2(opNot, cop2(opLt, right, left)))
			case opGe:
				r = h.rval(cop2(opNot, cop2(opLt, left, right)))
			}

		case opLand:
			endLabel := h.newLabel("end")
			rr := h.newTmp(TypeOf(Sym("int")))
			left := h.rval(e.args[0])
			h.emit(Sym("load"), rr, left)
			h.emit(Sym("if"), rr, endLabel)
			right := h.rval(e.args[1])
			h.emit(Sym("load"), rr, right)
			h.emit(Sym("label"), endLabel)
			r = rr

		case opLor:
			endLabel := h.newLabel("end")
			rr := h.newTmp(TypeOf(Sym("int")))
			r2 := h.newTmp(TypeOf(Sym("int")))
			left := h.rval(e.args[0])
			h.emit(Sym("load"), rr, left)
			h.emit(Sym("not"), r2, rr)
			h.emit(Sym("if"), r2, endLabel)
			right := h.rval(e.args[1])
			h.emit(Sym("load"), rr, right)
			h.emit(Sym("label"), endLabel)
			r = rr

		case opCall:
			lmdV := h.rval(e.args[0])
			args := e.args[1:]
			if ValType(lmdV).Kind == "macro" {
				// マクロの実行
				fn := ValVal(lmdV).(MacroFn)
				x := fn(h, args, e.block)
				if x.stmts != nil {
					for _, st := range x.stmts {
						h.lval(st)
					}
				} else if x.expr != nil {
					r = h.rval(x.expr)
				}
			} else {
				// 普通の関数コール
				lmdType := ValType(lmdV)
				if lmdType.Base != TypeOf(Sym("void")) {
					r = h.newTmp(lmdType.Base)
				}
				if len(args) != len(lmdType.Args) {
					panic(&CompileError{Msg: fmt.Sprintf("%s has %d but %d", valToS(lmdV), len(lmdType.Args), len(args))})
				}
				if h.lmd.Type.Fastcall() {
					panic(&CompileError{Msg: "cannot call function from fastcall"})
				}

				if lmdType.Fastcall() {
					if h.fastCalling {
						panic(&CompileError{Msg: "cannot fastcall in fastcalling"})
					}
					h.fastCalling = true
					h.emit(Sym("push_fastcall_result"), lmdType.Base)
					for i, arg := range args {
						v := h.rval(arg)
						CompatibleType(lmdType.Args[i], ValType(v))
						v = h.cast(v, lmdType.Args[i])
						h.emit(Sym("push_fastcall_arg"), lmdType.Args[i], v)
					}
					h.emit(Sym("fastcall"), r, lmdV)
					h.fastCalling = false
				} else {
					h.emit(Sym("push_result"), lmdType.Base)
					for i, arg := range args {
						v := h.rval(arg)
						CompatibleType(lmdType.Args[i], ValType(v))
						v = h.cast(v, lmdType.Args[i])
						h.emit(Sym("push_arg"), lmdType.Args[i], v)
					}
					h.emit(Sym("call"), r, lmdV)
				}
			}

		case opRef: // &演算子
			left, lv := h.lval(e.args[0])
			if lv {
				r = left
			} else {
				if !ValAssignable(left) {
					panic(&CompileError{Msg: fmt.Sprintf("%s is not left value", valToS(left))})
				}
				r = h.newTmp(TypeOf([]any{Sym("pointer"), ValType(left)}))
				h.emit(Sym("ref"), r, left)
			}

		case opDeref: // *演算子
			r = h.rval(e.args[0])
			if ValType(r).Kind != "pointer" {
				// Ruby版では未代入の `left` を参照するため空文字列になる
				panic(&CompileError{Msg: " is not pointer"})
			}
			leftValue = true

		case opIndex: // []演算子
			left := h.rval(e.args[0])
			right := h.rval(e.args[1])
			if ValType(left).Kind != "pointer" && ValType(left).Kind != "array" {
				panic(&CompileError{Msg: "index must be pointer or array"})
			}
			if ValType(right).Kind != "int" {
				panic(&CompileError{Msg: "index must be int"})
			}
			r = h.newTmp(TypeOf([]any{Sym("pointer"), ValType(left).Base}))
			h.emit(Sym("index"), r, left, right)
			leftValue = true

		default:
			panic(fmt.Sprintf("unknown op %s", e.op))
		}
	default:
		panic(fmt.Sprintf("unknown expression kind %d", e.kind))
	}
	return r, leftValue
}

func (h *Hlc) emit(op ...any) {
	h.lmd.Ops = append(h.lmd.Ops, op)
}

func (h *Hlc) newLabel(name string) string {
	return h.newLabels(name)[0]
}

func (h *Hlc) newLabels(names ...string) []string {
	r := make([]string, len(names))
	for i, n := range names {
		r[i] = "@" + n + "_" + fmt.Sprintf("%d", h.tmpCount())
	}
	return r
}

func (h *Hlc) newTmp(typ *Type) *Value {
	return h.addVar(NewValue("local", Sym(fmt.Sprintf("$%d", h.tmpCount())), typ, nil, omap1("local_type", Sym("temp"))))
}

// cast は v を type にキャストする (必要ならコードも生成)。
func (h *Hlc) cast(v any, typ *Type) any {
	if typ.Kind == "int" {
		// int の変換
		if typ == ValType(v) {
			return v
		}
		if typ.Size <= ValType(v).Size {
			return v
		}
		if !ValType(v).Signed {
			return v
		}
		if ValKind(v) == "literal" {
			return v
		}
		newV := h.newTmp(typ)
		h.emit(Sym("sign_extension"), newV, v)
		return newV
	} else if typ.Kind == "pointer" && ValType(v).Kind == "array" && ValType(v).Base == typ.Base {
		return NewPointeredArray(v)
	}
	return v
}

// makeCompatible は互換型に変換する (キャストコード生成込み)。
func (h *Hlc) makeCompatible(a, b any) (*Type, any, any) {
	typ := CompatibleType(ValType(a), ValType(b))
	a = h.cast(a, typ)
	b = h.cast(b, typ)
	return typ, a, b
}

// tryMakeCompatible は makeCompatible の CompileError を捕捉するバージョン。
func (h *Hlc) tryMakeCompatible(a, b any) (typ *Type, ra, rb any, err *CompileError) {
	defer func() {
		if r := recover(); r != nil {
			if ce, ok := r.(*CompileError); ok {
				err = ce
				return
			}
			panic(r)
		}
	}()
	typ, ra, rb = h.makeCompatible(a, b)
	return
}
