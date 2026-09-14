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

// MacroFn は Go 組み込みマクロ (Ruby 版 fclib/*.rb の defmacro 相当)。
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
		panic(&diag.Error{Msg: "constant value required"})
	}
	return c.val
}

// mustString は文字列リテラル由来の値であることを要求し、その文字列を返す。
func mustString(c *cexpr) string {
	v := mustValue(c)
	if !v.IsString {
		panic(&diag.Error{Msg: "string literal required"})
	}
	return v.Str
}

// compatible は互換型を返す (なければ CompileError)。
func (h *Hlc) compatible(a, b *types.Type) *types.Type {
	r := h.prog.Types.Compatible(a, b)
	if r == nil {
		panic(&diag.Error{Msg: fmt.Sprintf("not compatible type '%s' and '%s'", a, b)})
	}
	return r
}

// guessType は宣言型 typ (省略可) と初期値 val から変数の型を決める。
func (h *Hlc) guessType(typ *types.Type, val ir.Operand) *types.Type {
	if typ != nil {
		return h.compatible(typ, ir.ValType(val))
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
		h.compileStatement(s)
	}
}

func (h *Hlc) mustInModule() {
	if h.lmd != nil {
		panic(&diag.Error{Msg: "must be at module level (not inside a function)"})
	}
}

// scopeIsPublic は宣言の可視性を決める (文に public が付いていればそれ、なければモジュールの現在値)。
func (h *Hlc) scopeIsPublic(publicPos syntax.Pos) bool {
	return publicPos.IsValid() || h.module.CurrentPublic
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
		case "macro":
			if h.module.Version >= syntax.Version2 {
				panic(&diag.Error{Msg: fmt.Sprintf("include(%q): Ruby macros are not supported in fc 2 (printf / unittest_run_tests are built in; use `const T = textmap(\"...\")` for text tables)", filename)})
			}
			// 既知の .rb はファイルが無くても受理する (組み込みで代替済み。fclib/*.rb はリポジトリから削除した)
			reg, ok := macroFiles[filename]
			if !ok {
				ref, _ := h.resolveFile(filename)
				panic(&diag.Error{Msg: fmt.Sprintf("macro file %s is not supported by go port", ref)})
			}
			reg(h)
		case "chr":
			ref, _ := h.resolveFile(filename)
			h.module.IncludeChrs = append(h.module.IncludeChrs, ref)
		default:
			panic(&diag.Error{Msg: fmt.Sprintf("invalid keyword %s", kind)})
		}
		h.module.Depends = append(h.module.Depends, filename)

	case *syntax.UseDecl:
		h.mustInModule()
		id := s.Module.Name
		m := h.useModule(id)
		h.module.AddUse(m)
		// 再輸出: v1 は常に (glob も束縛も public)、v2 は `public use` のときだけ (doc/v2_grammar.md §3.2)
		reexport := h.module.Version < syntax.Version2 || s.PublicPos.IsValid()
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
		cond := h.rval(toC(s.Cond))
		h.emit(&ir.Op{Code: ir.OpIf, Src: []ir.Operand{cond}, Label: elseLabel})
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
				panic(&diag.Error{Msg: "can't return without value"})
			}
			h.emit(&ir.Op{Code: ir.OpReturn, Src: []ir.Operand{h.rval(toC(s.Value))}})
		} else {
			// void関数
			if s.Value != nil {
				panic(&diag.Error{Msg: "can't return with value from void function"})
			}
			h.emit(&ir.Op{Code: ir.OpReturn})
		}

	case *syntax.ExprStmt:
		h.lval(toC(s.X))

	case *syntax.SwitchStmt:
		// TODO: jumptableを使った実装をいれる
		cond := h.rval(toC(s.Tag))
		tmp := h.newTmp(h.prog.Types.IntType(1, false))
		endLabel := h.newLabel("end")
		// v2: ラベルなし break は switch を抜ける。v1 では switch は break の対象外 (外側のループを抜ける)
		isV2 := h.module.Version >= syntax.Version2
		if isV2 {
			h.pushBreakable(breakable{isSwitch: true, breakLabel: endLabel})
		} else {
			h.pendingLabel = nil
		}
		for _, c := range s.Cases {
			labels := h.newLabels("then", "else")
			thenLabel, elseLabel := labels[0], labels[1]
			for _, v := range c.Values {
				h.emit(&ir.Op{Code: ir.OpEq, Dst: tmp, Src: []ir.Operand{cond, h.constEvalOperand(toC(v))}})
				h.emit(&ir.Op{Code: ir.OpNot, Dst: tmp, Src: []ir.Operand{tmp}})
				h.emit(&ir.Op{Code: ir.OpIf, Src: []ir.Operand{tmp}, Label: thenLabel})
			}
			h.emit(&ir.Op{Code: ir.OpJump, Label: elseLabel})
			h.emit(&ir.Op{Code: ir.OpLabel, Label: thenLabel})
			h.compileStmts(c.Body)
			h.emit(&ir.Op{Code: ir.OpJump, Label: endLabel})
			h.emit(&ir.Op{Code: ir.OpLabel, Label: elseLabel})
		}
		if s.Default != nil {
			h.compileStmts(s.Default.Body)
		}
		h.emit(&ir.Op{Code: ir.OpLabel, Label: endLabel})
		if isV2 {
			h.popBreakable()
		}

		// TODO: 一時的に、public/privateの切り替えを可能にしている。そのうち消すこと
	case *syntax.ScopeLabel:
		h.module.CurrentPublic = s.Public

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
	var init ir.Operand
	if sp.Init != nil {
		init = h.rval(toC(sp.Init))
	}
	if init != nil && h.lmd == nil {
		panic(&diag.Error{Msg: "can't init global variable"})
	}
	typ := h.typeEval(sp.Type)
	if typ == nil {
		typ = h.guessType(nil, init)
	}
	if typ != nil && init != nil {
		h.compatible(typ, ir.ValType(init))
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
		cv := h.constEval(val)
		if cv.kind != cValue {
			panic(&diag.Error{Msg: fmt.Sprintf("const %s must be constant", name)})
		}
		v := cv.val
		t := h.guessType(h.typeEval(typ), v)
		if v.Type.Kind == types.Macro {
			// const T = textmap("..."): マクロ値そのものを名前に束縛する (シンボルは作らない。型指定は guessType で弾かれる)
			v.Name = name
			newVal = h.addVar(v)
		} else if v.Kind == ir.KindArrayLiteral {
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
		panic(&diag.Error{Msg: "constant value required"})
	}
	return r.val
}

