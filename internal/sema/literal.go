package sema

import (
	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// adaptLiteral は、片方が型のない整数定数 (ir.Value.Untyped: リテラル・型を書かない const・sizeof など) の二項演算・比較で、
// 定数を相手の整数型に合わせる (Agent/discussions/2026-09-20-v3-plan.md §10.2。Go・Rust と同じ考え方)。
//
// 値が相手の型に収まるときは、今の互換型 (大きいほう、同じ大きさなら符号付き) も相手の型になるので何もしない
// (`vx < 0` (vx:i8) は i8、`w + 5` (w:u16) は u16)。定数のほうが大きい型のとき (`x * 300`、`0x2000 + y * 32`) も今までどおり
// 広いほうで計算する。変わるのは、大きさは相手以下で符号のために収まらないときだけ:
//   - 演算 (cmp == false): 相手の型に切り詰める。`x + -1` (x:u8) は u8 の x + 255 (以前は i8 になっていた)。
//     `x - 128` (x:i8) のように相手が符号付きなら、以前から相手の型なので結果は同じ
//   - 比較 (cmp == true): エラー。`s < 200` (s:i8) は 200 が -56 に、`x == -1` (x:u8) は x == 255 になっていた
//
// 判断は型を決める段の計画の adaptLit (typeplan.go) で、ここは評価した値に当てる形 (for-each の範囲・定数の畳み込みが使う)。
func (h *Hlc) adaptLiteral(a, b ir.Operand, cmp bool) (ir.Operand, ir.Operand) {
	ai, bi := valueInfo(a), valueInfo(b)
	return h.adaptLit(ai, bi, cmp).apply(a), h.adaptLit(bi, ai, cmp).apply(b)
}

// litDecision は二項演算・比較の片方の型のない整数定数を、相手に合わせるときの判断 (literalRule)。
type litDecision int

const (
	litKeep       litDecision = iota // そのまま (定数同士・相手が整数でない・定数のほうが大きい型: 互換型は広いほう)
	litFits                          // 相手の型に収まる (値はそのままで、互換型が相手の型になる)
	litTruncate                      // 演算で相手 (符号なし) の型に切り詰める (`x + -1` (x:u8) は x + 255)
	litSignedWide                    // 比較で相手が符号付き、定数が大きい型: 定数の大きさの符号付きの型にする (`s < 300` (s:i8) は i16)
	litCmpError                      // 比較で相手の型に収まらない: エラー
	litTooWide                       // fc 4 の演算で、定数がどの整数型 (32 ビット) にも入らない: エラー
)

// maxIntBytes は整数型のいちばん大きいバイト数 (u32 / i32。fc 4 だけ。Agent/wiki/design/wide-int.md)。
const maxIntBytes = 4

// maxIntSize はコンパイル中のモジュールで使える整数型のいちばん大きいバイト数 (fc 4 は 4、fc 2 / 3 は 2)。型のない定数の型
// (IntValue) と、その外の定数を数学の値のまま扱う境界 (foldBeyond など) がこれで決まる。
func (h *Hlc) maxIntSize() int {
	if h.v4() {
		return maxIntBytes
	}
	return 2
}

// anyIntRange は size バイトのどれかの整数型に入る値の範囲 (符号付きの下限〜符号なしの上限)。
func anyIntRange(size int) (lo, hi int) { return -(1 << (8*size - 1)), 1<<(8*size) - 1 }

// literalRule は型のない整数定数 (型 lt、値 n) を相手の型 other に合わせる規則 (Agent/discussions/2026-09-20-v3-plan.md §10.2)。
// otherUntyped は相手も型のない定数か。cmp は比較か。maxSize はその版の整数型のいちばん大きいバイト数 (maxIntSize)。
// 値を作り直すのは adaptLiteral、型だけを決めるのは型を決める段 (typeplan.go の adaptLit) で、どちらもこの判断を使う。
func literalRule(lt *types.Type, n int, otherUntyped bool, other *types.Type, cmp bool, maxSize int) litDecision {
	if otherUntyped || other.Kind != types.Int || other.Tagged() || other.Size < 1 || other.Size > maxSize {
		return litKeep // 定数同士 (普通は畳み込まれている) か、相手が整数でない
	}
	if lo, hi := anyIntRange(maxSize); n < lo || n > hi {
		// どの整数型にも入らない定数: 比較は foldBeyond が数学の値で畳む。fc 4 の演算はエラー (fc 2 / 3 は今までどおり広いほうの型)
		if !cmp && maxSize > 2 {
			return litTooWide
		}
		return litKeep
	}
	if lt.Size > other.Size {
		// 定数のほうが大きい型: 今までどおり広げる。比較で相手が符号付きなら、定数の大きさの符号付きの型にする
		// (`s < 300` (s:i8) が u16 で比べて -10 < 300 が偽だった)
		if cmp && other.Signed && !lt.Signed && n <= 1<<(8*lt.Size-1)-1 {
			return litSignedWide
		}
		return litKeep
	}
	if lo, hi := intRange(other); n >= lo && n <= hi {
		return litFits
	}
	if cmp {
		return litCmpError
	}
	if other.Signed {
		return litFits // 互換型が既に相手の型 (同じ大きさなら符号付きが勝つ)。値もそのままでよい (IR を以前と同じに保つ)
	}
	return litTruncate
}

// intRange は整数型 t (1〜4 バイト) の値の範囲。
func intRange(t *types.Type) (lo, hi int) {
	hi = 1<<(8*t.Size) - 1
	if t.Signed {
		lo, hi = -(hi+1)/2, (hi+1)/2-1
	}
	return lo, hi
}

// foldBeyond は、どの整数型 (fc 4 は 32 ビット、fc 2 / 3 は 16 ビット。maxIntSize) にも収まらない定数と整数の値の比較を畳む
// (結果は値によらず決まる)。広げる先の型が無く、定数を切り詰めて比べていた (fc 3 の `w == 65537` (w = 1) が真。警告は
// 「決して等しくない」と出ていた)。
func (h *Hlc) foldBeyond(op cop, left, right ir.Operand) (int, bool) {
	lo, hi := anyIntRange(h.maxIntSize())
	beyond := func(v ir.Operand) (int, bool) {
		k, ok := ir.ValIntLiteral(v)
		return k, ok && (k < lo || k > hi)
	}
	isInt := func(v ir.Operand) bool {
		t := ir.ValType(v)
		return t.Kind == types.Int && !t.Tagged()
	}
	// 相手の値: リテラルなら自分の型で読んだ値 (両方とも定数のときも数学の値で比べる)、変数なら型の範囲のどこか
	cmp := func(a, b int) int {
		switch {
		case op == opEq && a == b, op == opLt && a < b:
			return 1
		}
		return 0
	}
	if k, ok := beyond(right); ok && isInt(left) {
		if n, lit := ir.ValIntLiteral(left); lit {
			return cmp(wrapInt(n, ir.ValType(left)), k), true
		}
		if op == opEq {
			return 0, true
		}
		return b2i(k > 0), true // v < k
	}
	if k, ok := beyond(left); ok && isInt(right) {
		if n, lit := ir.ValIntLiteral(right); lit {
			return cmp(k, wrapInt(n, ir.ValType(right))), true
		}
		if op == opEq {
			return 0, true
		}
		return b2i(k < 0), true // k < v
	}
	return 0, false
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// adaptLiteralNoErr は adaptLiteral と同じだが、比較で収まらない定数のエラーを出さずにそのまま返す (定数の畳み込みの型を
// 決めるため。エラーは実行時の式を作る側 (lval) が出す)。
func (h *Hlc) adaptLiteralNoErr(a, b ir.Operand, cmp bool) (ra, rb ir.Operand) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(*diag.Error); !ok {
				panic(r)
			}
			ra, rb = a, b
		}
	}()
	return h.adaptLiteral(a, b, cmp)
}

// rewriteBeyond16 は fc 3 のモジュールの二項の算術 e (項の値 left, right) で、16 ビットに入らない型のない定数が型付きの整数と
// 計算されるとき、migrate に `as` (fc 3 の定数の型 u16 / i16) を報告する。fc 3 は定数を 16 ビットの型にして実行時に折り返すが、
// fc 4 は u24 / u32 で計算するので、そのまま移すと結果が変わる (fuzz の TestRandomMigrate。Agent/wiki/design/wide-int.md)。
func (h *Hlc) rewriteBeyond16(e *cexpr, left, right ir.Operand) {
	vals := [2]ir.Operand{left, right}
	for i, v := range vals {
		if i == 1 && (e.op == opShiftLeft || e.op == opShiftRight) {
			break // シフト量は折り返さない
		}
		lv, ok := v.(*ir.Value)
		other := ir.ValType(vals[1-i])
		if !ok || lv.Kind != ir.KindLiteral || !lv.IsInt || !lv.Untyped || other.Kind != types.Int || other.Tagged() {
			continue
		}
		if lv.Int < -32768 || lv.Int > 65535 {
			h.rewriteAs("constant-range", e.args[i], lv.Type.String())
		}
	}
}
