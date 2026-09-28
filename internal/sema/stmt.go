package sema

// 文の IR 化 (if / loop / for / switch / break / continue …) と条件式の直接分岐化。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

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
		case *syntax.LoopStmt, *syntax.WhileStmt, *syntax.ForStmt, *syntax.ForInStmt:
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
	shifts := len(h.shifts)
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if len(h.shifts) > shifts {
			h.shifts = h.shifts[:shifts]
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
		h.cmemo, h.sliceMemo = nil, nil
		h.prog.report(ce) // 上限なら Fatal を投げる
		h.declareBad(s)
	}()
	h.compileStatement(s)
	h.checkShifts(shifts)
}

func (h *Hlc) compileStatement(s syntax.Stmt) {
	h.updatePos(s)
	h.cmemo, h.sliceMemo = nil, nil
	if h.prog.LogEveryStatement && h.prog.LogEnabled && h.lmd != nil {
		h.logEveryStatement()
	}

	switch s := s.(type) {

	case *syntax.PlacementBlock:
		h.compilePlacementBlock(s)
	case *syntax.Block:
		// ブロックごとにスコープを作る (language_reference §1.3。素の `{ }` の宣言が外へ漏れ、同じ名前を宣言できなかった)
		h.inScope(func() { h.compileStmts(s.Stmts) })

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
			checkBareOption(e)
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
			if r.key == "bank" && val.Kind == ir.OptStr {
				h.updatePos(r.val)
				val = h.bankByName(val.Str) // @(bank: "en"): fc.toml のバンクの表で番号に (fixed は -1)
			}
			if r.key == "bss" {
				h.updatePos(r.val)
				validateBss(val)
			} else {
				h.prog.Options.Set(r.key, val)
			}
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
				// fc 1 の Ruby マクロ (削除済み)。案内だけ残す
				panic(&diag.Error{Msg: fmt.Sprintf("include(%q): .rb macros are not supported (printf / unittest_run_tests are built in; use `const T = textmap(\"...\")` for text tables)", filename)})
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
					h.module.AsmSymbols = append(h.module.AsmSymbols, ir.AsmSymbols(string(data))...)
				}
			}
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
				v := m.Lookup(name.Name)
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
			params[i] = lambdaParam{name: p.Name.Name, typ: p.Type, init: p.Init}
		}
		lam := &cexpr{kind: cLambda, pos: s.Pos(), lam: &lambdaLit{
			name: s.Name.Name, params: params, result: s.Result, body: s.Body, options: parseOptions(s.Options),
		}}
		h.compileConstSpec(s.Name.Name, s.Name.End(), nil, lam, nil, s.PublicPos)

	case *syntax.VarDecl:
		if s.Alias {
			h.compileStorageAlias(s)
		} else if s.Const {
			for _, sp := range s.Specs {
				opts := parseOptions(sp.Options)
				build := opts.Flag("build")
				var init *cexpr
				if build {
					e := h.buildConstInit(sp.Name.Name, sp)
					if str, ok := e.(*syntax.StringLit); ok {
						h.declareBuildString(sp.Name.Name, str.Value, s.PublicPos).Build = true
						continue
					}
					init = toC(e)
				} else if sp.Init != nil {
					init = toC(sp.Init)
				}
				h.compileConstSpec(sp.Name.Name, sp.Name.End(), sp.Type, init, opts, s.PublicPos)
				if build {
					if v := h.scope.Local(sp.Name.Name); v != nil {
						v.Build = true
					}
				}
			}
		} else {
			for _, sp := range s.Specs {
				h.compileVarSpec(sp, s.PublicPos)
			}
		}

	case *syntax.EnumDecl:
		h.compileEnumDecl(s)

	case *syntax.StaticIfStmt:
		// fc 3 の @if (関数の中): 選ばれた側だけを同じスコープでコンパイルする
		for _, st := range h.staticBranch(s) {
			h.compileStatement(st)
		}

	case *syntax.IfStmt:
		labels := h.newLabels("then", "else", "end")
		thenLabel, elseLabel, endLabel := labels[0], labels[1], labels[2]
		h.compileCond(toC(s.Cond), elseLabel, false)
		h.emit(&ir.Op{Code: ir.OpLabel, Label: thenLabel})
		h.inScope(func() { h.compileStatement(s.Then) })
		if s.Else == nil {
			h.warnBranchEndLog()
		}
		h.emit(&ir.Op{Code: ir.OpJump, Label: endLabel})
		h.emit(&ir.Op{Code: ir.OpLabel, Label: elseLabel})
		if s.Else != nil {
			h.inScope(func() { h.compileStatement(s.Else) })
			h.warnBranchEndLog()
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
		// { init; loop { if (cond) { body; step: step; } else break; } } (while への脱糖と同じ IR の形)。
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

	case *syntax.ForInStmt:
		h.compileForIn(s)

	case *syntax.IncDecStmt:
		// x++ → x = x + 1 (v1 の for のインクリメントと同じ形。左辺値は 2 回評価される)
		op := syntax.Plus
		if s.Op == syntax.Dec {
			op = syntax.Minus
		}
		h.compileStatement(&syntax.ExprStmt{X: &syntax.AssignExpr{Lhs: s.X, OpPos: s.OpPos, Op: syntax.Assign,
			Rhs: &syntax.BinaryExpr{X: s.X, OpPos: s.OpPos, Op: op, Y: &syntax.IntLit{ValuePos: s.OpPos, Value: 1, Text: "1"}}}})

	case *syntax.FallthroughStmt:
		panic(&diag.Error{Msg: "fallthrough must be the last statement of a switch case"})

	case *syntax.BreakStmt:
		h.emit(&ir.Op{Code: ir.OpJump, Label: h.findBreakable("break", s.Label).breakLabel})

	case *syntax.ContinueStmt:
		b := h.findBreakable("continue", s.Label)
		if b.isSwitch {
			panic(&diag.Error{Msg: fmt.Sprintf("cannot continue a switch (label %s)", b.label)})
		}
		h.emit(&ir.Op{Code: ir.OpJump, Label: b.continueLabel})

	case *syntax.ReturnStmt:
		h.requireFunction()
		if h.lmd.Type.Base.Kind != types.Void {
			// 非void関数
			if s.Value == nil {
				panic(&diag.Error{Msg: fmt.Sprintf("can't return without value from %s (returns %s)", h.lmd.Name, h.lmd.Type.Base)})
			}
			rt := h.lmd.Type.Base
			rc := h.withExpected(toC(s.Value), rt)
			v := h.rval(rc)
			h.compatibleAssign("return from "+h.lmd.Name, rt, ir.ValType(v))
			h.warnDropConst("return from "+h.lmd.Name, rt, v)
			h.warnReturnLocalAddr(s.Value, rt)
			h.emit(&ir.Op{Code: ir.OpReturn, Src: []ir.Operand{h.convert(v, rt, rc)}})
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
		vals := make([][]caseItem, len(s.Cases))
		allInt, n, minV, maxV := true, 0, 0, 0
		for ci, c := range s.Cases {
			for _, v := range c.Values {
				if r, ok := v.(*syntax.RangeExpr); ok {
					// fc 3 の `case lo..hi:` / `case lo..=hi:` (2026-09-27)
					it := h.caseRange(r, ir.ValType(cond))
					for k := it.lo; k <= it.last; k++ {
						if seen[k] {
							panic(&diag.Error{Msg: fmt.Sprintf("duplicate case value %d (in the range %d..=%d)", k, it.lo, it.last), Pos: syntax.At(h.module.Path, v.Pos())})
						}
						seen[k] = true
					}
					if n == 0 || it.lo < minV {
						minV = it.lo
					}
					if n == 0 || it.last > maxV {
						maxV = it.last
					}
					n += it.last - it.lo + 1
					vals[ci] = append(vals[ci], it)
					continue
				}
				cv := h.constEvalOperand(h.withExpected(toC(v), ir.ValType(cond))) // enum なら `case .A:`
				if t := ir.ValType(cv); t.Kind == types.Array || t.Kind == types.Struct {
					// `case "A":` (C の文字のつもり。fc の "A" / 'A' は 2 バイトの文字列) が codegen で panic していた
					msg := fmt.Sprintf("case value must be an integer constant (got %s)", t)
					if lv := ir.ValLiteral(cv); lv != nil && lv.IsString {
						msg += "; a string literal is an array in fc (write the character code, e.g. 65 for \"A\")"
					}
					panic(&diag.Error{Msg: msg, Pos: syntax.At(h.module.Path, v.Pos())})
				}
				if k, ok := ir.ValIntLiteral(cv); ok {
					if ct := ir.ValType(cond); ct.Kind == types.Int && ct.Enum == nil && ct.Size <= 2 {
						// case の値も比較と同じく、タグの型に収まらなければエラー (u8 の `case 300:` が黙って一度も一致せず、
						// `case -1:` が 255 に一致していた。survey 2026-09-27、`case -1:` のエラーは 2026-09-27 決定)
						if lo, hi := intRange(ct); k < lo || k > hi {
							panic(&diag.Error{Msg: fmt.Sprintf("case value %d does not fit in %s, the type of the switch value", k, ct), Pos: syntax.At(h.module.Path, v.Pos())})
						}
					}
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
				vals[ci] = append(vals[ci], caseItem{v: cv})
			}
		}
		h.warnEnumSwitch(ir.ValType(cond), seen, s.Default != nil)
		// ジャンプテーブル (switchTableMin 個以上の整数の case が密に並ぶとき。language_reference.md §5):
		//   switch tag, min, [label...]; jump default; case...: ...; jump end; default: ...; end:
		// 1 バイトのタグだけ (飛び先 - 1 を pha; pha; rts で飛ぶ。比較の連鎖は平均 3 + 5N/2 サイクル、表は約 33 で一定)
		if allInt && n >= switchTableMin && ir.ValType(cond).Size == 1 && maxV-minV+1 <= 2*n && maxV-minV+1 <= 255 && !h.prog.Config.Disabled("switch") {
			defaultLabel := h.newLabel("default")
			caseLabels := make([]string, len(s.Cases))
			table := make([]string, maxV-minV+1)
			for k := range table {
				table[k] = defaultLabel
			}
			for ci := range s.Cases {
				caseLabels[ci] = h.newLabel("case")
				for _, it := range vals[ci] {
					if it.isRange {
						for k := it.lo; k <= it.last; k++ {
							table[k-minV] = caseLabels[ci]
						}
						continue
					}
					k, _ := ir.ValIntLiteral(it.v)
					table[k-minV] = caseLabels[ci]
				}
			}
			h.emit(&ir.Op{Code: ir.OpSwitch, Src: []ir.Operand{cond, h.IntValue(minV)}, Labels: table})
			h.emit(&ir.Op{Code: ir.OpJump, Label: defaultLabel})
			for ci, c := range s.Cases {
				h.emit(&ir.Op{Code: ir.OpLabel, Label: caseLabels[ci]})
				next := defaultLabel // fallthrough の行き先 (次の case の本体。最後の case なら default)
				if ci+1 < len(caseLabels) {
					next = caseLabels[ci+1]
				}
				h.compileCaseBody(s, ci, c.Body, next, endLabel)
			}
			h.emit(&ir.Op{Code: ir.OpLabel, Label: defaultLabel})
		} else {
			// fallthrough の行き先の本体の先頭のラベル (fallthrough される case だけ置く。ラベルはブロックの切れ目に
			// なって最適化の結果を変えうるので、使わないときは今までどおり出さない)
			bodyLabels := make([]string, len(s.Cases)+1) // 最後は default の本体
			for ci, c := range s.Cases {
				if endsWithFallthrough(c.Body) {
					bodyLabels[ci+1] = h.newLabel("case")
				}
			}
			for ci, c := range s.Cases {
				labels := h.newLabels("then", "else")
				thenLabel, elseLabel := labels[0], labels[1]
				if bodyLabels[ci] != "" {
					thenLabel = bodyLabels[ci]
				}
				// 値ごとに `eq t; if_true t goto then`、最後の値だけ `eq t; if t goto else` (一致しなければ次の case へ)。
				// t は値ごとに新しい一時変数にする (定義 1 つ + 直後で使用、でコンディションフラグに割り付く: cmp; bne)
				for k, it := range vals[ci] {
					tmp := h.newTmp(h.prog.Types.IntType(1, false))
					if it.isRange {
						h.caseRangeTest(tmp, cond, it)
					} else {
						h.emit(&ir.Op{Code: ir.OpEq, Dst: tmp, Src: []ir.Operand{cond, it.v}})
					}
					if k < len(vals[ci])-1 {
						h.emit(&ir.Op{Code: ir.OpIfTrue, Src: []ir.Operand{tmp}, Label: thenLabel})
					} else {
						h.emit(&ir.Op{Code: ir.OpIf, Src: []ir.Operand{tmp}, Label: elseLabel})
					}
				}
				if len(vals[ci]) > 1 || bodyLabels[ci] != "" {
					h.emit(&ir.Op{Code: ir.OpLabel, Label: thenLabel})
				}
				h.compileCaseBody(s, ci, c.Body, bodyLabels[ci+1], endLabel)
				h.emit(&ir.Op{Code: ir.OpLabel, Label: elseLabel})
			}
			if l := bodyLabels[len(s.Cases)]; l != "" {
				h.emit(&ir.Op{Code: ir.OpLabel, Label: l})
			}
		}
		if s.Default != nil {
			h.compileCaseBody(s, -1, s.Default.Body, "", endLabel)
		}
		h.emit(&ir.Op{Code: ir.OpLabel, Label: endLabel})
		h.popBreakable()

	default:
		panic(fmt.Sprintf("unknown statement %T", s))
	}
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
	if e.kind == cOp && e.op == opEq {
		// `f == true` (f は bool): 0 かどうかだけを見る (`lda f / bne`。bool は正規化しないので 5 も真)
		for k := 0; k < 2; k++ {
			if isTrueLit(e.args[1-k]) {
				v := h.rval(e.args[k])
				if ir.ValType(v).Kind == types.Bool {
					code := ir.OpIf
					if jumpIfTrue {
						code = ir.OpIfTrue
					}
					h.emit(&ir.Op{Code: code, Src: []ir.Operand{v}, Label: label})
					return
				}
				e = cop2(opEq, cv(h.operandValue(v)), e.args[1-k])
				break
			}
		}
	}
	v := h.rval(e)
	if isAggregate(ir.ValType(v)) {
		panic(&diag.Error{Msg: fmt.Sprintf("a value of type %s cannot be used as a condition (struct / array values have only == and !=)", ir.ValType(v))})
	}
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

// requireFunction は関数の外 (トップレベルの裸のブロックの中など) の実行文をエラーにする
// (fuzz で発覚。h.lmd (現在の関数) が無いまま進むと nil 参照で落ちる)。
func (h *Hlc) requireFunction() {
	if h.lmd == nil {
		panic(&diag.Error{Msg: "executable statement is not allowed at module level; put it in a function"})
	}
}

// endsWithFallthrough は case の本体の最後の文が `fallthrough;` か (fc 3)。
func endsWithFallthrough(body []syntax.Stmt) bool {
	if len(body) == 0 {
		return false
	}
	_, ok := body[len(body)-1].(*syntax.FallthroughStmt)
	return ok
}

// compileCaseBody は switch の case (ci。default は -1) の本体と、その後ろのジャンプ (switch の出口 end、fallthrough
// なら次の case の本体 next) を出す。fc 3 では case ごとにスコープを作る (case の中の宣言は、ほかの case と switch の
// 後ろからは見えない)。fc 2 は今までどおり (囲むスコープに宣言する)。
func (h *Hlc) compileCaseBody(s *syntax.SwitchStmt, ci int, body []syntax.Stmt, next, end string) {
	target := end
	if endsWithFallthrough(body) {
		ft := body[len(body)-1]
		body = body[:len(body)-1]
		switch {
		case ci < 0:
			h.updatePos(ft)
			panic(&diag.Error{Msg: "cannot fallthrough from default (it is the last clause)"})
		case ci == len(s.Cases)-1 && s.Default == nil:
			h.updatePos(ft)
			panic(&diag.Error{Msg: "cannot fallthrough from the last case (no default follows)"})
		}
		target = next
	}
	if h.version() >= syntax.Version3 {
		for _, st := range body {
			if d, ok := st.(*syntax.VarDecl); ok {
				if h.caseDecls == nil {
					h.caseDecls = map[string]bool{}
				}
				for _, sp := range d.Specs {
					h.caseDecls[sp.Name.Name] = true
				}
			}
		}
		h.inScope(func() { h.compileStmts(body) })
	} else {
		h.compileStmts(body)
	}
	if ci >= 0 || target != end {
		h.emit(&ir.Op{Code: ir.OpJump, Label: target})
	}
}
