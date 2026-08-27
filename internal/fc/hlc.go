package fc

// lib/fc/hlc.rb の High-Level コンパイラ (FCソース → 中間コード) の厳密移植。
// const_eval の AST破壊的書き換え、tmp_count の採番順、エラーメッセージ文言まで 1:1 で再現する。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Hlc struct {
	PosInfo *OMap // key: ASTノード(構造的等値), val: []any{filename, line_no}
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
}

// MacroFn は Go組み込みマクロ (Ruby版の defmacro の Proc 相当)。
type MacroFn func(h *Hlc, args []any, block any) any

func NewHlc(libPath []string) *Hlc {
	h := &Hlc{
		PosInfo: NewOMap(),
		Modules: NewOMap(),
		Options: NewOMap(),
		LibPath: libPath,
	}
	h.globalScope = NewScope(nil)
	h.scope = h.globalScope

	h.defmacro(Sym("asm"), func(h *Hlc, args []any, block any) any {
		for _, line := range args {
			h.emit(Sym("asm"), line.(*Value).BaseString)
		}
		return nil
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

// at は Ruby の ast[i] 相当 (範囲外は nil)。
func at(ast []any, i int) any {
	if i < 0 || i >= len(ast) {
		return nil
	}
	return ast[i]
}

func (h *Hlc) updatePos(ast any) {
	if v, ok := h.PosInfo.Get(ast); ok {
		pair := v.([]any)
		h.curFilename = pair[0].(string)
		h.curLineNo = pair[1].(int)
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
	ast, posInfo, perr := ParseSrc(src, path)
	if perr != nil {
		if ce, ok := perr.(*CompileError); ok {
			panic(ce)
		}
		panic(&CompileError{Msg: perr.Error()})
	}
	for _, e := range posInfo.Entries() {
		h.PosInfo.Set(e.Key, e.Val)
	}

	h.attachScope(h.module.Scope, func() {
		h.compileBlock(ast)
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

		ast := deepCopyAST(lmd.Ast) // deep copy ast
		h.compileBlock(ast)

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

func (h *Hlc) compileBlock(ast any) {
	if ast == nil {
		return
	}
	for _, stmt := range ast.([]any) {
		h.compileStatement(stmt)
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

func (h *Hlc) compileStatement(stmt any) {
	h.updatePos(stmt)
	ast := stmt.([]any)

	switch ast[0] {

	case Sym("block"):
		h.compileBlock(at(ast, 1))

	case Sym("blank"):
		// DO NOTHING

	case Sym("options"):
		h.mustInModule()
		for _, e := range at(ast, 1).(*OMap).Entries() {
			var val any
			cv := h.constEval(e.Val).(*Value)
			if cv.BaseString != nil {
				val = cv.BaseString
			} else {
				val = cv.Val
			}
			h.Options.Set(e.Key, val)
			h.module.Options.Set(e.Key, val)
		}

	case Sym("include"):
		h.mustInModule()
		filename := ast[1].(string)
		optIdent := at(ast, 2)
		if optIdent == nil {
			switch filepath.Ext(filename) {
			case ".asm", ".inc":
				optIdent = Sym("asm")
			case ".chr":
				optIdent = Sym("chr")
			case ".rb":
				optIdent = Sym("macro")
			default:
				panic(fmt.Sprintf("unknown include extension %s", filename))
			}
		}
		switch optIdent {
		case Sym("asm"):
			h.module.IncludeAsms = append(h.module.IncludeAsms, filename)
		case Sym("macro"):
			path := h.findModule(filename)
			reg, ok := macroFiles[filename]
			if !ok {
				panic(&CompileError{Msg: fmt.Sprintf("macro file %s is not supported by go port", path)})
			}
			reg(h)
		case Sym("chr"):
			h.module.IncludeChrs = append(h.module.IncludeChrs, h.findModule(filename))
		default:
			panic(&CompileError{Msg: fmt.Sprintf("invalid keyword %s", ToS(optIdent))})
		}
		h.module.Depends = append(h.module.Depends, filename)

	case Sym("use"):
		h.mustInModule()
		id := ast[1].(Sym)
		as := at(ast, 2)
		from := at(ast, 3)
		m := h.compileModule(string(id) + ".fc")
		h.module.Modules.Set(id, m)
		if as != nil && from != nil {
			panic("use with both as and from")
		}
		if s, ok := from.(string); ok && s == "*" {
			h.scope.Use(m.Scope)
		} else {
			if as != nil {
				id = as.(Sym)
			}
			v := h.addVar(NewValue("global", id, TypeOf(Sym("module")), m, NewOMap()))
			v.Public = true
		}

	case Sym("function"):
		pub, id, args, baseType, opt, block := at(ast, 1), at(ast, 2), at(ast, 3), at(ast, 4), at(ast, 5), at(ast, 6)
		lambdaOpt := omap1("id", id)
		if opt != nil {
			for _, e := range opt.(*OMap).Entries() {
				lambdaOpt.Set(e.Key, e.Val)
			}
		}
		h.compileStatement([]any{Sym("const"),
			[]any{[]any{id, nil, []any{Sym("lambda"), []any{Sym("lambda"), args, baseType}, block, lambdaOpt}}},
			pub})

	case Sym("var"):
		for _, vAny := range ast[1].([]any) {
			v := vAny.([]any)
			id, typAst, initAst := at(v, 0), at(v, 1), at(v, 2)
			opt, _ := at(v, 3).(*OMap)
			if opt == nil {
				opt = NewOMap()
			}
			var init any
			if initAst != nil {
				init = h.rval(v[2])
			}
			if init != nil && h.lmd == nil {
				panic(&CompileError{Msg: "can't init global variable"})
			}
			typ := h.typeEval(typAst)
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
			sc := at(ast, 2)
			if sc == nil {
				sc = h.module.CurrentScope
			}
			if sc == Sym("public") {
				vv.Public = true
			}
			if init != nil {
				h.emit(Sym("load"), vv, init)
			}
		}

	case Sym("const"):
		for _, vAny := range ast[1].([]any) {
			v := vAny.([]any)
			id, typAst, valAst := at(v, 0), at(v, 1), at(v, 2)
			opt, _ := at(v, 3).(*OMap)
			var newVal *Value
			if valAst != nil {
				val := h.constEval(valAst).(*Value)
				typ := GuessType(h.typeEval(typAst), val)
				if _, isArr := val.Val.([]any); isArr {
					symbol := h.addDef(id, "block", typ, val.Val)
					newVal = h.addVar(NewValue("global", id, typ, symbol, opt))
				} else {
					newVal = h.addVar(NewValue("literal", id, typ, val.Val, opt))
					if h.lmd == nil {
						h.addDef(id, "equ", typ, val.Val)
					}
				}
			} else {
				if addr, ok := opt.GetOr(Sym("address")).(string); opt != nil && ok {
					typ := h.typeEval(typAst)
					newVal = h.addVar(NewValue("global", id, typ, addr, opt))
				} else {
					panic(&CompileError{Msg: fmt.Sprintf("cannot define const without value %s", ToS(v[0]))})
				}
			}
			sc := at(ast, 2)
			if sc == nil {
				sc = h.module.CurrentScope
			}
			if sc == Sym("public") {
				newVal.Public = true
			}
		}

	case Sym("if"):
		labels := h.newLabels("then", "else", "end")
		thenLabel, elseLabel, endLabel := labels[0], labels[1], labels[2]
		cond := h.rval(at(ast, 1))
		h.emit(Sym("if"), cond, elseLabel)
		h.emit(Sym("label"), thenLabel)
		h.inScope(func() { h.compileStatement(ast[2]) })
		h.emit(Sym("jump"), endLabel)
		h.emit(Sym("label"), elseLabel)
		if at(ast, 3) != nil {
			h.inScope(func() { h.compileStatement(ast[3]) })
		}
		h.emit(Sym("label"), endLabel)

	case Sym("loop"):
		h.inScope(func() {
			labels := h.newLabels("begin", "end")
			h.loops = append(h.loops, labels)
			h.emit(Sym("label"), labels[0])
			h.compileStatement(ast[1])
			h.emit(Sym("jump"), labels[0])
			h.emit(Sym("label"), labels[1])
			h.loops = h.loops[:len(h.loops)-1]
		})

	case Sym("while"):
		h.compileStatement([]any{Sym("loop"), []any{Sym("if"), ast[1], ast[2], []any{Sym("break")}}})

	case Sym("for"):
		h.compileBlock([]any{
			[]any{Sym("exp"), []any{Sym("load"), ast[1], ast[2]}},
			[]any{Sym("while"),
				[]any{Sym("lt"), ast[1], ast[3]},
				[]any{Sym("block"), cons(ast[4].([]any),
					[]any{Sym("exp"), []any{Sym("load"), ast[1], []any{Sym("add"), ast[1], 1}}})},
			}})

	case Sym("break"):
		if len(h.loops) == 0 {
			panic(&CompileError{Msg: "cannot break without loop"})
		}
		h.emit(Sym("jump"), h.loops[len(h.loops)-1][1])

	case Sym("continue"):
		if len(h.loops) == 0 {
			panic(&CompileError{Msg: "cannot break without loop"})
		}
		h.emit(Sym("jump"), h.loops[len(h.loops)-1][0])

	case Sym("return"):
		if h.lmd.Type.Base != TypeOf(Sym("void")) {
			// 非void関数
			if at(ast, 1) == nil {
				panic(&CompileError{Msg: "can't return without value"})
			}
			h.emit(Sym("return"), h.rval(ast[1]))
		} else {
			// void関数
			if at(ast, 1) != nil {
				panic(&CompileError{Msg: "can't return with value from void function"})
			}
			h.emit(Sym("return"))
		}

	case Sym("exp"):
		h.lval(ast[1])

	case Sym("switch"):
		// TODO: jumptableを使った実装をいれる
		condAst, cases, defaultBlock := at(ast, 1), at(ast, 2), at(ast, 3)
		cond := h.rval(condAst)
		tmp := h.newTmp(TypeOf(Sym("int")))
		endLabel := h.newLabel("end")
		for _, cAny := range cases.([]any) {
			c := cAny.([]any)
			labels := h.newLabels("then", "else")
			thenLabel, elseLabel := labels[0], labels[1]
			for _, v := range c[0].([]any) {
				h.emit(Sym("eq"), tmp, cond, h.constEval(v))
				h.emit(Sym("not"), tmp, tmp)
				h.emit(Sym("if"), tmp, thenLabel)
			}
			h.emit(Sym("jump"), elseLabel)
			h.emit(Sym("label"), thenLabel)
			h.compileBlock(c[1])
			h.emit(Sym("jump"), endLabel)
			h.emit(Sym("label"), elseLabel)
		}
		h.compileBlock(defaultBlock)
		h.emit(Sym("label"), endLabel)

		// TODO: 一時的に、public/privateの切り替えを可能にしている。そのうち消すこと
	case Sym("public"):
		h.module.CurrentScope = "public"

	case Sym("private"):
		h.module.CurrentScope = "private"

	default:
		panic(fmt.Sprintf("unknow op %v", SexpStr(stmt)))
	}
}

// ---------------------------------------------------------------
// 定数式の評価
// ---------------------------------------------------------------

// constEval は const_eval 相当。
// *Value もしくは []any(AST) を返す。ASTは破壊的に書き換えられる (Ruby版と同じ)。
func (h *Hlc) constEval(astAny any) any {
	var r any = astAny

	switch ast := astAny.(type) {

	case int:
		r = NewIntValue(ast)

	case Sym:
		r = h.scope.FindMust(ast, true)

	case string:
		// String#unpack('c*') は符号付きバイト
		elems := make([]any, 0, len(ast)+1)
		for i := 0; i < len(ast); i++ {
			elems = append(elems, int(int8(ast[i])))
		}
		elems = append(elems, 0)
		rv := h.constEval([]any{Sym("array"), elems}).(*Value)
		rv.BaseString = ast
		r = rv

	case []any:
		switch ast[0] {
		case Sym("array"):
			vals := make([]any, len(ast[1].([]any)))
			for i, v := range ast[1].([]any) {
				vals[i] = h.constEval(v)
			}
			var typ *Type
			for i, v := range vals {
				if i == 0 {
					typ = ValType(v)
				} else {
					typ = CompatibleType(typ, ValType(v))
				}
			}
			r = NewValue("array_literal", Sym(fmt.Sprintf("$%d", h.tmpCount())), TypeOf([]any{Sym("array"), len(vals), typ}), vals, nil)

		case Sym("incbin"):
			data, err := os.ReadFile(h.findModule(ast[1].(string)))
			if err != nil {
				panic(&CompileError{Msg: err.Error()})
			}
			// unpack('C*') は符号なしバイト
			elems := make([]any, len(data))
			for i, b := range data {
				elems[i] = int(b)
			}
			r = h.constEval([]any{Sym("array"), elems})

		case Sym("lambda"):
			typAst := ast[1].([]any)
			block := at(ast, 2)
			opt, _ := at(ast, 3).(*OMap)
			if opt == nil {
				opt = NewOMap()
			}
			if !eqAny(typAst[0], Sym("lambda")) {
				panic(&CompileError{Msg: "must be lambda type"})
			}
			argAsts := typAst[1].([]any)
			args := make([]any, len(argAsts))
			for i, a := range argAsts {
				pair := a.([]any)
				args[i] = []any{pair[0], TypeOf(pair[1])}
			}
			baseType := TypeOf(typAst[2])
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
			if block == nil {
				opt.Set(Sym("extern"), true)
			}
			lmd := NewLambda(id, args, baseType, opt, block)
			h.module.Lambdas = append(h.module.Lambdas, lmd)
			h.addDefModule(id, "code", lmd.Type, lmd)
			r = NewValue("literal", nil, lmd.Type, id, nil)

		case Sym("dot"):
			left := h.constEval(ast[1]).(*Value)
			mod, ok := left.Val.(*Module)
			if !ok {
				panic("dot: not a module")
			}
			r = mod.Scope.FindMust(ast[2].(Sym), true)

		case Sym("add"), Sym("sub"), Sym("mul"), Sym("div"), Sym("mod"),
			Sym("eq"), Sym("ne"), Sym("lt"), Sym("gt"), Sym("le"), Sym("ge"),
			Sym("rsh"), Sym("lsh"), Sym("and"), Sym("or"), Sym("xor"),
			Sym("land"), Sym("lor"), Sym("not"), Sym("uminus"),
			Sym("shift_left"), Sym("shift_right"):
			ast[1] = h.constEval(ast[1])
			if at(ast, 2) != nil {
				ast[2] = h.constEval(ast[2])
			}
			v1v, ok1 := ast[1].(*Value)
			lit1 := ok1 && v1v.Kind == "literal" && isInt(v1v.Val)
			lit2 := at(ast, 2) == nil
			var v2v *Value
			if !lit2 {
				var ok2 bool
				v2v, ok2 = ast[2].(*Value)
				lit2 = ok2 && v2v.Kind == "literal" && isInt(v2v.Val)
			}
			if lit1 && lit2 {
				v1 := v1v.Val.(int)
				v2 := 0
				if v2v != nil {
					v2 = v2v.Val.(int)
				}
				var n, bn any
				switch ast[0] {
				case Sym("add"):
					n = v1 + v2
				case Sym("sub"):
					n = v1 - v2
				case Sym("mul"):
					n = v1 * v2
				case Sym("div"):
					n = rubyDiv(v1, v2)
				case Sym("mod"):
					n = rubyMod(v1, v2)
				case Sym("eq"):
					bn = v1 == v2
				case Sym("ne"):
					bn = v1 != v2
				case Sym("lt"):
					bn = v1 < v2
				case Sym("gt"):
					bn = v1 > v2
				case Sym("le"):
					bn = v1 <= v2
				case Sym("ge"):
					bn = v1 >= v2
				case Sym("and"):
					n = v1 & v2
				case Sym("or"):
					n = v1 | v2
				case Sym("xor"):
					n = v1 ^ v2
				case Sym("land"):
					bn = v1 != 0 && v2 != 0
				case Sym("lor"):
					bn = v1 != 0 || v2 != 0
				case Sym("not"):
					bn = v1 == 0
				case Sym("uminus"):
					n = -v1
				case Sym("shift_left"):
					n = rubyShl(v1, v2)
				case Sym("shift_right"):
					n = rubyShr(v1, v2)
				default:
					panic("unreachable")
				}
				if bn != nil {
					if bn.(bool) {
						n = 1
					} else {
						n = 0
					}
				}
				r = NewIntValue(n.(int))
			}

		case Sym("call"):
			ast[1] = h.constEval(ast[1])
			oldArgs := ast[2].([]any)
			newArgs := make([]any, len(oldArgs))
			for i, e := range oldArgs {
				newArgs[i] = h.constEval(e)
			}
			ast[2] = newArgs

		case Sym("load"), Sym("index"):
			for i := 1; i < len(ast); i++ {
				ast[i] = h.constEval(ast[i])
			}

		case Sym("cast"):
			ast[1] = h.constEval(ast[1])
			ast[2] = h.typeEval(ast[2])
			if v, ok := ast[1].(*Value); ok && v.Kind == "literal" && isInt(v.Val) {
				r = NewValue("literal", nil, ast[2].(*Type), v.Val, nil)
			}

		case Sym("ref"), Sym("deref"):
			ast[1] = h.constEval(ast[1])

		default:
			panic(fmt.Sprintf("invalid op %s", SexpStr(ast)))
		}
	}
	return r
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

// typeEval は type_eval 相当。
func (h *Hlc) typeEval(astAny any) *Type {
	if ast, ok := astAny.([]any); ok && eqAny(ast[0], Sym("array")) {
		var size any
		if sv := h.constEval(at(ast, 1)); sv != nil {
			size = sv.(*Value).Val
		}
		return TypeOf([]any{Sym("array"), size, ast[2]})
	} else if astAny != nil {
		return TypeOf(astAny)
	}
	return nil
}

// ---------------------------------------------------------------
// 式のコンパイル
// ---------------------------------------------------------------

// rval は右辺値として評価し、値を返す。
func (h *Hlc) rval(ast any) any {
	v, left := h.lval(ast)
	if left {
		r := h.newTmp(ValType(v).Base)
		h.emit(Sym("pget"), r, v)
		return r
	}
	return v
}

// lval は左辺値として評価し、(値, 左辺値かどうか) を返す。
func (h *Hlc) lval(astAny any) (any, bool) {
	leftValue := false
	evaled := h.constEval(astAny)
	var r any

	switch ast := evaled.(type) {

	case *Value:
		if ast.Kind == "array_literal" {
			symbol := h.addDef(Sym(fmt.Sprintf("_%d", h.tmpCount())), "block", ast.Type, ast.Val)
			r = NewValue("global", fmt.Sprintf("$%d", h.tmpCount()), ast.Type, symbol, nil)
		} else {
			r = ast
		}

	case []any:
		switch ast[0] {

		case Sym("load"):
			left, lv := h.lval(ast[1])
			right := h.rval(ast[2])
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

		case Sym("not"), Sym("uminus"):
			left := h.rval(ast[1])
			r = h.newTmp(ValType(left))
			h.emit(ast[0], r, left)

		case Sym("add"), Sym("sub"), Sym("mul"), Sym("div"), Sym("mod"),
			Sym("and"), Sym("or"), Sym("xor"), Sym("shift_left"), Sym("shift_right"):
			left := h.rval(ast[1])
			right := h.rval(ast[2])
			typ, l2, r2, cerr := h.tryMakeCompatible(left, right)
			if cerr != nil {
				if (eqAny(ast[0], Sym("add")) || eqAny(ast[0], Sym("sub"))) &&
					ValType(left).Kind == "pointer" && ValType(right).Kind == "int" {
					typ = ValType(left)
				} else {
					panic(cerr)
				}
			} else {
				left, right = l2, r2
			}
			r = h.newTmp(typ)
			h.emit(ast[0], r, left, right)

		case Sym("eq"), Sym("lt"):
			left := h.rval(ast[1])
			right := h.rval(ast[2])
			_, left, right = h.makeCompatible(left, right)
			r = h.newTmp(TypeOf(Sym("int")))
			h.emit(ast[0], r, left, right)

		case Sym("ne"), Sym("gt"), Sym("le"), Sym("ge"):
			// これらは、eq,lt の引数の順番とnotを組合せて合成する
			left := ast[1]
			right := ast[2]
			switch ast[0] {
			case Sym("ne"):
				r = h.rval([]any{Sym("not"), []any{Sym("eq"), left, right}})
			case Sym("gt"):
				r = h.rval([]any{Sym("lt"), right, left})
			case Sym("le"):
				r = h.rval([]any{Sym("not"), []any{Sym("lt"), right, left}})
			case Sym("ge"):
				r = h.rval([]any{Sym("not"), []any{Sym("lt"), left, right}})
			}

		case Sym("land"):
			endLabel := h.newLabel("end")
			rr := h.newTmp(TypeOf(Sym("int")))
			left := h.rval(ast[1])
			h.emit(Sym("load"), rr, left)
			h.emit(Sym("if"), rr, endLabel)
			right := h.rval(ast[2])
			h.emit(Sym("load"), rr, right)
			h.emit(Sym("label"), endLabel)
			r = rr

		case Sym("lor"):
			endLabel := h.newLabel("end")
			rr := h.newTmp(TypeOf(Sym("int")))
			r2 := h.newTmp(TypeOf(Sym("int")))
			left := h.rval(ast[1])
			h.emit(Sym("load"), rr, left)
			h.emit(Sym("not"), r2, rr)
			h.emit(Sym("if"), r2, endLabel)
			right := h.rval(ast[2])
			h.emit(Sym("load"), rr, right)
			h.emit(Sym("label"), endLabel)
			r = rr

		case Sym("call"):
			lmdV := h.rval(ast[1])
			if ValType(lmdV).Kind == "macro" {
				// マクロの実行
				fn := ValVal(lmdV).(MacroFn)
				x := fn(h, ast[2].([]any), at(ast, 3))
				if x != nil {
					xs := x.([]any)
					if len(xs) > 0 && eqAny(xs[0], Sym("block")) {
						h.compileBlock(xs[1:])
					} else {
						r = h.rval(x)
					}
				}
			} else {
				// 普通の関数コール
				lmdType := ValType(lmdV)
				if lmdType.Base != TypeOf(Sym("void")) {
					r = h.newTmp(lmdType.Base)
				}
				args := ast[2].([]any)
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

		case Sym("ref"): // &演算子
			left, lv := h.lval(ast[1])
			if lv {
				r = left
			} else {
				if !ValAssignable(left) {
					panic(&CompileError{Msg: fmt.Sprintf("%s is not left value", valToS(left))})
				}
				r = h.newTmp(TypeOf([]any{Sym("pointer"), ValType(left)}))
				h.emit(Sym("ref"), r, left)
			}

		case Sym("deref"): // *演算子
			r = h.rval(ast[1])
			if ValType(r).Kind != "pointer" {
				// Ruby版では未代入の `left` を参照するため空文字列になる
				panic(&CompileError{Msg: " is not pointer"})
			}
			leftValue = true

		case Sym("index"): // []演算子
			left := h.rval(ast[1])
			right := h.rval(ast[2])
			if ValType(left).Kind != "pointer" && ValType(left).Kind != "array" {
				panic(&CompileError{Msg: "index must be pointer or array"})
			}
			if ValType(right).Kind != "int" {
				panic(&CompileError{Msg: "index must be int"})
			}
			r = h.newTmp(TypeOf([]any{Sym("pointer"), ValType(left).Base}))
			h.emit(Sym("index"), r, left, right)
			leftValue = true

		case Sym("cast"):
			r = NewCastedValue(h.rval(ast[1]), TypeOf(ast[2]), 0)

		default:
			panic(fmt.Sprintf("unknown op %s", SexpStr(ast)))
		}
	default:
		panic(fmt.Sprintf("unknown op %v", evaled))
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

// deepCopyAST は Marshal.load(Marshal.dump(ast)) 相当のASTディープコピー。
func deepCopyAST(v any) any {
	switch x := v.(type) {
	case []any:
		c := make([]any, len(x))
		for i, e := range x {
			c[i] = deepCopyAST(e)
		}
		return c
	case *OMap:
		c := NewOMap()
		for _, e := range x.Entries() {
			c.Set(deepCopyAST(e.Key), deepCopyAST(e.Val))
		}
		return c
	default:
		return v
	}
}
