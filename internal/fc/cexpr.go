package fc

// cexpr は HLC 内部の式表現。
//
// 構文木 (syntax.Expr) をそのまま定数評価の対象にせず、一度この形に変換してから評価する。
// 理由は 2 つ:
//   - 構文木は不変に保つ (C1)。旧実装 (Ruby 由来) は定数評価が AST を破壊的に書き換えていた
//   - マクロの展開結果は「評価済みの値 (*Value) を葉に持つ式」であり、構文木では表せない
//
// constEval の入力 (未評価) と出力 (評価済み: 葉は cValue、演算ノードの子は評価済み) の両方を表す。
// 評価済みの木に constEval を再適用しても結果は変わらない (冪等)。
//
// 旧実装では `x += 1` の脱糖 `(load X (add X 1))` で部分木 X が 2 箇所から共有され、
// 破壊的評価により「2 回目の訪問は評価済みの X を見る」挙動になっていた。
// これは Hlc.cmemo (同一ノードの評価結果のメモ) で再現する。

import "github.com/haramako/fc/internal/syntax"

type ckind uint8

const (
	cValue  ckind = iota // 評価済みの値 (val)
	cInt                 // 整数リテラル (n)
	cStr                 // 文字列リテラル (s)
	cIdent               // 識別子 (name)
	cArray               // 配列リテラル (args)
	cIncbin              // incbin (s = パス)
	cLambda              // 関数リテラル (lambda)
	cDot                 // モジュール参照 args[0] . name
	cCast                // キャスト args[0] を typ へ (評価後は ty)
	cOp                  // 演算 op (args)
)

// cop は演算の種類。文字列値は旧 IR の opcode (Sym) と同じ綴り。
type cop string

const (
	opLoad       cop = "load"
	opAdd        cop = "add"
	opSub        cop = "sub"
	opMul        cop = "mul"
	opDiv        cop = "div"
	opMod        cop = "mod"
	opAnd        cop = "and"
	opOr         cop = "or"
	opXor        cop = "xor"
	opLand       cop = "land"
	opLor        cop = "lor"
	opEq         cop = "eq"
	opNe         cop = "ne"
	opLt         cop = "lt"
	opGt         cop = "gt"
	opLe         cop = "le"
	opGe         cop = "ge"
	opShiftLeft  cop = "shift_left"
	opShiftRight cop = "shift_right"
	opNot        cop = "not"
	opUminus     cop = "uminus"
	opCall       cop = "call" // args[0] = 関数、args[1:] = 実引数、block = 後置ブロック
	opIndex      cop = "index"
	opRef        cop = "ref"
	opDeref      cop = "deref"
)

// lambdaParam は関数リテラルの引数 (名前と型式)。
type lambdaParam struct {
	name string
	typ  syntax.TypeExpr
}

// lambdaLit は関数リテラル (関数宣言の脱糖結果、または `-> type { ... }`)。
type lambdaLit struct {
	params []lambdaParam
	result syntax.TypeExpr
	body   *syntax.Block // nil なら extern
	opt    *OMap         // id / options(...) の生の値
}

type cexpr struct {
	kind  ckind
	val   *Value
	n     int
	s     string
	name  string
	op    cop
	args  []*cexpr
	typ   syntax.TypeExpr // cCast の型式
	ty    *Type           // cCast の評価後の型
	block *syntax.Block   // opCall の後置ブロック
	lam   *lambdaLit      // cLambda
	pos   syntax.Pos      // 元の構文木上の位置 (エラー報告用)
}

// ---------------------------------------------------------------
// コンストラクタ (マクロ実装からも使う)
// ---------------------------------------------------------------

func cv(v *Value) *cexpr         { return &cexpr{kind: cValue, val: v} }
func cint(n int) *cexpr          { return &cexpr{kind: cInt, n: n} }
func cstr(s string) *cexpr       { return &cexpr{kind: cStr, s: s} }
func cident(name string) *cexpr  { return &cexpr{kind: cIdent, name: name} }
func carray(elems []*cexpr) *cexpr { return &cexpr{kind: cArray, args: elems} }

func cop2(op cop, args ...*cexpr) *cexpr { return &cexpr{kind: cOp, op: op, args: args} }

// ccall は関数呼び出し fn(args...)。
func ccall(fn *cexpr, args ...*cexpr) *cexpr {
	return &cexpr{kind: cOp, op: opCall, args: append([]*cexpr{fn}, args...)}
}

// isLiteralInt は評価済みの整数リテラルかを返す。
func (c *cexpr) isLiteralInt() bool {
	return c.kind == cValue && c.val.Kind == "literal" && isInt(c.val.Val)
}

// ---------------------------------------------------------------
// 構文木 → cexpr (未評価)
// ---------------------------------------------------------------

var binaryOps = map[syntax.Kind]cop{
	syntax.Plus: opAdd, syntax.Minus: opSub, syntax.Star: opMul, syntax.Slash: opDiv,
	syntax.Percent: opMod, syntax.Amp: opAnd, syntax.Pipe: opOr, syntax.Caret: opXor,
	syntax.AndAnd: opLand, syntax.OrOr: opLor, syntax.EqEq: opEq, syntax.Neq: opNe,
	syntax.Lt: opLt, syntax.Gt: opGt, syntax.Leq: opLe, syntax.Geq: opGe,
	syntax.Shl: opShiftLeft, syntax.Shr: opShiftRight,
}

var unaryOps = map[syntax.Kind]cop{
	syntax.Not: opNot, syntax.Minus: opUminus, syntax.Star: opDeref, syntax.Amp: opRef,
}

// toC は構文木の式を未評価の cexpr に変換する。
func toC(e syntax.Expr) *cexpr {
	c := toC0(e)
	c.pos = e.Pos()
	return c
}

