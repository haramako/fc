package sema

// 宣言 (var / const / struct) の処理と、宣言の置ける場所の検査。

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// declareBad はエラーになった宣言の名前を Bad 型で束縛する (未宣言のまま残すと使う側が全部 "not found" になる)。
func (h *Hlc) declareBad(s syntax.Stmt) {
	bad := func(id *syntax.Ident) {
		if id == nil || h.scope.BoundHere(id.Name) {
			return
		}
		v := h.addVar(ir.NewGlobal(id.Name, h.prog.Types.Bad(), "$bad"))
		switch s := s.(type) {
		case *syntax.VarDecl:
			v.Public = s.PublicPos.IsValid()
		case *syntax.FuncDecl:
			v.Public = s.PublicPos.IsValid()
		case *syntax.UseDecl:
			v.Public = s.PublicPos.IsValid()
		}
	}
	switch s := s.(type) {
	case *syntax.VarDecl:
		for _, sp := range s.Specs {
			bad(sp.Name)
		}
	case *syntax.FuncDecl:
		bad(s.Name)
	case *syntax.StructDecl:
		bad(s.Name)
	case *syntax.SoaDecl:
		bad(s.Name)
	case *syntax.UseDecl:
		if s.As != nil {
			bad(s.As)
		} else if !s.FromAll && len(s.Names) == 0 {
			bad(s.Module)
		}
		for _, n := range s.Names {
			bad(n)
		}
	}
}

func (h *Hlc) mustInModule() {
	if h.lmd != nil {
		panic(&diag.Error{Msg: "must be at module level (not inside a function)"})
	}
}

// scopeIsPublic は宣言の可視性 (`public` が付いていれば公開。既定は private)。
func (h *Hlc) scopeIsPublic(publicPos syntax.Pos) bool {
	return publicPos.IsValid()
}

// optionValueOf は定数評価済みの値を options の値にする (整数 / 文字列 / シンボル)。
func optionValueOf(v *ir.Value) ir.OptionValue {
	switch {
	case v.IsString:
		return ir.OptionValue{Kind: ir.OptStr, Str: v.Str}
	case v.Kind == ir.KindLiteral && v.IsInt:
		return ir.OptionValue{Kind: ir.OptInt, Int: v.Int}
	case v.Symbol != "":
		return ir.OptionValue{Kind: ir.OptIdent, Str: v.Symbol}
	}
	panic(&diag.Error{Msg: "option value must be an integer or string"})
}

