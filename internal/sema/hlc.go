package sema

// HLC: 構文木 (internal/syntax) → 中間コード (ir.Lambda.Ops)。lib/fc/hlc.rb 由来。
//
// 入力は型付き構文木で、定数評価は純関数 (構文木は変異しない。評価結果は cexpr に持つ)。
// tmp_count の採番順・エラーメッセージ文言は旧実装と同一で、生成される IR はバイト単位で一致する。
// プログラム横断の状態と 2 相コンパイルの駆動は program.go。

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"bytes"
	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// Hlc はモジュール単位の意味解析コンテキスト (HLC = High Level Compiler、lib/fc/hlc.rb 由来の名)。
// プログラム横断の状態は prog に、依存モジュールの解決は deps に委ねる。
type Hlc struct {
	prog *Program
	deps Resolver

	scope        *ir.Scope
	loops        []breakable   // 囲んでいるループ/switch (内側が末尾)
	pendingLabel *syntax.Ident // 直前の `L:` ラベル。次に始まるループ/switch が引き取る
	fastCalling  bool

	module *ir.Module
	lmd    *ir.Lambda
	curPos syntax.Position // 処理中の文/式の位置 (CompileError に位置が無いとき補完する)

	// constEval のメモ。同一の未評価ノードが複数箇所から共有されるとき (`+=` の脱糖)、
	// 2 回目以降は 1 回目の評価結果を返す (旧実装の破壊的評価と同じ挙動)。文ごとにリセットする
	cmemo map[*cexpr]*cexpr
}

// breakable は break / continue の飛び先になる文 (ループ、v2 では switch も)。
type breakable struct {
	label         string // 文ラベル (無ければ "")
	isSwitch      bool
	breakLabel    string
	continueLabel string // switch では ""
}

// pushBreakable はループ/switch の開始時に飛び先を登録する (直前の文ラベルがあれば引き取る)。
func (h *Hlc) pushBreakable(b breakable) {
	if h.pendingLabel != nil {
		b.label = h.pendingLabel.Name
		h.pendingLabel = nil
		for _, o := range h.loops {
			if o.label == b.label {
				panic(&diag.Error{Msg: fmt.Sprintf("label %s is already in use", b.label)})
			}
		}
	}
	h.loops = append(h.loops, b)
}

func (h *Hlc) popBreakable() {
	h.loops = h.loops[:len(h.loops)-1]
}

// forHasContinue は for の本体に、この for を対象にする continue があるか
// (ラベルなしで、間にループを挟まないもの。または `continue label` でこの for のラベルを指すもの)。
func forHasContinue(body *syntax.Block, label *syntax.Ident) bool {
	found := false
	var walk func(n syntax.Node, nested bool)
	walk = func(n syntax.Node, nested bool) {
		if found {
			return
		}
		switch n := n.(type) {
		case *syntax.ContinueStmt:
			if n.Label == nil && !nested || n.Label != nil && label != nil && n.Label.Name == label.Name {
				found = true
			}
			return
		case *syntax.LoopStmt, *syntax.WhileStmt, *syntax.ForStmt:
			nested = true
		case *syntax.LambdaExpr:
			return
		}
		for _, c := range syntax.Children(n) {
			walk(c, nested)
		}
	}
	walk(body, false)
	return found
}

// findBreakable は break / continue の飛び先を決める (doc/v2_grammar.md §3.7)。
//   - ラベル付きならそのラベルの文
//   - ラベルなし: break は最も内側のループまたは switch (v1 では switch を積まないのでループのみ)、
//     continue は最も内側のループ
func (h *Hlc) findBreakable(kw string, label *syntax.Ident) breakable {
	if label != nil {
		for i := len(h.loops) - 1; i >= 0; i-- {
			if h.loops[i].label == label.Name {
				return h.loops[i]
			}
		}
		panic(&diag.Error{Msg: fmt.Sprintf("label %s not found for %s", label.Name, kw), Pos: syntax.At(h.module.Path, label.NamePos)})
	}
	for i := len(h.loops) - 1; i >= 0; i-- {
		if kw == "continue" && h.loops[i].isSwitch {
			continue
		}
		return h.loops[i]
	}
	panic(&diag.Error{Msg: fmt.Sprintf("cannot %s without loop", kw)})
}

// MacroFn は組み込みマクロ (printf / unittest_run_tests / textmap など。builtins.go)。
// args は評価済みの実引数、block は呼び出しの後置ブロック。
type MacroFn func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult

// macroResult はマクロの展開結果。expr (式として展開) と stmts (式文の列として展開) は排他。
// どちらも nil なら何も展開しない (asm マクロのように副作用のみ)。
type macroResult struct {
	expr  *cexpr
	stmts []*cexpr
}

// ---------------------------------------------------------------
// ユーティリティ
// ---------------------------------------------------------------

// updatePos は現在処理中の文の位置を記録する (CompileError に付与するため)。
// 位置を持たない合成ノード (while/for の脱糖) では更新しない。
func (h *Hlc) updatePos(s syntax.Stmt) {
	if p := s.Pos(); p.IsValid() {
		h.curPos = syntax.At(h.module.Path, p)
	}
}

// enterExpr は式の評価中だけ現在位置をその式に移す。返り値を defer で呼んで元に戻す。
// 位置を持たない式 (マクロ展開や合成ノード) では何もしない。
// 評価中に位置のない CompileError が panic で通過したら、そのときの位置を付けてから伝搬させる
// (巻き戻し中に位置が親へ戻ってしまい、最終的に文頭しか指せなくなるのを防ぐ)。
func (h *Hlc) enterExpr(pos syntax.Pos) func() {
	if !pos.IsValid() {
		return func() {}
	}
	old := h.curPos
	h.curPos = syntax.At(h.module.Path, pos)
	return func() {
		if r := recover(); r != nil {
			if ce, ok := r.(*diag.Error); ok && !ce.Pos.IsValid() {
				ce.Pos = h.curPos
			}
			h.curPos = old
			panic(r)
		}
		h.curPos = old
	}
}

// tmpCount はモジュール内の連番を進めて返す。
// モジュール単位で閉じているので、モジュールを単独で再コンパイルしても同じ名前になる (C5)。
func (h *Hlc) tmpCount() int {
	h.module.Seq++
	return h.module.Seq
}

func (h *Hlc) tmpName(prefix string) string {
	return fmt.Sprintf("%s%d", prefix, h.tmpCount())
}

func (h *Hlc) defmacro(name string, fn MacroFn) *ir.Value {
	v := h.addVar(ir.NewGlobal(name, h.prog.Types.Macro(), ""))
	v.Public = true
	h.prog.macros[v] = fn
	return v
}

// addVar は変数/定数を追加する。
func (h *Hlc) addVar(v *ir.Value) *ir.Value {
	if h.lmd != nil {
		h.lmd.Vars = append(h.lmd.Vars, v)
	} else if h.module != nil {
		h.module.Vars = append(h.module.Vars, v)
	}
	h.scope.Declare(v)
	return v
}

func defsFind(defs []*ir.Def, symbol string) bool {
	for _, d := range defs {
		if d.Sym == symbol {
			return true
		}
	}
	return false
}

// addDef は現在の関数 (またはモジュール) に定義を追加し、そのシンボル名を返す。
// モジュールレベルでは `_<module>_<name>` にマングルされる。同名が既にあれば追加しない。
func (h *Hlc) addDef(name string, d *ir.Def) string {
	if h.lmd != nil {
		d.Sym = name
		if !defsFind(h.lmd.Defs, d.Sym) {
			h.lmd.Defs = append(h.lmd.Defs, d)
		}
	} else {
		d.Sym = fmt.Sprintf("_%s_%s", h.module.Id, name)
		if !defsFind(h.module.Defs, d.Sym) {
			h.module.Defs = append(h.module.Defs, d)
		}
	}
	return d.Sym
}

// addDefModule はモジュールにシンボル名そのままの定義を追加する (関数用)。
func (h *Hlc) addDefModule(d *ir.Def) {
	if !defsFind(h.module.Defs, d.Sym) {
		h.module.Defs = append(h.module.Defs, d)
	}
}

func (h *Hlc) inScope(f func()) {
	old := h.scope
	h.scope = ir.NewScope(old)
	f()
	h.scope = old
}

func (h *Hlc) attachScope(newScope *ir.Scope, f func()) {
	old := h.scope
	h.scope = newScope
	f()
	h.scope = old
}

// mustValue は評価済みの値であることを要求する (マクロ引数など)。
func mustValue(c *cexpr) *ir.Value {
	if c.kind != cValue {
		panic(&diag.Error{Msg: "constant value required (got an expression that is evaluated at runtime)"})
	}
	return c.val
}

// mustString は文字列リテラル由来の値であることを要求し、その文字列を返す。
func mustString(c *cexpr) string {
	v := mustValue(c)
	if !v.IsString {
		panic(&diag.Error{Msg: fmt.Sprintf("string literal required (got %s)", describe(v))})
	}
	return v.Str
}

// describe はエラーメッセージ用の値の表示 (名前があれば名前、リテラルは値、それ以外は型)。
func describe(v ir.Operand) string {
	if val := ir.UnderlyingValue(v); val != nil {
		switch {
		case val.Name != "" && val.Kind != ir.KindLiteral:
			return "`" + val.Name + "`"
		case val.Name != "":
			return "`" + val.Name + "`"
		case val.Kind == ir.KindLiteral && val.IsInt:
			return fmt.Sprintf("%d", val.Int)
		case val.IsString:
			return fmt.Sprintf("%q", val.Str)
		}
	}
	return "expression of type " + ir.ValType(v).String()
}

// compatible は互換型を返す (なければ CompileError)。
func (h *Hlc) compatible(a, b *types.Type) *types.Type {
	r := h.prog.Types.Compatible(a, b)
	if r == nil {
		panic(&diag.Error{Msg: fmt.Sprintf("types %s and %s are not compatible", a, b)})
	}
	return r
}

// compatibleAssign は代入 (初期化・引数・戻り値も) の型検査。to が代入先。
func (h *Hlc) compatibleAssign(what string, to, from *types.Type) *types.Type {
	r := h.prog.Types.Compatible(to, from)
	if r == nil {
		panic(&diag.Error{Msg: fmt.Sprintf("%s: cannot assign %s to %s (not compatible types)", what, from, to)})
	}
	return r
}