func (h *Hlc) constEval0(c *cexpr) *cexpr {
	switch c.kind {

	case cInt:
		return cv(h.IntValue(c.n))

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
		return cv(ir.NewSymbolLiteral("", lmd.Type, id))

	case cDot:
		left := h.constEval(c.args[0])
		var mod *ir.ModuleInterface
		if left.kind == cValue {
			mod = left.val.Module
		}
		if mod == nil {
			panic(&diag.Error{Msg: fmt.Sprintf("%s is not a module", c.args[0].name)})
		}
		return cv(mod.LookupMust(c.name))

	case cCast:
		x := h.constEval(c.args[0])
		ty := c.ty // 評価済みノードの再評価では型を再計算しない
		if ty == nil {
			ty = h.typeEval(c.typ)
		}
		if x.isLiteralInt() {
			return cv(ir.NewIntLiteral("", ty, x.val.Int))
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
				v1 := args[0].val.Int
				v2 := 0
				if len(args) > 1 {
					v2 = args[1].val.Int
				}
				return cv(h.IntValue(foldIntOp(c.op, v1, v2)))
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

// IntValue は値から型を推定した整数リテラル (Value.new_int 相当)。
// 旧実装の境界 (-128 が sint16 になる) をそのまま保存する。
func (h *Hlc) IntValue(n int) *ir.Value {
	var t *types.Type
	switch {
	case n >= 256:
		t = h.prog.Types.IntType(2, false)
	case n < -127:
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
	// 旧実装は truthy(opt[:fastcall]) で、値が何であれキーがあれば fastcall 扱い
	typ := h.prog.Types.Func(argTypes, baseType, opts.Has("fastcall"))
	return &ir.Lambda{Id: id, Name: name, Params: params, Type: typ, Options: opts, Extern: body == nil, Body: body}
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
	case opUminus:
		return -v1
	case opShiftLeft:
		return ir.Shl(v1, v2)
	case opShiftRight:
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
		ty, ok := h.prog.Types.Named(t.Name.Name)
		if !ok {
			panic(&diag.Error{Msg: fmt.Sprintf("invalid basic type %s", t.Name.Name)})
		}
		return ty
	case *syntax.PointerType:
		return h.prog.Types.PointerTo(h.typeOf(t.Elem))
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
		panic(&diag.Error{Msg: "expression has no value (void)"})
	}
	if left {
		r := h.newTmp(ir.ValType(v).Base)
		h.emit(&ir.Op{Code: ir.OpPget, Dst: r, Src: []ir.Operand{v}})
		return r
	}
	return v
}

// lval は左辺値として評価し、(値, 左辺値かどうか) を返す。
func (h *Hlc) lval(c *cexpr) (ir.Operand, bool) {
	defer h.enterExpr(c.pos)()
	leftValue := false
	e := h.constEval(c)
	var r ir.Operand

	switch e.kind {

	case cValue:
		if e.val.Kind == ir.KindArrayLiteral {
			symbol := h.addDef(h.tmpName("_"), &ir.Def{Kind: ir.DefBlock, Type: e.val.Type, Elems: e.val.Elems})
			r = ir.NewGlobal(h.tmpName("$"), e.val.Type, symbol)
		} else {
			r = e.val
		}

	case cCast:
		r = ir.NewCastedValue(h.rval(e.args[0]), e.ty, 0)

	case cOp:
		switch e.op {

		case opLoad:
			left, lv := h.lval(e.args[0])
			right := h.rval(e.args[1])
			if lv {
				h.compatible(ir.ValType(left).Base, ir.ValType(right))
				right = h.cast(right, ir.ValType(left).Base)
				h.emit(&ir.Op{Code: ir.OpPset, Src: []ir.Operand{left, right}})
				r = left
				leftValue = true
			} else {
				h.compatible(ir.ValType(left), ir.ValType(right))
				if !ir.ValAssignable(left) {
					panic(&diag.Error{Msg: fmt.Sprintf("%s is not left value", ir.OperandString(left))})
				}
				right = h.cast(right, ir.ValType(left))
				h.emit(&ir.Op{Code: ir.OpLoad, Dst: left, Src: []ir.Operand{right}})
				r = left
			}

		case opNot, opUminus:
			left := h.rval(e.args[0])
			tmp := h.newTmp(ir.ValType(left))
			h.emit(&ir.Op{Code: copToOpCode[e.op], Dst: tmp, Src: []ir.Operand{left}})
			r = tmp

		case opAdd, opSub, opMul, opDiv, opMod,
			opAnd, opOr, opXor, opShiftLeft, opShiftRight:
			left := h.rval(e.args[0])
			right := h.rval(e.args[1])
			typ, l2, r2, cerr := h.tryMakeCompatible(left, right)
			if cerr != nil {
				if (e.op == opAdd || e.op == opSub) &&
					ir.ValType(left).Kind == types.Pointer && ir.ValType(right).Kind == types.Int {
					typ = ir.ValType(left)
				} else {
					panic(cerr)
				}
			} else {
				left, right = l2, r2
			}
			tmp := h.newTmp(typ)
			h.emit(&ir.Op{Code: copToOpCode[e.op], Dst: tmp, Src: []ir.Operand{left, right}})
			r = tmp

		case opEq, opLt:
			left := h.rval(e.args[0])
			right := h.rval(e.args[1])
			_, left, right = h.makeCompatible(left, right)
			tmp := h.newTmp(h.prog.Types.IntType(1, false))
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

		case opLand:
			endLabel := h.newLabel("end")
			rr := h.newTmp(h.prog.Types.IntType(1, false))
			left := h.rval(e.args[0])
			h.emit(&ir.Op{Code: ir.OpLoad, Dst: rr, Src: []ir.Operand{left}})
			h.emit(&ir.Op{Code: ir.OpIf, Src: []ir.Operand{rr}, Label: endLabel})
			right := h.rval(e.args[1])
			h.emit(&ir.Op{Code: ir.OpLoad, Dst: rr, Src: []ir.Operand{right}})
			h.emit(&ir.Op{Code: ir.OpLabel, Label: endLabel})
			r = rr

		case opLor:
			endLabel := h.newLabel("end")
			rr := h.newTmp(h.prog.Types.IntType(1, false))
			r2 := h.newTmp(h.prog.Types.IntType(1, false))
			left := h.rval(e.args[0])
			h.emit(&ir.Op{Code: ir.OpLoad, Dst: rr, Src: []ir.Operand{left}})
			h.emit(&ir.Op{Code: ir.OpNot, Dst: r2, Src: []ir.Operand{rr}})
			h.emit(&ir.Op{Code: ir.OpIf, Src: []ir.Operand{r2}, Label: endLabel})
			right := h.rval(e.args[1])
			h.emit(&ir.Op{Code: ir.OpLoad, Dst: rr, Src: []ir.Operand{right}})
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
					panic(&diag.Error{Msg: fmt.Sprintf("%s has %d but %d", ir.OperandString(lmdV), len(lmdType.Params), len(args))})
				}
				if h.lmd.Type.Fastcall() {
					panic(&diag.Error{Msg: "cannot call function from fastcall"})
				}

				if lmdType.Fastcall() {
					if h.fastCalling {
						panic(&diag.Error{Msg: "cannot fastcall in fastcalling"})
					}
					h.fastCalling = true
					h.emit(&ir.Op{Code: ir.OpPushFastcallResult, Type: lmdType.Base})
					for i, arg := range args {
						v := h.rval(arg)
						h.compatible(lmdType.Params[i], ir.ValType(v))
						v = h.cast(v, lmdType.Params[i])
						h.emit(&ir.Op{Code: ir.OpPushFastcallArg, Type: lmdType.Params[i], Src: []ir.Operand{v}})
					}
					h.emit(&ir.Op{Code: ir.OpFastcall, Dst: r, Src: []ir.Operand{lmdV}})
					h.fastCalling = false
				} else {
					h.emit(&ir.Op{Code: ir.OpPushResult, Type: lmdType.Base})
					for i, arg := range args {
						v := h.rval(arg)
						h.compatible(lmdType.Params[i], ir.ValType(v))
						v = h.cast(v, lmdType.Params[i])
						h.emit(&ir.Op{Code: ir.OpPushArg, Type: lmdType.Params[i], Src: []ir.Operand{v}})
					}
					h.emit(&ir.Op{Code: ir.OpCall, Dst: r, Src: []ir.Operand{lmdV}})
				}
			}

		case opRef: // &演算子
			left, lv := h.lval(e.args[0])
			if lv {
				r = left
			} else {
				if !ir.ValAssignable(left) {
					panic(&diag.Error{Msg: fmt.Sprintf("%s is not left value", ir.OperandString(left))})
				}
				tmp := h.newTmp(h.prog.Types.PointerTo(ir.ValType(left)))
				h.emit(&ir.Op{Code: ir.OpRef, Dst: tmp, Src: []ir.Operand{left}})
				r = tmp
			}

		case opDeref: // *演算子
			r = h.rval(e.args[0])
			if ir.ValType(r).Kind != types.Pointer {
				// Ruby版では未代入の `left` を参照するため空文字列になる
				panic(&diag.Error{Msg: " is not pointer"})
			}
			leftValue = true

		case opIndex: // []演算子
			left := h.rval(e.args[0])
			right := h.rval(e.args[1])
			if ir.ValType(left).Kind != types.Pointer && ir.ValType(left).Kind != types.Array {
				panic(&diag.Error{Msg: "index must be pointer or array"})
			}
			if ir.ValType(right).Kind != types.Int {
				panic(&diag.Error{Msg: "index must be int"})
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
	} else if typ.Kind == types.Pointer && ir.ValType(v).Kind == types.Array && ir.ValType(v).Base == typ.Base {
		return ir.NewPointeredArray(v, h.prog.Types.PointerTo(ir.ValType(v).Base))
	}
	return v
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

// ReadSource はソースファイルを読み込む。
// Ruby版は File.read (テキストモード) で読むため、Windows では CRLF→LF 変換が行われる。
// 同じ挙動になるよう常に CRLF→LF 変換する (golden は Windows で生成されている)。
func ReadSource(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")), nil
}