// compileVarSpec は var 宣言の 1 変数分。
func (h *Hlc) compileVarSpec(sp *syntax.VarSpec, publicPos syntax.Pos) {
	name := sp.Name.Name
	opt := parseOptions(sp.Options)
	typ := h.typeEval(sp.Type)
	var init ir.Operand
	var initC *cexpr
	var initChecked bool // E・D の判定を評価の前に済ませた (preConvert)
	var initPre bool     // 型の照合を評価の前に済ませた (assignPre)
	if sl, ok := sp.Init.(*syntax.StructLit); ok && sl.Type == nil && typ == nil {
		panic(&diag.Error{Msg: fmt.Sprintf("`%s`: a struct literal without a type name needs the variable's type (write `var %s:T = {…}` or `var %s = T{…}`)", name, name, name)})
	}
	var ginit *ir.Value // モジュールの変数の初期値 (fc 4。起動のときに写す)
	if sp.Init != nil && h.lmd == nil {
		typ, ginit = h.globalInit(name, sp, typ, opt)
	} else if sp.Init != nil {
		c := h.withExpected(toC(sp.Init), typ)
		initC = c
		if sp.Type == nil {
			h.rewriteStringRows(c, sp.Name.End())
		}
		if typ != nil && typ.Kind == types.Array {
			if e := h.constEval(c); e.kind == cValue && e.val.Kind == ir.KindArrayLiteral {
				c = cv(h.fitArrayLiteral(e.val, typ))
			}
		}
		if h.rewriting() && (typ == nil || typ.Kind == types.Array && typ.Length < 0) {
			// fc 3 の `var s = "abc"` は終端の 0 を含めた 4 バイトの配列 (@len も 4)。fc 4 では同じにするため `"abc\0"` に
			if e := h.constEval(c); e.kind == cValue && e.val.IsString && e.val.StrTerm {
				h.rewriteStrTerm(e.val)
			}
		}
		// 型の照合を評価の前に (rvalAssign と同じ。長さを初期値から決める `[?]T` は長さが決まってから下で)
		initPre = typ != nil && !(typ.Kind == types.Array && typ.Length < 0) && h.assignPre("`"+name+"`", c, typ)
		if typ == nil || typ.Kind == types.Array && !typ.IsSoa {
			init = h.arrayValue(c) // `var b = a[1]` (2 次元配列の行) は行の写し (要素へのポインタにしない)
		}
		if init == nil {
			init, initChecked = h.rvalPreConv(c, typ, typ != nil)
		}
		// `var a:[?]u8 = [1, 2, 3];`: 長さを初期値から決める (長さ未定のままフレームに領域が取られず、ほかのローカルを壊していた)
		if it := ir.ValType(init); typ != nil && typ.Kind == types.Array && typ.Length < 0 && it.Kind == types.Array && it.Length >= 0 {
			typ = h.prog.Types.ArrayOf(typ.Base, it.Length)
		}
	}
	if typ != nil {
		h.checkComplete(typ, "variable "+name)
	}
	// 型を省いた変数は、読み取り専用のポインタ・slice で初期化すると読み取り専用 (`var p = &TAB[i]` は *const T)
	inferRO := false
	if typ == nil {
		if init != nil {
			h.checkMixedUse(init, "a variable without a type ("+name+")")
		}
		typ = h.guessType(name, nil, init)
		inferRO = typ != nil && (typ.Kind == types.Pointer || typ.IsSlice()) && h.readOnly(init)
	}
	if typ != nil && init != nil {
		h.assignPost("`"+name+"`", typ, init, initPre)
		if !inferRO {
			h.warnDropConst("`"+name+"`", typ, init)
		}
	}
	var vv *ir.Value
	if h.lmd == nil {
		var symbol string
		var bss *ir.Def
		if addr, ok := opt.Get("address"); ok {
			// 固定番地 (メモリマップド I/O)。asm のシンボルへの束縛は const の options(symbol:) で (§4.1)
			if addr.Kind == ir.OptIdent {
				// 整数の const の名前 (`@(address: ADDR)`。fc.toml で変えられる @(build) の const でもよい: fclib/nes/oam.fc)
				if e := h.constEval(toC(sp.Options.Get("address"))); e.kind == cValue && e.val.Kind == ir.KindLiteral && e.val.IsInt {
					addr = ir.OptionValue{Kind: ir.OptInt, Int: e.val.Int}
				}
			}
			if addr.Kind != ir.OptInt {
				panic(&diag.Error{Msg: fmt.Sprintf("`%s`: options(address:) takes a number; to refer to an assembler symbol, declare a const with options(symbol: \"%s\")", name, addr.Str)})
			}
			symbol = h.addDef(name, &ir.Def{Kind: ir.DefEqu, Type: typ, Equ: ir.NewIntLiteral("", typ, addr.Int),
				AddressVar: h.module.Id + "." + name, Pos: h.curPos})
		} else {
			seg := h.groupBss
			if sv, ok := opt.Get("segment"); ok {
				seg = sv.Text()
				if seg == "" {
					seg = "BSS"
				} // explicit legacy default overrides inherited bss
			}
			d := &ir.Def{Kind: ir.DefBss, Type: typ, Segment: seg, Init: ginit}
			bss = d
			if sym, ok := symbolOption(opt); ok {
				// options(symbol: "name"): fc が確保する領域のシンボル名を固定する (asm から参照するとき)
				d.Sym = sym
				h.addDefModule(d)
				symbol = d.Sym
			} else {
				symbol = h.addDef(name, d)
			}
		}
		st, ro := h.storageType(typ)
		vv = h.addVar(ir.NewGlobal(name, st, symbol))
		vv.ReadOnly = ro || inferRO
		h.prog.storageGlobals[vv] = true
		vv.Volatile = opt.Has("address") || opt.Flag("volatile") // I/O レジスタは読むたび / 書くたびに意味がある
		// fc 4: private で既定の BSS の変数は、出力するコードから参照されなければ領域を取らない (pipeline.markUnusedGlobals)。
		// 置き場所を指定した変数 (@(segment:) / @(bss:)) は並びを当てにしている (整列の詰め物など) かもしれないので残す
		if bss != nil && bss.Segment == "" && h.v4() && !opt.Has("symbol") && !h.scopeIsPublic(publicPos) {
			bss.Droppable = true
		}
	} else {
		st, ro := h.storageType(typ)
		vv = h.addVar(ir.NewLocal(name, st, ir.LTNone))
		vv.ReadOnly = ro || inferRO
	}
	if h.scopeIsPublic(publicPos) {
		vv.Public = true
	}
	if init != nil {
		// 代入 (assign) と同じく宣言の型へ変換する (i8 の値で i16 / u16 を初期化するときの符号拡張。していなくて
		// `var c:i16 = gv;` (gv:i8 = -4) が 252 になっていた。survey 2026-09-27)
		h.emit(&ir.Op{Code: ir.OpLoad, Dst: vv, Src: []ir.Operand{h.convertValue(init, vv.Type, initC, initChecked)}})
	}
}