// pointerElems はポインタ配列の const (`[N]*T`) の要素をアドレス (シンボルのリテラル) にする。
// 文字列 / 配列リテラルは無名の配列定数に切り出し、配列定数の名前 (グローバル) はそのシンボル、関数や整数 (null の 0) はそのまま。
func (h *Hlc) pointerElems(name string, arr *ir.Value, ptr *types.Type) *ir.Value {
	elems := make([]ir.Operand, len(arr.Elems))
	for i, e := range arr.Elems {
		ev := ir.ValLiteral(e)
		switch {
		case ev != nil && ev.Kind == ir.KindArrayLiteral:
			if !ev.IsString {
				h.compatibleAssign(fmt.Sprintf("element %d of `%s`", i, name), ptr, ev.Type)
			}
			sym := h.addDef(h.tmpName("_"), &ir.Def{Kind: ir.DefBlock, Type: ev.Type, Elems: ev.Elems})
			elems[i] = ir.NewSymbolLiteral("", ptr, sym)
		case ev != nil && ev.Kind == ir.KindGlobal && ev.Type.Kind == types.Array && ev.Symbol != "":
			h.compatibleAssign(fmt.Sprintf("element %d of `%s`", i, name), ptr, ev.Type)
			elems[i] = ir.NewSymbolLiteral("", ptr, ev.Symbol)
		case ev != nil && ev.Kind == ir.KindLiteral:
			elems[i] = e // 関数のシンボル、整数 (null)
		default:
			panic(&diag.Error{Msg: fmt.Sprintf("element %d of `%s`: constant address required (string, array constant, or function)", i, name)})
		}
	}
	return ir.NewArrayLiteral(arr.Name, h.prog.Types.ArrayOf(ptr, len(elems)), elems)
}

// guessType は宣言型 typ (省略可) と初期値 val から変数の型を決める。
func (h *Hlc) guessType(name string, typ *types.Type, val ir.Operand) *types.Type {
	if typ != nil {
		return h.compatibleAssign("`"+name+"`", typ, ir.ValType(val))
	}
	return ir.ValType(val)
}

// ---------------------------------------------------------------
// 依存モジュール
// ---------------------------------------------------------------

// useModule は `use name` の先の外面を Resolver から得る (相 1 まで処理済み)。
// importer が触れるのは ModuleInterface だけ (C4)。
func (h *Hlc) useModule(name string) *ir.ModuleInterface {
	m, err := h.deps.Module(name)
	if err != nil {
		panic(err)
	}
	return m.Interface()
}

// resolveFile は include / incbin のファイル名を解決する (ref: 生成物に埋め込む参照形、abs: 読み込み用)。
func (h *Hlc) resolveFile(name string) (ref, abs string) {
	ref, abs, err := h.deps.File(name)
	if err != nil {
		panic(err)
	}
	return ref, abs
}

// readFile は検索パス上のファイルを読む (incbin、マクロの外部表など)。
func (h *Hlc) readFile(name string) []byte {
	_, abs := h.resolveFile(name)
	data, err := os.ReadFile(abs)
	if err != nil {
		panic(&diag.Error{Msg: err.Error()})
	}
	return data
}

// ---------------------------------------------------------------
// Lambdaのコンパイル
// ---------------------------------------------------------------

// switchTableMin はジャンプテーブルにする case の数の下限 (比較の連鎖と表の損益分岐点。language_reference.md §5)。
const switchTableMin = 10

