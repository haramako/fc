package sema

// cexpr は HLC 内部の式表現。
//
// 構文木 (syntax.Expr) をそのまま定数評価の対象にせず、一度この形に変換してから評価する。
// 理由は 2 つ:
//   - 構文木は不変に保つ (C1)。定数評価は cexpr の側で行い、構文木には触らない
//   - マクロの展開結果は「評価済みの値 (*ir.Value) を葉に持つ式」であり、構文木では表せない
//
// constEval の入力 (未評価) と出力 (評価済み: 葉は cValue、演算ノードの子は評価済み) の両方を表す。
// 評価済みの木に constEval を再適用しても結果は変わらない (冪等)。
//
// 旧実装では `x += 1` の脱糖 `(load X (add X 1))` で部分木 X が 2 箇所から共有され、
// 破壊的評価により「2 回目の訪問は評価済みの X を見る」挙動になっていた。
// これは Hlc.cmemo (同一ノードの評価結果のメモ) で再現する。

import (
	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

type ckind uint8

const (
	cValue     ckind = iota // 評価済みの値 (val)
	cInt                    // 整数リテラル (n)
	cStr                    // 文字列リテラル (s)
	cIdent                  // 識別子 (name)
	cArray                  // 配列リテラル (args)
	cIncbin                 // incbin (s = パス)
	cLambda                 // 関数リテラル (lambda)
	cDot                    // モジュール参照 args[0] . name
	cCast                   // キャスト args[0] を typ へ (評価後は ty)
	cOp                     // 演算 op (args)
	cStructLit              // struct リテラル (typ = 型名 (省略なら nil)、ty = 確定した型、fields)
	cSizeof                 // sizeof(typ)
	cNull                   // null (型は文脈から。ty が決まれば 0 のリテラルになる)
	cEnumShort              // fc 3 の `.Name` (enum のメンバー。型は文脈から (withExpected)。name)
	cNullFn                 // fc 3 の @null_fn (何もしない関数。型は文脈の fn(...):void。無ければ fn():void)
)

// cop は演算の種類。文字列値は IR の opcode 名と同じ綴り。
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
	opField      cop = "field"    // args[0] . name (struct のフィールド参照。cDot の評価で module でないと分かったもの)
	opMin        cop = "min"      // min(a, b) (組み込み。型は両辺の互換型、符号もそれに従う)
	opMax        cop = "max"      // max(a, b)
	opClamp      cop = "clamp"    // clamp(x, lo, hi)
	opBitNot     cop = "bitnot"   // ~x
	opSlice      cop = "slice"    // fc 3 の範囲 args[0][args[1]..args[2]] (lo / hi は省けば nil)
	opToSlice    cop = "to_slice" // 配列 / slice args[0] を slice の型 ty にする (withExpected が挟む)
	opLen        cop = "len"      // @len(args[0]) の実行時の値 (slice の長さ)
)

// compoundOps は複合代入 `x op= y` の op。
var compoundOps = map[syntax.Kind]cop{
	syntax.AddEq: opAdd, syntax.SubEq: opSub, syntax.MulEq: opMul, syntax.DivEq: opDiv, syntax.ModEq: opMod,
	syntax.AndEq: opAnd, syntax.OrEq: opOr, syntax.XorEq: opXor, syntax.ShlEq: opShiftLeft, syntax.ShrEq: opShiftRight,
}

// opSymbol は演算のソース上の綴り (エラーメッセージ用)。
func opSymbol(op cop) string {
	switch op { // 単項演算
	case opNot:
		return "!"
	case opBitNot:
		return "~"
	case opUminus:
		return "-"
	}
	for k, v := range binaryOps {
		if v == op {
			return k.String()
		}
	}
	return string(op)
}

// cfield は struct リテラルの 1 項目。key が "" なら位置指定。
type cfield struct {
	key string
	val *cexpr
}

// lambdaParam は関数リテラルの引数 (名前と型式)。
type lambdaParam struct {
	name string
	typ  syntax.TypeExpr
	init syntax.Expr // named function declaration's optional constant default
}

// lambdaLit は関数リテラル (関数宣言の脱糖結果、または `-> type { ... }`)。
type lambdaLit struct {
	name    string // 宣言名 (関数リテラルでは "")
	params  []lambdaParam
	result  syntax.TypeExpr
	body    *syntax.Block // nil なら extern
	options ir.Options    // options(...) の生の値
}