// globalInit はモジュールの変数の初期値 (fc 4) を定数にする (const の表と同じ規則: 整数・配列・struct・文字列、名前付きの
// const、関数・配列定数・グローバル変数のアドレス)。起動のときに RAM へ写す (codegen の initRecords、runtime の
// fc_global_init)。型を書かなければ初期値の型。fc 3 まではエラー (0 で始まる)。
func (h *Hlc) globalInit(name string, sp *syntax.VarSpec, typ *types.Type, opt ir.Options) (*types.Type, *ir.Value) {
	if !h.v4() {
		panic(&diag.Error{Msg: fmt.Sprintf("can't init global variable %s (globals start as 0; assign it in a function, or use const)", name)})
	}
	if opt.Has("address") {
		panic(&diag.Error{Msg: fmt.Sprintf("`%s`: a variable at a fixed address (@(address:)) cannot have an initial value (it is I/O or memory that fc does not set up; write it in a function)", name)})
	}
	val := toC(sp.Init)
	if sp.Type == nil {
		h.rewriteStringRows(val, sp.Name.End())
	}
	if val.kind == cOp && val.op == opCall {
		// 文字表の変換器 `_T("…")` / `_T('あ')` は定数を作るマクロ (関数の中では lval が展開する)
		if fn := h.constEval(val.args[0]); fn.kind == cValue && h.prog.textmaps[fn.val] != nil {
			val = h.prog.macros[fn.val](h, val.args[1:], nil).expr
		}
	}
	h.constIndex = true
	cv := h.constEval(h.constSlice(h.withExpected(val, typ)))
	h.constIndex = false
	if typ != nil && typ.Kind == types.Pointer {
		cv = h.constAddress(cv, typ) // `var p:*u8 = &g;`、`var p:*const u8 = TABLE;`
	} else if typ == nil {
		cv = h.constRefAddress(cv, nil)
	}
	h.reportNonConst(cv)
	if cv.kind != cValue || cv.val.Kind == ir.KindGlobal && h.prog.constArrays[cv.val] == nil {
		panic(&diag.Error{Msg: fmt.Sprintf("the initial value of global variable %s must be a constant (globals are set when the program starts; assign it in a function)", name)})
	}
	if h.prog.storageAliases[cv.val] != nil || cv.val.Type.Kind == types.Macro {
		panic(&diag.Error{Msg: fmt.Sprintf("the initial value of global variable %s must be a constant", name)})
	}
	if t := typ; t == nil {
		cv = h.namedConstValue(cv, cv.val.Type)
	} else {
		cv = h.namedConstValue(cv, t)
	}
	v := h.fitArrayLiteral(h.padArrayLiteral(name, cv.val, typ), typ)
	if v.Kind == ir.KindArrayLiteral && typ != nil && typ.Kind == types.Array && typ.Base.Kind == types.Pointer {
		v = h.pointerElems(name, v, typ.Base) // `var ps:[2]*u8 = ["ab", &g];` (const の表と同じ)
	}
	checkRaggedLiteral(v)
	t := h.guessType(name, typ, v)
	h.checkConstRange(name, sp.Type, typ, v, t, val)
	if typ != nil && v.Kind != ir.KindArrayLiteral {
		h.warnDropConst("`"+name+"`", typ, v)
	}
	return t, v
}

// reportNonConst は const の初期値の配列・struct リテラル c のうち、定数でない最初の要素の理由 (constant value required /
// storage alias) をその要素の位置で出す (無ければ何もしない。呼んだ側が "must be constant" を出す)。
func (h *Hlc) reportNonConst(c *cexpr) {
	var items []*cexpr
	switch {
	case c.kind == cArray && c.rt:
		items = c.args
	case c.kind == cStructLit:
		for _, f := range c.flds {
			items = append(items, f.val)
		}
	default:
		return
	}
	for _, e := range items {
		func() {
			defer h.enterExpr(e.pos)()
			x := h.constEval(h.constSlice(e))
			if x.kind == cArray || x.kind == cStructLit {
				h.reportNonConst(x)
			}
			h.constEvalOperand(x)
			if !isConstElem(x) {
				if x.val.Kind == ir.KindGlobal && x.val.Name != "" {
					panic(&diag.Error{Msg: fmt.Sprintf("`%s` is a variable; a const needs values known at compile time (its address `&%s` can be used for a pointer)", x.val.Name, x.val.Name)})
				}
				panic(&diag.Error{Msg: "constant value required (got an expression that is evaluated at runtime)"})
			}
		}()
	}
}