func (h *Hlc) compileLambda(lmd *ir.Lambda) {
	oldLmd := h.lmd
	h.lmd = lmd
	if len(h.loops) != 0 {
		panic("loops not empty")
	}
	h.inScope(func() {
		// 帰り値の追加
		if lmd.Type.Base.Kind != types.Void {
			lmd.Result = ir.NewLocal("$result", lmd.Type.Base, ir.LTResult)
			lmd.Vars = append([]*ir.Value{lmd.Result}, lmd.Vars...)
		}

		// 引数の追加
		lmd.Args = make([]*ir.Value, len(lmd.Params))
		for i, p := range lmd.Params {
			lmd.Args[i] = h.addVar(ir.NewLocal(p.Name, p.Type, ir.LTArg))
		}

		if lmd.Body != nil {
			h.compileStmts(lmd.Body.Stmts)
			if lmd.Type.Base.Kind != types.Void && !terminates(lmd.Body) {
				// 終端に落ちると rts が無く次の関数へ流れて暴走する (fc 1 は黙って通していた)
				panic(&diag.Error{Msg: fmt.Sprintf("missing return at end of function %s (returns %s)", lmd.Name, lmd.Type.Base),
					Pos: syntax.Position{Filename: lmd.Pos.Filename, Line: lmd.Body.Rbrace.Line, Col: lmd.Body.Rbrace.Col}})
			}
		}

		// returnを追加する
		var last *ir.Op
		if len(lmd.Ops) > 0 {
			last = lmd.Ops[len(lmd.Ops)-1]
		}
		if last == nil || last.Code != ir.OpReturn {
			if lmd.Type.Base.Kind == types.Void {
				h.emit(&ir.Op{Code: ir.OpReturn})
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
		h.compileStatementRecover(s)
	}
}

// compileStatementRecover は 1 文をコンパイルし、エラーなら記録して次の文へ進めるようにする (複数エラー報告)。
// 途中で抜けた分のスコープ・ループのスタックなどを元に戻し、失敗した宣言の名前は Bad 型で束縛して
// 以降の参照が巻き添えのエラーを出さないようにする。
func (h *Hlc) compileStatementRecover(s syntax.Stmt) {
	scope, loops, pending, fast := h.scope, len(h.loops), h.pendingLabel, h.fastCalling
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		ce, ok := r.(*diag.Error)
		if !ok || ce.Fatal {
			panic(r)
		}
		if !ce.Pos.IsValid() {
			ce.Pos = h.curPos
		}
		h.scope, h.pendingLabel, h.fastCalling = scope, pending, fast
		h.loops = h.loops[:loops]
		h.cmemo = nil
		h.prog.report(ce) // 上限なら Fatal を投げる
		h.declareBad(s)
	}()
	h.compileStatement(s)
}

// declareBad はエラーになった宣言の名前を Bad 型で束縛する (未宣言のまま残すと使う側が全部 "not found" になる)。
func (h *Hlc) declareBad(s syntax.Stmt) {
	bad := func(id *syntax.Ident) {
		if id == nil || h.scope.DeclaredHere(id.Name) {
			return
		}
		h.addVar(ir.NewGlobal(id.Name, h.prog.Types.Bad(), "$bad"))
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
		// 重複キーは後勝ち (位置は最初のもの) なので、一度キー順を確定してから評価する
		type rawOpt struct {
			key string
			val syntax.Expr
		}
		var raws []rawOpt
		for _, e := range s.Options.Entries {
			found := false
			for i := range raws {
				if raws[i].key == e.Key.Name {
					raws[i].val = e.Value
					found = true
				}
			}
			if !found {
				raws = append(raws, rawOpt{e.Key.Name, e.Value})
			}
		}
		for _, r := range raws {
			val := optionValueOf(mustValue(h.constEval(toC(r.val))))
			h.prog.Options.Set(r.key, val)
			h.module.Options.Set(r.key, val)
		}

	case *syntax.StructDecl:
		h.compileStructDecl(s)

	case *syntax.SoaDecl:
		h.compileSoaDecl(s)

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
				panic(&diag.Error{Msg: fmt.Sprintf("unknown include extension %s", filename)})
			}
		}
		switch kind {
		case "asm":
			h.module.IncludeAsms = append(h.module.IncludeAsms, filename)
			// asm が参照するシンボルを控える (fc の関数なら呼び出し規約を Entry に、変数なら volatile に)
			if _, abs, err := h.deps.File(filename); err == nil {
				if data, err := os.ReadFile(abs); err == nil {
					h.module.AsmSymbols = append(h.module.AsmSymbols, reAsmSymbol.FindAllString(string(data), -1)...)
				}
			}
		case "macro":
			panic(&diag.Error{Msg: fmt.Sprintf("include(%q): .rb macros are not supported (printf / unittest_run_tests are built in; use `const T = textmap(\"...\")` for text tables)", filename)})
		case "chr":
			ref, _ := h.resolveFile(filename)
			h.module.IncludeChrs = append(h.module.IncludeChrs, ref)
		default:
			panic(&diag.Error{Msg: fmt.Sprintf("unknown include kind %s (asm / chr)", kind)})
		}
		h.module.Depends = append(h.module.Depends, filename)

	case *syntax.UseDecl:
		h.mustInModule()
		id := s.Module.Name
		m := h.useModule(id)
		h.module.AddUse(m)
		// 再輸出は `public use` のときだけ (doc/v2_grammar.md §3.2)
		reexport := s.PublicPos.IsValid()
		switch {
		case s.FromAll:
			h.scope.Use(m, reexport)
		case len(s.Names) > 0:
			// 選択的インポート (v2): 公開宣言を非修飾名で自スコープに束縛する。
			// 束縛は宣言そのものの Value を共有する (別名ではなく同じ実体)。再輸出は public use のときだけ
			for _, name := range s.Names {
				v := m.LookupPublic(name.Name)
				if v == nil {
					if m.LookupInternal(name.Name) != nil {
						panic(&diag.Error{Msg: fmt.Sprintf("%s.%s is private (declare it with `public` in module %s)", m.Id, name.Name, m.Id), Pos: syntax.At(h.module.Path, name.NamePos)})
					}
					panic(&diag.Error{Msg: fmt.Sprintf("%s not found in module %s", name.Name, m.Id), Pos: syntax.At(h.module.Path, name.NamePos)})
				}
				h.scope.Alias(name.Name, v, reexport)
			}
		default:
			if s.As != nil {
				id = s.As.Name
			}
			v := h.addVar(ir.NewModuleValue(id, h.prog.Types.Module(), m))
			v.Public = reexport
		}

	case *syntax.FuncDecl:
		// const <name> = <lambda> に脱糖する (旧実装と同じ)
		params := make([]lambdaParam, len(s.Params))
		for i, p := range s.Params {
			if p.Type == nil {
				panic(&diag.Error{Msg: fmt.Sprintf("parameter %s requires type", p.Name.Name)})
			}
			params[i] = lambdaParam{name: p.Name.Name, typ: p.Type}
		}
		lam := &cexpr{kind: cLambda, pos: s.Pos(), lam: &lambdaLit{
			name: s.Name.Name, params: params, result: s.Result, body: s.Body, options: parseOptions(s.Options),
		}}
		h.compileConstSpec(s.Name.Name, nil, lam, nil, s.PublicPos)

	case *syntax.VarDecl:
		if s.Const {
			for _, sp := range s.Specs {
				var init *cexpr
				if sp.Init != nil {
					init = toC(sp.Init)
				}
				h.compileConstSpec(sp.Name.Name, sp.Type, init, parseOptions(sp.Options), s.PublicPos)
			}
		} else {
			for _, sp := range s.Specs {
				h.compileVarSpec(sp, s.PublicPos)
			}
		}

	case *syntax.IfStmt:
		labels := h.newLabels("then", "else", "end")
		thenLabel, elseLabel, endLabel := labels[0], labels[1], labels[2]
		h.compileCond(toC(s.Cond), elseLabel, false)
		h.emit(&ir.Op{Code: ir.OpLabel, Label: thenLabel})
		h.inScope(func() { h.compileStatement(s.Then) })
		h.emit(&ir.Op{Code: ir.OpJump, Label: endLabel})
		h.emit(&ir.Op{Code: ir.OpLabel, Label: elseLabel})
		if s.Else != nil {
			h.inScope(func() { h.compileStatement(s.Else) })
		}
		h.emit(&ir.Op{Code: ir.OpLabel, Label: endLabel})

	case *syntax.LoopStmt:
		h.inScope(func() {
			labels := h.newLabels("begin", "end")
			h.pushBreakable(breakable{continueLabel: labels[0], breakLabel: labels[1]})
			h.emit(&ir.Op{Code: ir.OpLabel, Label: labels[0]})
			h.compileStatement(s.Body)
			h.emit(&ir.Op{Code: ir.OpJump, Label: labels[0]})
			h.emit(&ir.Op{Code: ir.OpLabel, Label: labels[1]})
			h.popBreakable()
		})

	case *syntax.LabeledStmt:
		if strings.HasPrefix(s.Label.Name, "@") {
			// コンパイラ内部のラベル (for の step)。IR ラベルを置いて中の文を続ける
			h.emit(&ir.Op{Code: ir.OpLabel, Label: s.Label.Name})
			h.compileStatement(s.Stmt)
			break
		}
		// ラベルは直後のループ/switch が pushBreakable で引き取る (while/for は loop に脱糖されるので、
		// 脱糖で先に出る代入文は触らない)
		h.pendingLabel = s.Label
		h.compileStatement(s.Stmt)
		if h.pendingLabel != nil {
			h.pendingLabel = nil
			panic(&diag.Error{Msg: fmt.Sprintf("label %s must be placed on loop / while / for / switch", s.Label.Name)})
		}

	case *syntax.WhileStmt:
		// loop() { if (cond) body else break; }
		h.compileStatement(&syntax.LoopStmt{Body: &syntax.IfStmt{Cond: s.Cond, Then: s.Body, Else: &syntax.BreakStmt{}}})

	case *syntax.ForStmt:
		if s.IsV1() {
			// v1: var = from; while (var < to) { body...; var = var + 1; }
			// (continue がインクリメントを飛ばす v1 の癖もそのまま)
			body := make([]syntax.Stmt, 0, len(s.Body.Stmts)+1)
			body = append(body, s.Body.Stmts...)
			body = append(body, &syntax.ExprStmt{X: &syntax.AssignExpr{Lhs: s.Var, Op: syntax.Assign,
				Rhs: &syntax.BinaryExpr{X: s.Var, Op: syntax.Plus, Y: &syntax.IntLit{Value: 1, Text: "1"}}}})
			h.compileStmts([]syntax.Stmt{
				&syntax.ExprStmt{X: &syntax.AssignExpr{Lhs: s.Var, Op: syntax.Assign, Rhs: s.From}},
				&syntax.WhileStmt{Cond: &syntax.BinaryExpr{X: s.Var, Op: syntax.Lt, Y: s.To}, Body: &syntax.Block{Stmts: body}},
			})
			break
		}
		// v2: { init; loop { if (cond) { body; step: step; } else break; } }
		// v1 の for (while への脱糖) と同じ IR 形にして、移行しても生成コードが変わらないようにする。
		// continue は step に飛ぶ。step のラベルは continue がこの for を指すときだけ作る
		// (ラベルを常に出すと asm に行が増える)。init の変数は for のスコープに閉じる
		h.inScope(func() {
			if s.Init != nil {
				h.compileStatement(s.Init)
			}
			label := h.pendingLabel // ラベル付き for なら continue L の判定に使う
			stepLabel := ""
			if forHasContinue(s.Body, label) {
				stepLabel = h.newLabel("step")
			}
			labels := h.newLabels("begin", "end")
			h.pushBreakable(breakable{continueLabel: stepLabel, breakLabel: labels[1]})
			h.emit(&ir.Op{Code: ir.OpLabel, Label: labels[0]})
			then := make([]syntax.Stmt, 0, len(s.Body.Stmts)+1)
			then = append(then, s.Body.Stmts...)
			var step syntax.Stmt = &syntax.EmptyStmt{}
			if s.Step != nil {
				step = s.Step
			}
			if stepLabel != "" {
				// 内部ラベル (名前が @ で始まる) は LabeledStmt の特別扱いで IR ラベルになる
				step = &syntax.LabeledStmt{Label: &syntax.Ident{Name: stepLabel}, Stmt: step}
			}
			then = append(then, step)
			var body syntax.Stmt = &syntax.Block{Stmts: then}
			if s.Cond != nil {
				body = &syntax.IfStmt{Cond: s.Cond, Then: body, Else: &syntax.BreakStmt{}}
			}
			h.compileStatement(body)
			h.emit(&ir.Op{Code: ir.OpJump, Label: labels[0]})
			h.emit(&ir.Op{Code: ir.OpLabel, Label: labels[1]})
			h.popBreakable()
		})

	case *syntax.IncDecStmt:
		// x++ → x = x + 1 (v1 の for のインクリメントと同じ形。左辺値は 2 回評価される)
		op := syntax.Plus
		if s.Op == syntax.Dec {
			op = syntax.Minus
		}
		h.compileStatement(&syntax.ExprStmt{X: &syntax.AssignExpr{Lhs: s.X, OpPos: s.OpPos, Op: syntax.Assign,
			Rhs: &syntax.BinaryExpr{X: s.X, OpPos: s.OpPos, Op: op, Y: &syntax.IntLit{ValuePos: s.OpPos, Value: 1, Text: "1"}}}})

	case *syntax.BreakStmt:
		h.emit(&ir.Op{Code: ir.OpJump, Label: h.findBreakable("break", s.Label).breakLabel})

	case *syntax.ContinueStmt:
		b := h.findBreakable("continue", s.Label)
		if b.isSwitch {
			panic(&diag.Error{Msg: fmt.Sprintf("cannot continue a switch (label %s)", b.label)})
		}
		h.emit(&ir.Op{Code: ir.OpJump, Label: b.continueLabel})

	case *syntax.ReturnStmt:
		if h.lmd.Type.Base.Kind != types.Void {
			// 非void関数
			if s.Value == nil {
				panic(&diag.Error{Msg: fmt.Sprintf("can't return without value from %s (returns %s)", h.lmd.Name, h.lmd.Type.Base)})
			}
			rt := h.lmd.Type.Base
			v := h.rval(h.withExpected(toC(s.Value), rt))
			h.compatibleAssign("return from "+h.lmd.Name, rt, ir.ValType(v))
			h.emit(&ir.Op{Code: ir.OpReturn, Src: []ir.Operand{h.cast(v, rt)}})
		} else {
			// void関数
			if s.Value != nil {
				panic(&diag.Error{Msg: fmt.Sprintf("can't return with value from void function %s", h.lmd.Name)})
			}
			h.emit(&ir.Op{Code: ir.OpReturn})
		}

	case *syntax.ExprStmt:
		h.lval(toC(s.X))

	case *syntax.SwitchStmt:
		cond := h.rval(toC(s.Tag))
		endLabel := h.newLabel("end")
		// ラベルなし break は switch を抜ける
		h.pushBreakable(breakable{isSwitch: true, breakLabel: endLabel})
		// case の値を先に評価する (重複の検出と、ジャンプテーブルにするかの判断)
		seen := map[int]bool{} // case の値の重複検出 (先勝ちで黙って通っていた)
		vals := make([][]ir.Operand, len(s.Cases))
		allInt, n, minV, maxV := true, 0, 0, 0
		for ci, c := range s.Cases {
			for _, v := range c.Values {
				cv := h.constEvalOperand(toC(v))
				if k, ok := ir.ValIntLiteral(cv); ok {
					if seen[k] {
						panic(&diag.Error{Msg: fmt.Sprintf("duplicate case value %d", k), Pos: syntax.At(h.module.Path, v.Pos())})
					}
					seen[k] = true
					if n == 0 || k < minV {
						minV = k
					}
					if n == 0 || k > maxV {
						maxV = k
					}
					n++
				} else {
					allInt = false
				}
				vals[ci] = append(vals[ci], cv)
			}
		}
		// ジャンプテーブル (switchTableMin 個以上の整数の case が密に並ぶとき。language_reference.md §5):
		//   switch tag, min, [label...]; jump default; case...: ...; jump end; default: ...; end:
		// 1 バイトのタグだけ (飛び先 - 1 を pha; pha; rts で飛ぶ。比較の連鎖は平均 3 + 5N/2 サイクル、表は約 33 で一定)
		if allInt && n >= switchTableMin && ir.ValType(cond).Size == 1 && maxV-minV+1 <= 2*n && maxV-minV+1 <= 255 && !ir.Disabled("switch") {
			defaultLabel := h.newLabel("default")
			caseLabels := make([]string, len(s.Cases))
			table := make([]string, maxV-minV+1)
			for k := range table {
				table[k] = defaultLabel
			}
			for ci := range s.Cases {
				caseLabels[ci] = h.newLabel("case")
				for _, cv := range vals[ci] {
					k, _ := ir.ValIntLiteral(cv)
					table[k-minV] = caseLabels[ci]
				}
			}
			h.emit(&ir.Op{Code: ir.OpSwitch, Src: []ir.Operand{cond, h.IntValue(minV)}, Labels: table})
			h.emit(&ir.Op{Code: ir.OpJump, Label: defaultLabel})
			for ci, c := range s.Cases {
				h.emit(&ir.Op{Code: ir.OpLabel, Label: caseLabels[ci]})
				h.compileStmts(c.Body)
				h.emit(&ir.Op{Code: ir.OpJump, Label: endLabel})
			}
			h.emit(&ir.Op{Code: ir.OpLabel, Label: defaultLabel})
		} else {
			for ci, c := range s.Cases {
				labels := h.newLabels("then", "else")
				thenLabel, elseLabel := labels[0], labels[1]
				// 値ごとに `eq t; if_true t goto then`、最後の値だけ `eq t; if t goto else` (一致しなければ次の case へ)。
				// t は値ごとに新しい一時変数にする (定義 1 つ + 直後で使用、でコンディションフラグに割り付く: cmp; bne)
				for k, cv := range vals[ci] {
					tmp := h.newTmp(h.prog.Types.IntType(1, false))
					h.emit(&ir.Op{Code: ir.OpEq, Dst: tmp, Src: []ir.Operand{cond, cv}})
					if k < len(vals[ci])-1 {
						h.emit(&ir.Op{Code: ir.OpIfTrue, Src: []ir.Operand{tmp}, Label: thenLabel})
					} else {
						h.emit(&ir.Op{Code: ir.OpIf, Src: []ir.Operand{tmp}, Label: elseLabel})
					}
				}
				if len(vals[ci]) > 1 {
					h.emit(&ir.Op{Code: ir.OpLabel, Label: thenLabel})
				}
				h.compileStmts(c.Body)
				h.emit(&ir.Op{Code: ir.OpJump, Label: endLabel})
				h.emit(&ir.Op{Code: ir.OpLabel, Label: elseLabel})
			}
		}
		if s.Default != nil {
			h.compileStmts(s.Default.Body)
		}
		h.emit(&ir.Op{Code: ir.OpLabel, Label: endLabel})
		h.popBreakable()

	default:
		panic(fmt.Sprintf("unknown statement %T", s))
	}
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
	if sp.Init != nil {
		init = h.rval(h.withExpected(toC(sp.Init), typ))
	}
	if init != nil && h.lmd == nil {
		panic(&diag.Error{Msg: fmt.Sprintf("can't init global variable %s (globals start as 0; assign it in a function, or use const)", name)})
	}
	if typ != nil {
		h.checkComplete(typ, "variable "+name)
	}
	if typ == nil {
		typ = h.guessType(name, nil, init)
	}
	if typ != nil && init != nil {
		h.compatibleAssign("`"+name+"`", typ, ir.ValType(init))
	}
	var vv *ir.Value
	if h.lmd == nil {
		var symbol string
		if addr, ok := opt.Get("address"); ok {
			var equ *ir.Value
			if addr.Kind == ir.OptInt {
				equ = ir.NewIntLiteral("", typ, addr.Int)
			} else {
				equ = ir.NewSymbolLiteral("", typ, addr.Str)
			}
			symbol = h.addDef(name, &ir.Def{Kind: ir.DefEqu, Type: typ, Equ: equ})
		} else {
			seg := ""
			if sv, ok := opt.Get("segment"); ok {
				seg = sv.Text()
			}
			symbol = h.addDef(name, &ir.Def{Kind: ir.DefBss, Type: typ, Segment: seg})
		}
		vv = h.addVar(ir.NewGlobal(name, typ, symbol))
		vv.Volatile = opt.Has("address") || opt.Has("volatile") // I/O レジスタは読むたび / 書くたびに意味がある
	} else {
		vv = h.addVar(ir.NewLocal(name, typ, ir.LTNone))
	}
	if h.scopeIsPublic(publicPos) {
		vv.Public = true
	}
	if init != nil {
		h.emit(&ir.Op{Code: ir.OpLoad, Dst: vv, Src: []ir.Operand{init}})
	}
}