type cexpr struct {
	kind  ckind
	val   *ir.Value
	n     int
	s     string
	name  string
	op    cop
	args  []*cexpr
	typ   syntax.TypeExpr // cCast の型式
	ty    *types.Type     // cCast の評価後の型
	ck    syntax.CastKind // cCast の種類 (v1 <T>x / as / bitcast)
	block *syntax.Block   // opCall の後置ブロック
	lam   *lambdaLit      // cLambda
	flds  []cfield        // cStructLit の項目
	rt    bool            // cArray: 実行時の値を要素に持つ (lval が一時変数に組み立てる。ty は文脈の配列型、無ければ nil)
	pos   syntax.Pos      // 元の構文木上の位置 (エラー報告用)
}

// ---------------------------------------------------------------
// コンストラクタ (マクロ実装からも使う)
// ---------------------------------------------------------------

func cv(v *ir.Value) *cexpr        { return &cexpr{kind: cValue, val: v} }
func cint(n int) *cexpr            { return &cexpr{kind: cInt, n: n} }
func cstr(s string) *cexpr         { return &cexpr{kind: cStr, s: s} }
func cident(name string) *cexpr    { return &cexpr{kind: cIdent, name: name} }
func carray(elems []*cexpr) *cexpr { return &cexpr{kind: cArray, args: elems} }

func cop2(op cop, args ...*cexpr) *cexpr { return &cexpr{kind: cOp, op: op, args: args} }

// ccall は関数呼び出し fn(args...)。
func ccall(fn *cexpr, args ...*cexpr) *cexpr {
	return &cexpr{kind: cOp, op: opCall, args: append([]*cexpr{fn}, args...)}
}

// isLiteralInt は評価済みの整数リテラルかを返す。
func (c *cexpr) isLiteralInt() bool {
	return c.kind == cValue && c.val.Kind == ir.KindLiteral && c.val.IsInt
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
	syntax.Not: opNot, syntax.Minus: opUminus, syntax.Star: opDeref, syntax.Amp: opRef, syntax.Tilde: opBitNot,
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
		if e.Name == "@null_fn" {
			return &cexpr{kind: cNullFn} // 型は文脈から (withExpected)
		}
		return cident(e.Name)
	case *syntax.IntLit:
		return cint(e.Value)
	case *syntax.BoolLit:
		if e.Value {
			return &cexpr{kind: cInt, n: 1, s: "bool"}
		}
		return &cexpr{kind: cInt, n: 0, s: "bool"}
	case *syntax.NullLit:
		return &cexpr{kind: cNull}
	case *syntax.StringLit:
		return cstr(e.Value)
	case *syntax.ParenExpr:
		return toC(e.X)
	case *syntax.EnumShortExpr:
		return &cexpr{kind: cEnumShort, name: e.Name.Name}
	case *syntax.BinaryExpr:
		if e.Op == syntax.Dot {
			id, ok := e.Y.(*syntax.Ident)
			if !ok {
				panic(&diag.Error{Msg: "dot: right side must be identifier"})
			}
			return &cexpr{kind: cDot, args: []*cexpr{toC(e.X)}, name: id.Name}
		}
		return cop2(binaryOps[e.Op], toC(e.X), toC(e.Y))
	case *syntax.AssignExpr:
		lhs := toC(e.Lhs)
		if op, ok := compoundOps[e.Op]; ok {
			// 複合代入の脱糖 (load X (op X rhs))。X は同一ノードを共有する (定数評価は cmemo で 1 回。
			// 実行時の評価は 2 回になるので、X に呼び出しがあれば hlc.go の opLoad で先に評価する)
			return cop2(opLoad, lhs, cop2(op, lhs, toC(e.Rhs)))
		}
		return cop2(opLoad, lhs, toC(e.Rhs))
	case *syntax.UnaryExpr:
		if e.Op == syntax.Plus {
			return toC(e.X)
		}
		return cop2(unaryOps[e.Op], toC(e.X))
	case *syntax.CastExpr:
		return &cexpr{kind: cCast, args: []*cexpr{toC(e.X)}, typ: e.Type, ck: e.Kind}
	case *syntax.CallExpr:
		args := make([]*cexpr, 0, len(e.Args)+1)
		args = append(args, toC(e.Fun))
		for _, a := range e.Args {
			args = append(args, toC(a))
		}
		return &cexpr{kind: cOp, op: opCall, args: args, block: e.Block}
	case *syntax.IndexExpr:
		return cop2(opIndex, toC(e.X), toC(e.Index))
	case *syntax.SliceExpr:
		c := &cexpr{kind: cOp, op: opSlice, args: []*cexpr{toC(e.X), nil, nil}}
		if e.Lo != nil {
			c.args[1] = toC(e.Lo)
		}
		if e.Hi != nil {
			c.args[2] = toC(e.Hi)
		}
		return c
	case *syntax.ArrayLit:
		elems := make([]*cexpr, len(e.Elems))
		for i, el := range e.Elems {
			elems[i] = toC(el)
		}
		return carray(elems)
	case *syntax.IncbinExpr:
		return &cexpr{kind: cIncbin, s: e.Path.Value}
	case *syntax.StructLit:
		flds := make([]cfield, len(e.Fields))
		for i, f := range e.Fields {
			if f.Key != nil {
				flds[i].key = f.Key.Name
			}
			flds[i].val = toC(f.Value)
		}
		c := &cexpr{kind: cStructLit, flds: flds}
		if e.Type != nil {
			c.typ = e.Type
		}
		return c
	case *syntax.SizeofExpr:
		return &cexpr{kind: cSizeof, typ: e.Type}
	case *syntax.LambdaExpr:
		ft, ok := e.Type.(*syntax.FuncType)
		if !ok {
			panic(&diag.Error{Msg: "must be lambda type"})
		}
		params := make([]lambdaParam, len(ft.Params))
		for i, p := range ft.Params {
			if p.Name == nil {
				panic(&diag.Error{Msg: "lambda parameter must have a name"})
			}
			params[i] = lambdaParam{name: p.Name.Name, typ: p.Type}
		}
		return &cexpr{kind: cLambda, lam: &lambdaLit{params: params, result: ft.Result, body: e.Body}}
	}
	panic("toC: unknown expression")
}