// compileConstSpec は const 宣言の 1 定数分 (関数宣言の脱糖にも使う)。
// typ / val / opt はそれぞれ省略可 (nil)。
func (h *Hlc) compileConstSpec(name string, nameEnd syntax.Pos, typ syntax.TypeExpr, val *cexpr, opt ir.Options, publicPos syntax.Pos) {
	var newVal *ir.Value
	if at, ok := typ.(*syntax.ArrayType); ok && at.IsSlice(h.version()) && val == nil {
		panic(&diag.Error{Msg: fmt.Sprintf("const %s: a slice is a run-time value (use [?]T for a constant array)", name)})
	}
	if val != nil {
		declType := h.typeEval(typ)
		if typ == nil {
			h.rewriteStringRows(val, nameEnd)
		}
		h.constIndex = h.v4()
		cv := h.constEval(h.constSlice(h.withExpected(val, declType)))
		h.constIndex = false
		if declType == nil || declType.Kind == types.Pointer {
			cv = h.constRefAddress(cv, declType) // `const PP:*P = &gp;`: グローバル変数のアドレス
		}
		h.reportNonConst(cv)
		if cv.kind != cValue {
			if declType.IsSlice() {
				panic(&diag.Error{Msg: fmt.Sprintf("const %s: a constant slice needs an array constant, an array literal or a string (use [?]T for a constant array)", name)})
			}
			panic(&diag.Error{Msg: fmt.Sprintf("const %s must be constant", name)})
		}
		if h.prog.storageAliases[cv.val] != nil {
			panic(&diag.Error{Msg: fmt.Sprintf("const %s cannot read a storage alias at compile time", name)})
		}
		v := h.fitArrayLiteral(h.padArrayLiteral(name, cv.val, declType), declType)
		if v.Kind == ir.KindArrayLiteral && declType != nil && declType.Kind == types.Array && declType.Base.Kind == types.Pointer {
			// const PS:[N]*T = ["...", other_const, ...]: ポインタの配列。要素の文字列 / 配列リテラルは無名の配列定数に
			// 切り出してそのアドレス、配列定数の名前 (address: で asm のシンボルに束縛したものも) はそのアドレスにする
			// (.word で並ぶ)。型検査は変換した後の値で (要素が名前だけだと変換前は `[N][M]T` で `[N]*T` に合わない)
			v = h.pointerElems(name, v, declType.Base)
		}
		checkRaggedLiteral(v)
		t := h.guessType(name, declType, v)
		h.checkConstRange(name, typ, declType, v, t, val)
		if v.Type.Kind == types.Macro {
			// const T = textmap("..."): マクロ値そのものを名前に束縛する (シンボルは作らない。型指定は guessType で弾かれる)
			v.Name = name
			newVal = h.addVar(v)
		} else if v.Kind == ir.KindArrayLiteral {
			if t.Kind == types.Pointer {
				// const P:*T = [...] / "..." は配列定数の宣言 (ポインタ変数ではない)。データ自体を名前に束縛する
				t = ir.ValType(v)
			}
			d := &ir.Def{Kind: ir.DefBlock, Type: t, Elems: v.Elems}
			var symbol string
			if sym, ok := symbolOption(opt); ok {
				d.Sym = sym // シンボル名を固定 (asm から参照する表など)
				h.addDefModule(d)
				symbol = d.Sym
			} else {
				symbol = h.addDef(name, d)
				// fc 4: モジュールの配列定数も (public でも)、出力するコードから参照されなければ出さない
				// (pipeline.markUnusedGlobals。math を使っても ATAN の表を使わなければ ROM を食わない)
				d.Droppable = h.lmd == nil && h.v4()
			}
			newVal = h.addVar(ir.NewGlobal(name, t, symbol))
			newVal.ReadOnly = true // const の配列は ROM (fc 3 の *const)
			h.prog.constArrays[newVal] = v
			if v.IsString && !explicitLength(typ) {
				h.markStrConst(newVal, typ, nameEnd)
				if h.rewriting() && v.StrTerm {
					// fc 3 の名前付きの文字列定数は終端の 0 を含めた配列 (長さ・添字・ポインタのどれでも 0 が見える)。fc 4 の文字列は
					// 0 終端にしないので、宣言を長さつき (`const NM:[4]u8 = "joe"`: 余りは 0) にしてデータも長さも変えない
					h.rewriteStrConstDecl(newVal)
				}
			}
		} else {
			if opt.Has("symbol") {
				panic(&diag.Error{Msg: fmt.Sprintf("`%s`: options(symbol:) needs an array constant (or no value to refer to an assembler symbol)", name)})
			}
			var lit *ir.Value
			if v.IsInt {
				lit = ir.NewIntLiteral(name, t, v.Int)
				lit.Untyped = v.Untyped && declType == nil // `const N = 200` は型のない定数、`const N:u8 = 200` は型付き
			} else {
				lit = ir.NewSymbolLiteral(name, t, v.Symbol)
				lit.SymOffset = v.SymOffset
			}
			newVal = h.addVar(lit)
			if h.lmd == nil {
				h.addDef(name, &ir.Def{Kind: ir.DefEqu, Type: t, Equ: lit})
			}
		}
	} else {
		// 値なしの const は asm 側の定義の参照: options(symbol: "...") が必須 (fc は `.global` を出す。同じモジュールに
		// include した asm で定義していても、別のオブジェクトファイルでもよい)
		if addr, ok := opt.Get("address"); ok && addr.Kind == ir.OptStr {
			panic(&diag.Error{Msg: fmt.Sprintf("`%s`: options(address: \"...\") is now options(symbol: \"%s\")", name, addr.Str)})
		}
		if sym, ok := symbolOption(opt); ok {
			t := h.typeEval(typ)
			h.addDefModule(&ir.Def{Sym: sym, Kind: ir.DefExtern, Type: t})
			newVal = h.addVar(ir.NewGlobal(name, t, sym))
		} else {
			panic(&diag.Error{Msg: fmt.Sprintf("cannot define const without value %s (a const defined in assembler needs options(symbol: \"...\"))", name)})
		}
	}
	if h.scopeIsPublic(publicPos) {
		newVal.Public = true
	}
}