// compileConstSpec は const 宣言の 1 定数分 (関数宣言の脱糖にも使う)。
// typ / val / opt はそれぞれ省略可 (nil)。
func (h *Hlc) compileConstSpec(name string, typ syntax.TypeExpr, val *cexpr, opt ir.Options, publicPos syntax.Pos) {
	var newVal *ir.Value
	if val != nil {
		declType := h.typeEval(typ)
		cv := h.constEval(h.withExpected(val, declType))
		if cv.kind != cValue {
			panic(&diag.Error{Msg: fmt.Sprintf("const %s must be constant", name)})
		}
		v := cv.val
		t := h.guessType(name, declType, v)
		if v.Type.Kind == types.Macro {
			// const T = textmap("..."): マクロ値そのものを名前に束縛する (シンボルは作らない。型指定は guessType で弾かれる)
			v.Name = name
			newVal = h.addVar(v)
		} else if v.Kind == ir.KindArrayLiteral {
			if t.Kind == types.Pointer {
				// const P:*T = [...] / "..." は配列定数の宣言 (ポインタ変数ではない)。データ自体を名前に束縛する
				t = ir.ValType(v)
			}
			if declType != nil && declType.Kind == types.Array && declType.Base.Kind == types.Pointer {
				// const PS:[N]*T = ["...", other_const, ...]: ポインタの配列。要素の文字列 / 配列リテラルは無名の配列定数に
				// 切り出してそのアドレス、配列定数の名前はそのアドレスにする (.word で並ぶ)
				v = h.pointerElems(name, v, declType.Base)
				t = h.guessType(name, declType, v)
			}
			symbol := h.addDef(name, &ir.Def{Kind: ir.DefBlock, Type: t, Elems: v.Elems})
			newVal = h.addVar(ir.NewGlobal(name, t, symbol))
		} else {
			var lit *ir.Value
			if v.IsInt {
				lit = ir.NewIntLiteral(name, t, v.Int)
			} else {
				lit = ir.NewSymbolLiteral(name, t, v.Symbol)
			}
			newVal = h.addVar(lit)
			if h.lmd == nil {
				h.addDef(name, &ir.Def{Kind: ir.DefEqu, Type: t, Equ: lit})
			}
		}
	} else {
		// 値なしの const は address:"..." (文字列) が必須
		if addr, ok := opt.Get("address"); ok && addr.Kind == ir.OptStr {
			newVal = h.addVar(ir.NewGlobal(name, h.typeEval(typ), addr.Str))
		} else {
			panic(&diag.Error{Msg: fmt.Sprintf("cannot define const without value %s", name)})
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
	defer h.enterExpr(c.pos)()
	r := h.constEval0(c)
	r.pos = c.pos
	h.cmemo[c] = r
	return r
}

// constEvalOperand は評価結果を IR のオペランド (*ir.Value) として取り出す。定数でなければ CompileError。
func (h *Hlc) constEvalOperand(c *cexpr) ir.Operand {
	r := h.constEval(c)
	if r.kind != cValue {
		panic(&diag.Error{Msg: "constant value required (got an expression that is evaluated at runtime)"})
	}
	return r.val
}

func (h *Hlc) constEval0(c *cexpr) *cexpr {
	switch c.kind {

	case cInt:
		if c.s == "bool" {
			return cv(ir.NewIntLiteral("", h.prog.Types.Bool(), c.n))
		}
		return cv(h.IntValue(c.n))

	case cNull:
		return &cexpr{kind: cNull} // 型が決まるまで保留 (withExpected)

	case cIdent:
		return cv(h.scope.FindMust(c.name, true))

	case cStr:
		// String#unpack('c*') は符号付きバイト
		elems := make([]*cexpr, 0, len(c.s)+1)
		for i := 0; i < len(c.s); i++ {
			elems = append(elems, cint(int(int8(c.s[i]))))
		}
		elems = append(elems, cint(0))
		rv := h.constEval(carray(elems)).val
		rv.IsString = true
		rv.Str = c.s
		return cv(rv)

	case cArray:
		vals := make([]ir.Operand, len(c.args))
		var typ *types.Type
		for i, e := range c.args {
			if h.needsExpected(e) {
				// 型名を省いた struct リテラルを含む配列: 宣言の型が与えられるまで評価を保留する
				return &cexpr{kind: cArray, args: c.args}
			}
			v := h.constEvalOperand(e)
			vals[i] = v
			if i == 0 {
				typ = ir.ValType(v)
			} else {
				typ = h.compatible(typ, ir.ValType(v))
			}
		}
		return cv(ir.NewArrayLiteral(h.tmpName("$"), h.prog.Types.ArrayOf(typ, len(vals)), vals))

	case cIncbin:
		data := h.readFile(c.s)
		// unpack('C*') は符号なしバイト
		elems := make([]*cexpr, len(data))
		for i, b := range data {
			elems[i] = cint(int(b))
		}
		return h.constEval(carray(elems))

	case cLambda:
		lam := c.lam
		params := make([]ir.Param, len(lam.params))
		for i, p := range lam.params {
			params[i] = ir.Param{Name: p.name, Type: h.typeOf(p.typ)}
		}
		baseType := h.typeOf(lam.result)
		var id string
		if sym, ok := lam.options.Get("symbol"); ok {
			id = sym.Text()
		} else if lam.name == "main" {
			id = "_main"
		} else if lam.name != "" {
			id = fmt.Sprintf("_%s_%s", h.module.Id, lam.name)
		} else {
			// 無名関数。連番がモジュール単位になったので、リンク時の衝突を避けるためモジュール名で修飾する
			id = fmt.Sprintf("_%s_%s", h.module.Id, h.tmpName("$"))
		}
		lmd := h.newLambda(id, lam.name, params, baseType, lam.options, lam.body)
		lmd.Pos = syntax.At(h.module.Path, c.pos)
		h.module.Lambdas = append(h.module.Lambdas, lmd)
		h.addDefModule(&ir.Def{Sym: id, Kind: ir.DefCode, Type: lmd.Type, Lambda: lmd})
		h.prog.lambdas[id] = lmd
		return cv(ir.NewSymbolLiteral("", lmd.Type, id))

	case cDot:
		left := h.constEval(c.args[0])
		if left.kind == cValue && left.val.Type.Kind == types.Bad {
			panic(&diag.Error{Suppressed: true})
		}
		if left.kind == cValue && left.val.Module != nil {
			return cv(left.val.Module.LookupMust(c.name))
		}
		// モジュールでなければ struct のフィールド参照 (実行時に評価する)
		return &cexpr{kind: cOp, op: opField, args: []*cexpr{left}, name: c.name}

	case cStructLit:
		return h.constEvalStructLit(c)

	case cSizeof:
		return cv(h.IntValue(h.sizeofType(c.typ)))

	case cCast:
		x := h.constEval(c.args[0])
		ty := c.ty // 評価済みノードの再評価では型を再計算しない
		if ty == nil {
			ty = h.typeEval(c.typ)
		}
		if x.kind == cNull {
			return cv(h.nullOf(ty).(*ir.Value)) // `null as *T`
		}
		if x.isLiteralInt() {
			// 整数リテラルはサイズを持たないので、bitcast のサイズ検査はしない (`bitcast<*int>(0x2000)`, `bitcast<fn():int>(0)`)
			h.recordCast(c, x.val.Type, ty)
			if c.ck == syntax.CastAs {
				h.checkCast(c.ck, x.val.Type, ty)
			}
			return cv(ir.NewIntLiteral("", ty, x.val.Int))
		}
		return &cexpr{kind: cCast, args: []*cexpr{x}, typ: c.typ, ty: ty, ck: c.ck, pos: c.pos}

	case cOp:
		switch c.op {
		case opAdd, opSub, opMul, opDiv, opMod,
			opEq, opNe, opLt, opGt, opLe, opGe,
			opAnd, opOr, opXor, opLand, opLor, opNot, opUminus, opBitNot,
			opShiftLeft, opShiftRight:
			args := make([]*cexpr, len(c.args))
			args[0] = h.constEval(c.args[0])
			if len(c.args) > 1 {
				args[1] = h.constEval(c.args[1])
			}
			if args[0].isLiteralInt() && (len(args) == 1 || args[1].isLiteralInt()) {
				v1 := args[0].val.Int
				v2 := 0
				if len(args) > 1 {
					v2 = args[1].val.Int
				}
				n := foldIntOp(c.op, v1, v2)
				switch c.op {
				case opEq, opNe, opLt, opGt, opLe, opGe, opLand, opLor, opNot:
					return cv(ir.NewIntLiteral("", h.prog.Types.Bool(), n)) // 比較・論理演算の定数畳み込みも bool
				}
				return cv(h.IntValue(n))
			}
			return &cexpr{kind: cOp, op: c.op, args: args}

		case opCall:
			args := make([]*cexpr, len(c.args))
			for i, a := range c.args {
				args[i] = h.constEval(a)
			}
			// 定数式で評価する組み込み (textmap など) はここで展開する
			if args[0].kind == cValue && args[0].val.Type.Kind == types.Macro {
				if fn, ok := h.prog.constMacros[args[0].val]; ok {
					return fn(h, args[1:])
				}
			}
			return &cexpr{kind: cOp, op: opCall, args: args, block: c.block}

		case opField:
			return &cexpr{kind: cOp, op: opField, args: []*cexpr{h.constEval(c.args[0])}, name: c.name}

		case opMin, opMax, opClamp:
			args := make([]*cexpr, len(c.args))
			allLit := true
			for i, a := range c.args {
				args[i] = h.constEval(a)
				allLit = allLit && args[i].isLiteralInt()
			}
			if allLit {
				v := args[0].val.Int
				switch c.op {
				case opMin:
					v = min(v, args[1].val.Int)
				case opMax:
					v = max(v, args[1].val.Int)
				case opClamp:
					v = min(max(v, args[1].val.Int), args[2].val.Int)
				}
				return cv(h.IntValue(v))
			}
			return &cexpr{kind: cOp, op: c.op, args: args}

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

// IntValue は値から型を推定した整数リテラル: 0〜255 → uint8、256 以上 → uint16、-128〜-1 → sint8、-129 以下 → sint16。
func (h *Hlc) IntValue(n int) *ir.Value {
	var t *types.Type
	switch {
	case n >= 256:
		t = h.prog.Types.IntType(2, false)
	case n < -128:
		t = h.prog.Types.IntType(2, true)
	case n < 0:
		t = h.prog.Types.IntType(1, true)
	default:
		t = h.prog.Types.IntType(1, false)
	}
	return ir.NewIntLiteral("", t, n)
}

// newLambda は ir.Lambda を作る。型は params / baseType / options(fastcall) から決まる。
func (h *Hlc) newLambda(id, name string, params []ir.Param, baseType *types.Type, opts ir.Options, body *syntax.Block) *ir.Lambda {
	argTypes := make([]*types.Type, len(params))
	for i, p := range params {
		argTypes[i] = p.Type
	}
	typ := h.prog.Types.Func(argTypes, baseType, opts.Has("fastcall"))
	return &ir.Lambda{Id: id, Name: name, Params: params, Type: typ, Options: opts, Module: h.module, Extern: body == nil, Body: body}
}

// foldIntOp は整数リテラル同士の演算を畳み込む (除算・剰余は床除算、比較・論理演算の結果は 0/1。実行時の演算と同じ規則)。
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
		return ir.FloorDiv(v1, v2)
	case opMod:
		return ir.FloorMod(v1, v2)
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
	case opBitNot:
		return ^v1
	case opUminus:
		return -v1
	case opShiftLeft, opShiftRight:
		if v2 < 0 {
			panic(&diag.Error{Msg: fmt.Sprintf("shift count %d is negative", v2)})
		}
		if op == opShiftLeft {
			return ir.Shl(v1, v2)
		}
		return ir.Shr(v1, v2)
	}
	panic("unreachable")
}

// ---------------------------------------------------------------
// 型式の評価
// ---------------------------------------------------------------

// typeOf は型式を型にする (配列長は整数リテラルのときだけ有効。定数式は長さ省略扱い)。
// これは旧実装 (評価前の AST を受け取る Type[]) の挙動で、最外の配列長を定数評価するのは typeEval。
func (h *Hlc) typeOf(t syntax.TypeExpr) *types.Type {
	switch t := t.(type) {
	case *syntax.NamedType:
		return h.namedType(t)
	case *syntax.PointerType:
		elem := h.typeOf(t.Elem)
		if elem.IsSoa {
			return h.prog.Types.SoaRef(elem, elem.Base, "") // `*Points`: SoA の要素ハンドル
		}
		return h.prog.Types.PointerTo(elem)
	case *syntax.ArrayType:
		n := -1
		if lit, ok := t.Len.(*syntax.IntLit); ok {
			n = lit.Value
		}
		return h.prog.Types.ArrayOf(h.typeOf(t.Elem), n)
	case *syntax.FuncType:
		params := make([]*types.Type, len(t.Params))
		for i, p := range t.Params {
			if p.Name != nil {
				panic(&diag.Error{Msg: "named parameter is not allowed in function type"})
			}
			params[i] = h.typeOf(p.Type)
		}
		return h.prog.Types.Func(params, h.typeOf(t.Result), false)
	}
	panic(fmt.Sprintf("typeOf: unknown type expression %T", t))
}

// namedType は型名 (基本型、または struct / soa 宣言の名前。`mod.Name` は他モジュールの公開型) を型にする。
func (h *Hlc) namedType(t *syntax.NamedType) *types.Type {
	name := t.Name.Name
	if t.Module == nil {
		if ty, ok := h.prog.Types.Named(name); ok {
			return ty
		}
		if v := h.scope.Find(name, true); v != nil {
			if v.TypeRef != nil {
				return v.TypeRef
			}
			if v.Type.Kind == types.Bad {
				return v.Type // エラーになった struct / soa 宣言
			}
		}
		panic(&diag.Error{Msg: fmt.Sprintf("unknown type %s", name)})
	}
	mv := h.scope.FindMust(t.Module.Name, true)
	if mv.Module == nil {
		panic(&diag.Error{Msg: fmt.Sprintf("%s is not a module (in type %s.%s)", t.Module.Name, t.Module.Name, name)})
	}
	v := mv.Module.LookupMust(name)
	if v.TypeRef == nil {
		panic(&diag.Error{Msg: fmt.Sprintf("%s.%s is not a type", t.Module.Name, name)})
	}
	return v.TypeRef
}

// checkComplete は変数・フィールド・配列要素に置ける型か検査する (void と未完成の struct は不可)。
func (h *Hlc) checkComplete(t *types.Type, what string) {
	switch {
	case t.Kind == types.Void:
		panic(&diag.Error{Msg: fmt.Sprintf("%s cannot be void", what)})
	case t.Kind == types.Struct && t.Size < 0:
		panic(&diag.Error{Msg: fmt.Sprintf("%s: struct %s is not complete yet (recursive struct must go through a pointer)", what, t.Name)})
	case t.Kind == types.Array && t.Base.Kind == types.Struct && t.Base.Size < 0:
		panic(&diag.Error{Msg: fmt.Sprintf("%s: struct %s is not complete yet (recursive struct must go through a pointer)", what, t.Base.Name)})
	}
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
	tv := h.addVar(ir.NewTypeValue(name, h.prog.Types.TypeName(), st))
	if h.scopeIsPublic(s.PublicPos) {
		tv.Public = true
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

// typeEval は type_eval 相当。最外の配列長だけを定数評価する (内側の次元は整数リテラルのみ有効)。
// nil (型省略) なら nil。
func (h *Hlc) typeEval(t syntax.TypeExpr) *types.Type {
	if t == nil {
		return nil
	}
	if at, ok := t.(*syntax.ArrayType); ok {
		n := -1
		if at.Len != nil {
			sv := h.constEval(toC(at.Len))
			if sv.kind != cValue {
				panic(&diag.Error{Msg: "array size must be constant"})
			}
			// 整数リテラル以外 (変数など) は長さ省略扱い (旧実装と同じ)
			if sv.val.Kind == ir.KindLiteral && sv.val.IsInt {
				n = sv.val.Int
			}
		}
		return h.prog.Types.ArrayOf(h.typeOf(at.Elem), n)
	}
	return h.typeOf(t)
}

// ---------------------------------------------------------------
// 式のコンパイル
// ---------------------------------------------------------------

// rval は右辺値として評価し、値を返す。
func (h *Hlc) rval(c *cexpr) ir.Operand {
	v, left := h.lval(c)
	if v == nil {
		// void 関数の呼び出しなど値を持たない式を、値が要る場所 (条件・代入・引数) に書いた
		panic(&diag.Error{Msg: "expression has no value (void function call used as a value)"})
	}
	if left {
		if ir.ValType(v).Kind == types.SoaRef {
			return h.soaGather(v)
		}
		r := h.newTmp(ir.ValType(v).Base)
		h.emit(&ir.Op{Code: ir.OpPget, Dst: r, Src: []ir.Operand{v}})
		return r
	}
	return v
}

// compileCond は条件文脈 (if / while の条件、値として使う && / ||) の式を分岐に落とす:
// 式が真 (jumpIfTrue) / 偽 (!jumpIfTrue) なら label へ飛ぶ。
//
// `!` は飛ぶ向きの反転、`&&` / `||` は短絡の分岐、`!=` / `<=` / `>=` は `==` / `<` の反転にして、
// 0 / 1 の値をメモリに作らない。比較の一時変数は定義の直後で使うのでコンディションフラグに割り付く
// (regalloc.allocateCond) → `cmp; bne L`。定数の条件は jump か何も出さない。
func (h *Hlc) compileCond(c *cexpr, label string, jumpIfTrue bool) {
	defer h.enterExpr(c.pos)()
	e := h.constEval(c)
	if e.kind == cOp {
		switch e.op {
		case opNot:
			h.compileCond(e.args[0], label, !jumpIfTrue)
			return
		case opLand:
			if jumpIfTrue {
				skip := h.newLabel("skip")
				h.compileCond(e.args[0], skip, false)
				h.compileCond(e.args[1], label, true)
				h.emit(&ir.Op{Code: ir.OpLabel, Label: skip})
			} else {
				h.compileCond(e.args[0], label, false)
				h.compileCond(e.args[1], label, false)
			}
			return
		case opLor:
			if jumpIfTrue {
				h.compileCond(e.args[0], label, true)
				h.compileCond(e.args[1], label, true)
			} else {
				skip := h.newLabel("skip")
				h.compileCond(e.args[0], skip, true)
				h.compileCond(e.args[1], label, false)
				h.emit(&ir.Op{Code: ir.OpLabel, Label: skip})
			}
			return
		case opNe:
			h.compileCond(cop2(opEq, e.args[0], e.args[1]), label, !jumpIfTrue)
			return
		case opLe:
			h.compileCond(cop2(opLt, e.args[1], e.args[0]), label, !jumpIfTrue)
			return
		case opGe:
			h.compileCond(cop2(opLt, e.args[0], e.args[1]), label, !jumpIfTrue)
			return
		}
	}
	v := h.rval(e)
	if n, ok := ir.ValIntLiteral(v); ok {
		if (n != 0) == jumpIfTrue {
			h.emit(&ir.Op{Code: ir.OpJump, Label: label})
		}
		return
	}
	code := ir.OpIf
	if jumpIfTrue {
		code = ir.OpIfTrue
	}
	h.emit(&ir.Op{Code: code, Src: []ir.Operand{v}, Label: label})
}

// lval は左辺値として評価し、(値, 左辺値かどうか) を返す。
func (h *Hlc) lval(c *cexpr) (ir.Operand, bool) {
	defer h.enterExpr(c.pos)()
	leftValue := false
	e := h.constEval(c)
	var r ir.Operand

	switch e.kind {

	case cValue:
		if e.val.Type.Kind == types.Bad {
			panic(&diag.Error{Suppressed: true}) // エラーになった宣言の参照: 報告済みなので黙って打ち切る
		}
		if e.val.Type.Kind == types.TypeName {
			panic(&diag.Error{Msg: fmt.Sprintf("%s is a type, not a value", e.val.Name)})
		}
		if e.val.Type.IsSoa {
			panic(&diag.Error{Msg: fmt.Sprintf("soa %s can only be indexed (%s[i]) or used as a type (*%s)", e.val.Name, e.val.Name, e.val.Name)})
		}
		if e.val.Kind == ir.KindArrayLiteral {
			symbol := h.addDef(h.tmpName("_"), &ir.Def{Kind: ir.DefBlock, Type: e.val.Type, Elems: e.val.Elems})
			r = ir.NewGlobal(h.tmpName("$"), e.val.Type, symbol)
		} else {
			r = e.val
		}

	case cCast:
		v := h.rval(e.args[0])
		h.recordCast(e, ir.ValType(v), e.ty)
		r = h.explicitCast(e.ck, v, e.ty)

	case cStructLit:
		// 実行時に組み立てる struct リテラル: 一時変数 (フレーム上) にフィールドごとに代入する
		if e.ty == nil {
			panic(&diag.Error{Msg: "struct literal without a type name needs a context that gives the type (declared type or assignment)"})
		}
		tmp := h.newTmp(e.ty)
		for _, f := range e.ty.Fields {
			dst := ir.NewCastedValue(tmp, f.Type, f.Offset)
			var v ir.Operand
			if fv := structLitField(e, f.Name); fv != nil {
				v = h.rval(fv)
				h.compatible(f.Type, ir.ValType(v))
				v = h.cast(v, f.Type)
			} else {
				v = h.zeroValue(f.Type)
			}
			h.emit(&ir.Op{Code: ir.OpLoad, Dst: dst, Src: []ir.Operand{v}})
		}
		r = tmp

	case cArray:
		panic(&diag.Error{Msg: "array literal with untyped struct literals needs a declared type"})

	case cNull:
		panic(&diag.Error{Msg: "null needs a context that gives the pointer type (assignment, comparison, argument, or `null as *T`)"})

	case cOp:
		switch e.op {

		case opLoad:
			if rhs := e.args[1]; rhs.kind == cOp && len(rhs.args) == 2 && rhs.args[0] == e.args[0] && containsCall(e.args[0]) {
				// 複合代入 `X op= v` は (load X (op X v)) に脱糖されていて X を 2 回評価する。X に関数呼び出しが
				// あるとき (`a[f()] += 1`) は呼び出しを先に 1 回だけ評価して値に置き換えてから続ける
				lhs := h.hoistCalls(e.args[0])
				e = &cexpr{kind: cOp, op: opLoad, args: []*cexpr{lhs, cop2(rhs.op, lhs, rhs.args[1])}, pos: e.pos}
			}
			if lhs := e.args[0]; lhs.kind == cOp && lhs.op == opField {
				// struct のフィールドへの代入。SoA の 2 バイト以上のフィールドは 1 つのポインタで表せないのでここで扱う
				fr := h.fieldRef(lhs.args[0], lhs.name)
				if fr.soaConst {
					panic(&diag.Error{Msg: "cannot assign to element of soa const"})
				}
				if fr.split != nil {
					right := h.rval(h.withExpected(e.args[1], fr.split.typ))
					h.soaStoreSplit(fr.split, right)
					r = right
					break
				}
				r = h.assign(fr.v, fr.lv, e.args[1])
				leftValue = fr.lv
				break
			}
			left, lv := h.lval(e.args[0])
			r = h.assign(left, lv, e.args[1])
			leftValue = lv

		case opNot, opUminus, opBitNot:
			left := h.rval(e.args[0])
			typ := ir.ValType(left)
			if e.op == opNot {
				typ = h.prog.Types.Bool() // `!x` は 0 / 1
			}
			tmp := h.newTmp(typ)
			h.emit(&ir.Op{Code: copToOpCode[e.op], Dst: tmp, Src: []ir.Operand{left}})
			r = tmp

		case opAdd, opSub, opMul, opDiv, opMod,
			opAnd, opOr, opXor, opShiftLeft, opShiftRight:
			left := h.rval(e.args[0])
			right := h.rval(e.args[1])
			typ, l2, r2, cerr := h.tryMakeCompatible(left, right)
			if cerr != nil {
				if (e.op == opAdd || e.op == opSub) &&
					(ir.ValType(left).Kind == types.Pointer || ir.ValType(left).Kind == types.SoaRef) && ir.ValType(right).Kind == types.Int {
					if ir.ValType(left).Kind == types.Pointer && ir.ValType(left).Base.Kind == types.Void {
						panic(&diag.Error{Msg: "no arithmetic on *void"})
					}
					typ = ir.ValType(left)
				} else {
					panic(&diag.Error{Msg: fmt.Sprintf("cannot apply %s to %s and %s (not compatible types)", opSymbol(e.op), ir.ValType(left), ir.ValType(right))})
				}
			} else {
				left, right = l2, r2
			}
			if k, lit := ir.ValIntLiteral(right); lit && k == 0 && (e.op == opDiv || e.op == opMod) {
				panic(&diag.Error{Msg: "div by 0"})
			}
			tmp := h.newTmp(typ)
			h.emit(&ir.Op{Code: copToOpCode[e.op], Dst: tmp, Src: []ir.Operand{left, right}})
			r = tmp

		case opEq, opLt:
			a0, a1 := e.args[0], e.args[1]
			if a0.kind == cNull && a1.kind != cNull {
				a0, a1 = a1, a0 // `null == p` も `p == null` と同じ
			}
			left := h.rval(a0)
			var right ir.Operand
			if a1.kind == cNull {
				right = h.nullOf(ir.ValType(left))
			} else {
				right = h.rval(a1)
			}
			if e.op == opEq && isVoidPtr(ir.ValType(right)) && !isVoidPtr(ir.ValType(left)) {
				left, right = right, left // *void との == は向きを問わない (Compatible は *void を左に置く)
			}
			_, left, right = h.makeCompatible(left, right)
			tmp := h.newTmp(h.prog.Types.Bool()) // 比較の結果は bool (uint8 と互換。language_reference.md §2)
			h.emit(&ir.Op{Code: copToOpCode[e.op], Dst: tmp, Src: []ir.Operand{left, right}})
			r = tmp

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

		case opLand, opLor:
			// 値として使う `a && b` / `a || b` は 0 / 1 (条件文脈では compileCond が分岐に展開する)
			endLabel := h.newLabel("end")
			boolT := h.prog.Types.Bool()
			rr := h.newTmp(boolT)
			h.emit(&ir.Op{Code: ir.OpLoad, Dst: rr, Src: []ir.Operand{ir.NewIntLiteral("", boolT, 0)}})
			h.compileCond(e, endLabel, false)
			h.emit(&ir.Op{Code: ir.OpLoad, Dst: rr, Src: []ir.Operand{ir.NewIntLiteral("", boolT, 1)}})
			h.emit(&ir.Op{Code: ir.OpLabel, Label: endLabel})
			r = rr

		case opCall:
			lmdV := h.rval(e.args[0])
			args := e.args[1:]
			if ir.ValType(lmdV).Kind == types.Macro {
				// マクロの実行
				fn := h.prog.macros[ir.ValLiteral(lmdV)]
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
				lmdType := ir.ValType(lmdV)
				if lmdType.Base.Kind != types.Void {
					r = h.newTmp(lmdType.Base)
				}
				if len(args) != len(lmdType.Params) {
					panic(&diag.Error{Msg: fmt.Sprintf("%s expects %d argument(s) but %d given", describe(lmdV), len(lmdType.Params), len(args))})
				}
				// 引数に呼び出しを含むときは、全部評価してから積む。積んでいる途中で別の呼び出しが走ると、呼び先の引数領域
				// (FC_FASTCALL_REG や、静的フレームなら呼び先と重なりうる兄弟のフレーム) が壊れるので。後ろの引数に
				// 呼び出しがあるときは、先に評価した値を一時変数に写して左から右の評価順を保つ。
				// 呼び出しを含まなければ評価しながら積む (`sub t; push_arg t` が隣り合い、t が A に割り付く)
				pre := containsCallAny(args)
				evalArg := func(i int) ir.Operand {
					v := h.rval(h.withExpected(args[i], lmdType.Params[i]))
					h.compatibleAssign(fmt.Sprintf("argument %d of %s", i+1, describe(lmdV)), lmdType.Params[i], ir.ValType(v))
					return h.cast(v, lmdType.Params[i])
				}
				argVals := make([]ir.Operand, len(args))
				if pre {
					for i := range args {
						v := evalArg(i)
						if containsCallAny(args[i+1:]) {
							v = h.freeze(v)
						}
						argVals[i] = v
					}
				}
				pushCode, argCode, callCode := ir.OpPushResult, ir.OpPushArg, ir.OpCall
				if lmdType.Fastcall() {
					if h.fastCalling {
						panic(&diag.Error{Msg: "cannot fastcall in fastcalling"})
					}
					h.fastCalling = true
					pushCode, argCode, callCode = ir.OpPushFastcallResult, ir.OpPushFastcallArg, ir.OpFastcall
				}
				h.emit(&ir.Op{Code: pushCode, Type: lmdType.Base})
				for i := range args {
					v := argVals[i]
					if !pre {
						v = evalArg(i)
					}
					h.emit(&ir.Op{Code: argCode, Type: lmdType.Params[i], Src: []ir.Operand{v}})
				}
				h.emit(&ir.Op{Code: callCode, Dst: r, Src: []ir.Operand{lmdV}, Far: h.isFarCall(lmdV)})
				h.fastCalling = false
			}

		case opRef: // &演算子
			var left ir.Operand
			var lv bool
			if a := e.args[0]; a.kind == cOp && a.op == opField {
				fr := h.fieldRef(a.args[0], a.name)
				if fr.split != nil {
					panic(&diag.Error{Msg: fmt.Sprintf("cannot take the address of soa field %s (2 bytes or more: stored as separate byte arrays)", a.name)})
				}
				left, lv = fr.v, fr.lv
			} else {
				left, lv = h.lval(a)
			}
			if lv {
				r = left
			} else {
				if !ir.ValAssignable(left) {
					panic(&diag.Error{Msg: fmt.Sprintf("cannot take the address of %s (not a variable)", describe(left))})
				}
				tmp := h.newTmp(h.prog.Types.PointerTo(ir.ValType(left)))
				h.emit(&ir.Op{Code: ir.OpRef, Dst: tmp, Src: []ir.Operand{left}})
				r = tmp
			}

		case opDeref: // *演算子
			if v, lv := h.lval(e.args[0]); lv && ir.ValType(v).Kind == types.SoaRef {
				// `*Points[i]`: 要素 (左辺値) の参照はがしは要素そのもの
				r = v
				leftValue = true
				break
			} else if lv {
				r = h.newTmp(ir.ValType(v).Base)
				h.emit(&ir.Op{Code: ir.OpPget, Dst: r, Src: []ir.Operand{v}})
			} else {
				r = v
			}
			if ir.ValType(r).Kind == types.Pointer && ir.ValType(r).Base.Kind == types.Void {
				panic(&diag.Error{Msg: "cannot dereference *void (bitcast to a typed pointer first)"})
			}
			if ir.ValType(r).Kind != types.Pointer && ir.ValType(r).Kind != types.SoaRef {
				panic(&diag.Error{Msg: fmt.Sprintf("cannot dereference %s (type %s is not a pointer)", describe(r), ir.ValType(r))})
			}
			leftValue = true

		case opMin, opMax, opClamp:
			// 組み込みの min / max / clamp: 互換型の一時変数に入れて、比較して入れ替える
			//   min: t = a; if (b < a) t = b     max: t = a; if (a < b) t = b
			//   clamp: t = x; if (t < lo) t = lo; if (hi < t) t = hi
			vals := make([]ir.Operand, len(e.args))
			for i, a := range e.args {
				vals[i] = h.rval(a)
			}
			typ := ir.ValType(vals[0])
			for _, v := range vals[1:] {
				typ = h.compatible(typ, ir.ValType(v))
			}
			if typ.Kind != types.Int && typ.Kind != types.Bool {
				panic(&diag.Error{Msg: fmt.Sprintf("%s: arguments must be integers (got %s)", e.op, typ)})
			}
			for i := range vals {
				vals[i] = h.cast(vals[i], typ)
			}
			tmp := h.newTmp(typ)
			h.emit(&ir.Op{Code: ir.OpLoad, Dst: tmp, Src: []ir.Operand{vals[0]}})
			replaceIf := func(a, b ir.Operand, with ir.Operand) { // if (a < b) tmp = with
				c := h.newTmp(h.prog.Types.IntType(1, false))
				end := h.newLabel("end")
				h.emit(&ir.Op{Code: ir.OpLt, Dst: c, Src: []ir.Operand{a, b}})
				h.emit(&ir.Op{Code: ir.OpIf, Src: []ir.Operand{c}, Label: end})
				h.emit(&ir.Op{Code: ir.OpLoad, Dst: tmp, Src: []ir.Operand{with}})
				h.emit(&ir.Op{Code: ir.OpLabel, Label: end})
			}
			switch e.op {
			case opMin:
				replaceIf(vals[1], tmp, vals[1])
			case opMax:
				replaceIf(tmp, vals[1], vals[1])
			case opClamp:
				replaceIf(tmp, vals[1], vals[1])
				replaceIf(vals[2], tmp, vals[2])
			}
			r = tmp

		case opField: // struct のフィールド参照 a.f (a が struct へのポインタなら自動で参照はがし)
			fr := h.fieldRef(e.args[0], e.name)
			if fr.split != nil {
				r = h.soaGatherSplit(fr.split)
			} else {
				r, leftValue = fr.v, fr.lv
			}

		case opIndex: // []演算子
			if a := e.args[0]; a.kind == cValue && a.val.Type.IsSoa {
				// `Points[i]`: SoA コンテナの添字はハンドルを作るだけ
				r = h.soaIndex(a.val.Type, h.rval(e.args[1]))
				leftValue = true
				break
			}
			left := h.rval(e.args[0])
			right := h.rval(e.args[1])
			if ir.ValType(left).Kind != types.Pointer && ir.ValType(left).Kind != types.Array {
				panic(&diag.Error{Msg: fmt.Sprintf("cannot index %s (type %s is not a pointer or array)", describe(left), ir.ValType(left))})
			}
			if ir.ValType(left).Base.Kind == types.Void {
				panic(&diag.Error{Msg: "cannot index *void (bitcast to a typed pointer first)"})
			}
			if ir.ValType(right).Kind != types.Int && ir.ValType(right).Kind != types.Bool {
				panic(&diag.Error{Msg: fmt.Sprintf("index must be an integer (got %s)", ir.ValType(right))})
			}
			tmp := h.newTmp(h.prog.Types.PointerTo(ir.ValType(left).Base))
			h.emit(&ir.Op{Code: ir.OpIndex, Dst: tmp, Src: []ir.Operand{left, right}})
			r = tmp
			leftValue = true

		default:
			panic(fmt.Sprintf("unknown op %s", e.op))
		}
	default:
		panic(fmt.Sprintf("unknown expression kind %d", e.kind))
	}
	return r, leftValue
}

// assign は代入 `left = rhs` (left は評価済みの左辺、lv は左辺値 (ポインタ) かどうか)。代入した値 (左辺) を返す。
func (h *Hlc) assign(left ir.Operand, lv bool, rhs *cexpr) ir.Operand {
	if h.needsExpected(rhs) {
		// `p = {1, 2}`: 左辺の型で struct リテラルの型を決める
		lt := ir.ValType(left)
		if lv {
			lt = lt.Base
		}
		rhs = h.withExpected(rhs, lt)
	}
	right := h.rval(rhs)
	if lv {
		if ir.ValType(left).Kind == types.SoaRef {
			h.soaScatter(left, right)
			return left
		}
		h.compatibleAssign("assignment", ir.ValType(left).Base, ir.ValType(right))
		right = h.cast(right, ir.ValType(left).Base)
		h.emit(&ir.Op{Code: ir.OpPset, Src: []ir.Operand{left, right}})
		return left
	}
	h.compatibleAssign("assignment to "+describe(left), ir.ValType(left), ir.ValType(right))
	if !ir.ValAssignable(left) {
		panic(&diag.Error{Msg: fmt.Sprintf("cannot assign to %s (not a variable)", describe(left))})
	}
	right = h.cast(right, ir.ValType(left))
	h.emit(&ir.Op{Code: ir.OpLoad, Dst: left, Src: []ir.Operand{right}})
	return left
}

// warn は警告を記録する (位置は処理中の文/式)。
func (h *Hlc) warn(format string, args ...any) {
	h.prog.Warnings = append(h.prog.Warnings, diag.Warning{Msg: fmt.Sprintf(format, args...), Pos: h.curPos})
}

func (h *Hlc) emit(op *ir.Op) {
	op.Pos = h.curPos
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

func (h *Hlc) newTmp(typ *types.Type) *ir.Value {
	return h.addVar(ir.NewLocal(h.tmpName("$"), typ, ir.LTTemp))
}

// isFarCall は呼び先 fn (関数のシンボルリテラル) が far call (farcall トランポリン経由) になるか (doc/v2_farcall.md §3.2):
// options(farcall: true) が有効で、呼び先が別モジュールの切替バンクの関数で、options(near: true) が付いていないとき。
// 関数ポインタ経由は対象外 (呼ぶ側の責任)。
func (h *Hlc) isFarCall(fn ir.Operand) bool {
	if !h.prog.FarCallEnabled() {
		return false
	}
	lit := ir.ValLiteral(fn)
	if lit == nil || lit.Kind != ir.KindLiteral || lit.IsInt {
		return false
	}
	callee, ok := h.prog.lambdas[lit.Symbol]
	if !ok || callee.Options.Has("near") {
		return false
	}
	if v, ok := callee.Options.Get("abi"); ok && v.Text() == "cc65" {
		// cc65 規約は A/X で引数を渡すが far call のトランポリンは A/Y を壊す。同じバンクに置くか near を付ける
		if calleeAt, callerAt := h.placementOf(callee), h.placementOf(h.lmd); calleeAt != nil && calleeAt != callerAt && calleeAt.Switchable() {
			panic(&diag.Error{Msg: fmt.Sprintf("cannot call cc65 abi function %s across banks (declare it options(near: true) if it is always mapped, or call it from its module)", callee.Name)})
		}
		return false
	}
	// 置き場所はモジュールのセグメントだが、options(segment: X) で別のモジュールのセグメントに置いた関数はそちらに従う
	// (X がモジュール名でなければ配置は手動なので near)
	calleeAt := h.placementOf(callee)
	callerAt := h.placementOf(h.lmd)
	if calleeAt == nil || calleeAt == callerAt || !calleeAt.Switchable() {
		return false
	}
	h.prog.FarCalls = append(h.prog.FarCalls, FarCall{Pos: h.curPos, Caller: h.lmd.Id, Callee: callee.Id})
	return true
}

// placementOf は関数が置かれるモジュール (= セグメント)。options(segment: X) があれば X という id のモジュール、
// X がモジュールでなければ nil (fc の管理外のセグメント)。
func (h *Hlc) placementOf(lmd *ir.Lambda) *ir.Module {
	if seg := lmd.Segment(); seg != "" {
		if m, ok := h.prog.Modules.Get(seg); ok {
			return m
		}
		return nil
	}
	return lmd.Module
}

// containsCall は式 (評価済みでもよい) に関数呼び出し (マクロ呼び出しも含む) が含まれるか。
func containsCall(c *cexpr) bool {
	if c == nil {
		return false
	}
	if c.kind == cOp && c.op == opCall {
		return true
	}
	for _, a := range c.args {
		if containsCall(a) {
			return true
		}
	}
	for _, f := range c.flds {
		if containsCall(f.val) {
			return true
		}
	}
	return false
}

var reAsmSymbol = regexp.MustCompile(`_[A-Za-z0-9_$]+`)

// containsCallAny は式のどれかが関数呼び出しを含むか。
func containsCallAny(cs []*cexpr) bool {
	for _, c := range cs {
		if containsCall(c) {
			return true
		}
	}
	return false
}

// freeze は値を「今の値」に固定する (変数なら一時変数に写す。一時変数・リテラルはそのまま)。
func (h *Hlc) freeze(v ir.Operand) ir.Operand {
	if val, ok := v.(*ir.Value); ok {
		if val.Kind == ir.KindLiteral || (val.Kind == ir.KindLocal && val.LocalType == ir.LTTemp) {
			return v
		}
	}
	tmp := h.newTmp(ir.ValType(v))
	h.emit(&ir.Op{Code: ir.OpLoad, Dst: tmp, Src: []ir.Operand{v}})
	return tmp
}

// hoistCalls は式の中の関数呼び出しを今ここで評価し、その値 (cValue) に置き換えた式を返す (呼び出しが無ければそのまま)。
func (h *Hlc) hoistCalls(c *cexpr) *cexpr {
	if !containsCall(c) {
		return c
	}
	if c.kind == cOp && c.op == opCall {
		return cv(h.operandValue(h.rval(c)))
	}
	n := *c
	n.args = make([]*cexpr, len(c.args))
	for i, a := range c.args {
		n.args[i] = h.hoistCalls(a)
	}
	if len(c.flds) > 0 {
		n.flds = make([]cfield, len(c.flds))
		for i, f := range c.flds {
			n.flds[i] = f
			n.flds[i].val = h.hoistCalls(f.val)
		}
	}
	return &n
}

// operandValue はオペランドを *ir.Value にする (CastedValue などは一時変数に写す)。マクロが cexpr の値として使うため。
func (h *Hlc) operandValue(v ir.Operand) *ir.Value {
	if val, ok := v.(*ir.Value); ok {
		return val
	}
	tmp := h.newTmp(ir.ValType(v))
	h.emit(&ir.Op{Code: ir.OpLoad, Dst: tmp, Src: []ir.Operand{v}})
	return tmp
}

// cast は v を type にキャストする (必要ならコードも生成)。
func (h *Hlc) cast(v ir.Operand, typ *types.Type) ir.Operand {
	if typ.Kind == types.Int {
		// int の変換
		if typ == ir.ValType(v) {
			return v
		}
		if typ.Size <= ir.ValType(v).Size {
			return v
		}
		if !ir.ValType(v).Signed {
			return v
		}
		if ir.ValKind(v) == ir.KindLiteral {
			return v
		}
		newV := h.newTmp(typ)
		h.emit(&ir.Op{Code: ir.OpSignExtension, Dst: newV, Src: []ir.Operand{v}})
		return newV
	} else if typ.Kind == types.Pointer && ir.ValType(v).Kind == types.Array && (ir.ValType(v).Base == typ.Base || typ.Base.Kind == types.Void) {
		return ir.NewPointeredArray(v, h.prog.Types.PointerTo(ir.ValType(v).Base))
	}
	return v
}

// isVoidPtr は *void か。
func isVoidPtr(t *types.Type) bool { return t.Kind == types.Pointer && t.Base.Kind == types.Void }

// nullOf は型 t (ポインタ / 関数ポインタ) の null (0 のリテラル)。SoA のハンドルは 0 が有効な要素なので null を持たない。
func (h *Hlc) nullOf(t *types.Type) ir.Operand {
	switch t.Kind {
	case types.Pointer, types.Func:
		return ir.NewIntLiteral("", t, 0)
	case types.SoaRef:
		panic(&diag.Error{Msg: fmt.Sprintf("soa handle %s has no null (index 0 is a valid element)", t)})
	}
	panic(&diag.Error{Msg: fmt.Sprintf("null cannot be used as %s", t)})
}

// classifyCast は v1 の `<T>x` を v2 の `as` (数値変換) / `bitcast` (ビット読み替え) のどちらで書くべきかを返す
// (fcc migrate 用)。同サイズの整数同士と、配列 → 同じ要素型のポインタは as。それ以外はビット読み替え。
func classifyCast(from, to *types.Type) syntax.CastKind {
	switch {
	case from.Kind == types.Int && to.Kind == types.Int:
		return syntax.CastAs
	case from.Kind == types.Array && to.Kind == types.Pointer && from.Base == to.Base:
		return syntax.CastAs
	}
	return syntax.CastBit
}

// recordCast はキャストの位置と種類を記録する (v1 のキャストを migrate が書き換えるため)。
// 位置は型式の位置で引く (括弧付きの式では cexpr の pos が括弧の位置に上書きされるため)。
func (h *Hlc) recordCast(c *cexpr, from, to *types.Type) {
	if c.ck != syntax.CastLegacy || c.typ == nil {
		return
	}
	h.prog.CastKinds[syntax.At(h.module.Path, c.typ.Pos())] = classifyCast(from, to)
}

// checkCast はキャストの種類ごとの規則を検査する (doc/v2_types_struct.md §3.5)。
func (h *Hlc) checkCast(kind syntax.CastKind, from, to *types.Type) {
	switch kind {
	case syntax.CastAs:
		// 数値変換: 整数 → 整数、配列 → 同じ要素型のポインタ
		if (from.Kind == types.Int || from.Kind == types.Bool) && (to.Kind == types.Int || to.Kind == types.Bool) {
			return
		}
		if from.Kind == types.Array && to.Kind == types.Pointer && (from.Base == to.Base || to.Base.Kind == types.Void) {
			return
		}
		if (from.Kind == types.Int && to.Kind == types.SoaRef) || (from.Kind == types.SoaRef && to.Kind == types.Int) {
			return // SoA のハンドルはインデックス (整数) と相互に変換できる
		}
		panic(&diag.Error{Msg: fmt.Sprintf("cannot convert %s to %s with `as` (use bitcast for bit reinterpretation)", from, to)})
	case syntax.CastBit:
		// ビット読み替え: サイズが同じもの同士 (配列はポインタ = 2 バイトとみなす)
		fromSize := from.Size
		if from.Kind == types.Array {
			fromSize = 2
		}
		if to.Kind == types.Array || to.Kind == types.Void || from.Kind == types.Void {
			panic(&diag.Error{Msg: fmt.Sprintf("cannot bitcast %s to %s", from, to)})
		}
		if fromSize != to.Size {
			panic(&diag.Error{Msg: fmt.Sprintf("cannot bitcast %s (%d bytes) to %s (%d bytes): sizes differ", from, fromSize, to, to.Size)})
		}
	}
}

// explicitCast は明示キャストの値を作る。
//   - v1 `<T>x` と `bitcast<T>(x)`: 型ラベルの貼り替え (CastedValue)
//   - `x as T`: 数値変換。拡張は元が符号付きなら符号拡張、縮小は下位バイト、同サイズはビットそのまま
func (h *Hlc) explicitCast(kind syntax.CastKind, v ir.Operand, to *types.Type) ir.Operand {
	from := ir.ValType(v)
	h.checkCast(kind, from, to)
	if kind == syntax.CastAs {
		if from.Kind == types.Array {
			return ir.NewPointeredArray(v, to)
		}
		if to.Size > from.Size && from.Signed {
			newV := h.newTmp(to)
			h.emit(&ir.Op{Code: ir.OpSignExtension, Dst: newV, Src: []ir.Operand{v}})
			return newV
		}
	}
	return ir.NewCastedValue(v, to, 0)
}

// makeCompatible は互換型に変換する (キャストコード生成込み)。
func (h *Hlc) makeCompatible(a, b ir.Operand) (*types.Type, ir.Operand, ir.Operand) {
	typ := h.compatible(ir.ValType(a), ir.ValType(b))
	a = h.cast(a, typ)
	b = h.cast(b, typ)
	return typ, a, b
}

// tryMakeCompatible は makeCompatible の CompileError を捕捉するバージョン。
func (h *Hlc) tryMakeCompatible(a, b ir.Operand) (typ *types.Type, ra, rb ir.Operand, err *diag.Error) {
	defer func() {
		if r := recover(); r != nil {
			if ce, ok := r.(*diag.Error); ok {
				err = ce
				return
			}
			panic(r)
		}
	}()
	typ, ra, rb = h.makeCompatible(a, b)
	return
}

// ReadSource はソースファイルを読み込む。改行は CRLF → LF に正規化する (文字列リテラル内の改行が OS で変わらないように)。
func ReadSource(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")), nil
}