// parseOptions は options(...) を生の値のまま ir.Options にする (重複キーは後勝ち・位置維持)。
// 値は整数 / 文字列 / 識別子のいずれか。nil なら nil。
// checkBareOption は値を省いた属性 (`@(inline)`) が真偽値の属性か検査する (bank / address などは値が要る)。
func checkBareOption(e *syntax.OptionEntry) {
	if e.Bare && !ir.FlagOptions[e.Key.Name] {
		panic(&diag.Error{Msg: "@(" + e.Key.Name + ") needs a value (`" + e.Key.Name + ": ...`; only flag attributes such as inline can omit it)"})
	}
}

func parseOptions(o *syntax.Options) ir.Options {
	if o == nil {
		return nil
	}
	var r ir.Options
	for _, e := range o.Entries {
		checkBareOption(e)
		if e.Key.Name == "bss" {
			panic(&diag.Error{Msg: "bss is only allowed on modules and placement blocks; use segment on individual declarations"})
		}
		var v ir.OptionValue
		switch x := e.Value.(type) {
		case *syntax.IntLit:
			v = ir.OptionValue{Kind: ir.OptInt, Int: x.Value}
		case *syntax.StringLit:
			v = ir.OptionValue{Kind: ir.OptStr, Str: x.Value}
		case *syntax.Ident:
			v = ir.OptionValue{Kind: ir.OptIdent, Str: x.Name}
		case *syntax.BoolLit:
			// options(fastcall: true): v1 の識別子 `true` と同じ扱い (ダンプも同じ。fastcall はキーの有無だけを見る)
			v = ir.OptionValue{Kind: ir.OptIdent, Str: "false"}
			if x.Value {
				v.Str = "true"
			}
		default:
			panic(&diag.Error{Msg: "option " + e.Key.Name + " must be a literal or identifier"})
		}
		r.Set(e.Key.Name, v)
	}
	return r
}

// copToOpCode は HLC 内部の演算名から IR の OpCode への対応。
var copToOpCode = map[cop]ir.OpCode{
	opLoad: ir.OpLoad, opAdd: ir.OpAdd, opSub: ir.OpSub, opMul: ir.OpMul, opDiv: ir.OpDiv, opMod: ir.OpMod,
	opAnd: ir.OpAnd, opOr: ir.OpOr, opXor: ir.OpXor, opShiftLeft: ir.OpShiftLeft, opShiftRight: ir.OpShiftRight,
	opNot: ir.OpNot, opUminus: ir.OpUminus, opEq: ir.OpEq, opLt: ir.OpLt, opBitNot: ir.OpBitNot,
}