func toC0(e syntax.Expr) *cexpr {
	switch e := e.(type) {
	case *syntax.Ident:
		return cident(e.Name)
	case *syntax.IntLit:
		return cint(e.Value)
	case *syntax.StringLit:
		return cstr(e.Value)
	case *syntax.ParenExpr:
		return toC(e.X)
	case *syntax.BinaryExpr:
		if e.Op == syntax.Dot {
			id, ok := e.Y.(*syntax.Ident)
			if !ok {
				panic(&CompileError{Msg: "dot: right side must be identifier"})
			}
			return &cexpr{kind: cDot, args: []*cexpr{toC(e.X)}, name: id.Name}
		}
		return cop2(binaryOps[e.Op], toC(e.X), toC(e.Y))
	case *syntax.AssignExpr:
		lhs := toC(e.Lhs)
		switch e.Op {
		case syntax.AddEq:
			// 旧文法の脱糖 (load X (add X rhs))。X は同一ノードを共有する (cmemo で 1 回だけ評価される)
			return cop2(opLoad, lhs, cop2(opAdd, lhs, toC(e.Rhs)))
		case syntax.SubEq:
			return cop2(opLoad, lhs, cop2(opSub, lhs, toC(e.Rhs)))
		}
		return cop2(opLoad, lhs, toC(e.Rhs))
	case *syntax.UnaryExpr:
		if e.Op == syntax.Plus {
			return toC(e.X)
		}
		return cop2(unaryOps[e.Op], toC(e.X))
	case *syntax.CastExpr:
		return &cexpr{kind: cCast, args: []*cexpr{toC(e.X)}, typ: e.Type}
	case *syntax.CallExpr:
		args := make([]*cexpr, 0, len(e.Args)+1)
		args = append(args, toC(e.Fun))
		for _, a := range e.Args {
			args = append(args, toC(a))
		}
		return &cexpr{kind: cOp, op: opCall, args: args, block: e.Block}
	case *syntax.IndexExpr:
		return cop2(opIndex, toC(e.X), toC(e.Index))
	case *syntax.ArrayLit:
		elems := make([]*cexpr, len(e.Elems))
		for i, el := range e.Elems {
			elems[i] = toC(el)
		}
		return carray(elems)
	case *syntax.IncbinExpr:
		return &cexpr{kind: cIncbin, s: e.Path.Value}
	case *syntax.LambdaExpr:
		ft, ok := e.Type.(*syntax.FuncType)
		if !ok {
			panic(&CompileError{Msg: "must be lambda type"})
		}
		params := make([]lambdaParam, len(ft.Params))
		for i, p := range ft.Params {
			if p.Name == nil {
				panic(&CompileError{Msg: "lambda parameter must have a name"})
			}
			params[i] = lambdaParam{name: p.Name.Name, typ: p.Type}
		}
		return &cexpr{kind: cLambda, lam: &lambdaLit{params: params, result: ft.Result, body: e.Body, opt: NewOMap()}}
	}
	panic("toC: unknown expression")
}

// ---------------------------------------------------------------
// 型式 → 旧形式の型 AST (TypeOf の入力)
// ---------------------------------------------------------------

// typeAST は型式を TypeOf が受け付ける旧形式に変換する。
// 配列長は整数リテラルのときだけ数値になり、それ以外 (省略・定数式) は長さ省略扱いになる
// (旧実装の TypeOf は評価していない AST を受け取るとそう振る舞った)。最外の配列長を
// 定数評価するのは typeEval の仕事。
// R1-e で TypeOf と共に型付きコンストラクタに置き換える。
func typeAST(t syntax.TypeExpr) any {
	switch t := t.(type) {
	case nil:
		return nil
	case *syntax.NamedType:
		return Sym(t.Name.Name)
	case *syntax.PointerType:
		return []any{Sym("pointer"), typeAST(t.Elem)}
	case *syntax.ArrayType:
		var n any
		if lit, ok := t.Len.(*syntax.IntLit); ok {
			n = lit.Value
		}
		return []any{Sym("array"), n, typeAST(t.Elem)}
	case *syntax.FuncType:
		params := make([]any, len(t.Params))
		for i, p := range t.Params {
			if p.Name != nil {
				// 旧実装では型位置の名前付き引数は TypeOf が "invalid type declaration" で落ちる。同じ形を渡す
				params[i] = []any{Sym(p.Name.Name), typeAST(p.Type)}
			} else {
				params[i] = typeAST(p.Type)
			}
		}
		return []any{Sym("lambda"), params, typeAST(t.Result)}
	}
	panic("typeAST: unknown type expression")
}

// rawOptionValue は options(...) の値を、Value.Opt / Lambda.Opt に格納する生の形にする
// (旧実装はパース直後の AST をそのまま格納していた: 整数 / 文字列 / 識別子(Sym))。
// それ以外の式は構文木ノードをそのまま入れる (旧実装の AST 相当。corpus には存在しない)。
func rawOptionValue(e syntax.Expr) any {
	switch e := e.(type) {
	case *syntax.IntLit:
		return e.Value
	case *syntax.StringLit:
		return e.Value
	case *syntax.Ident:
		return Sym(e.Name)
	}
	return e
}

// rawOptions は options(...) を Sym → 生の値 の OMap にする (重複キーは後勝ち・位置維持)。
// nil なら nil を返す。
func rawOptions(o *syntax.Options) *OMap {
	if o == nil {
		return nil
	}
	m := NewOMap()
	for _, e := range o.Entries {
		m.Set(Sym(e.Key.Name), rawOptionValue(e.Value))
	}
	return m
}