// ---------------------------------------------------------------
// 定数式の評価
// ---------------------------------------------------------------

// symbolOption は options(symbol: "name") の名前。識別子でなければエラー (空文字列だと codegen が
// シンボル無しの大域変数として落ち、空白などを含むと壊れた asm を出していた。fuzz で発覚)。
func symbolOption(opt ir.Options) (string, bool) {
	v, ok := opt.Get("symbol")
	if !ok {
		return "", false
	}
	sym := v.Text()
	if !asmSymbolRe.MatchString(sym) {
		panic(&diag.Error{Msg: fmt.Sprintf("options(symbol: %q): not a valid assembler symbol name", sym)})
	}
	return sym, true
}

// compileStructDecl は `struct Name { f:T; ... }`。型名を Value (Kind == TypeName, TypeRef) としてスコープに束縛する。
// 名前は先に束縛するので、フィールドから `*Name` で自己参照できる (値としての自己参照は checkComplete が弾く)。
func (h *Hlc) compileStructDecl(s *syntax.StructDecl) {
	h.mustInModule()
	name := s.Name.Name
	st := h.prog.Types.NewStruct(h.module.Id + "." + name)
	if st.Size >= 0 {
		panic(&diag.Error{Msg: fmt.Sprintf("struct %s already defined", name)})
	}
	if h.prog.typeDecls[st] != nil {
		// The identity was registered during collection, but retain Vars order.
		h.module.Vars = append(h.module.Vars, h.prog.typeDecls[st].identity)
	} else {
		tv := h.addVar(ir.NewTypeValue(name, h.prog.Types.TypeName(), st))
		tv.Public = h.scopeIsPublic(s.PublicPos)
	}
	fields := make([]types.Field, 0, len(s.Fields))
	for _, f := range s.Fields {
		fname := f.Name.Name
		for _, prev := range fields {
			if prev.Name == fname {
				panic(&diag.Error{Msg: fmt.Sprintf("field %s already defined in struct %s", fname, name)})
			}
		}
		ft := h.typeEval(f.Type)
		h.checkComplete(ft, "field "+fname)
		if ft.Size < 0 {
			panic(&diag.Error{Msg: fmt.Sprintf("field %s: array field must have a length", fname)})
		}
		fields = append(fields, types.Field{Name: fname, Type: ft})
	}
	h.prog.Types.SetFields(st, fields)
}
