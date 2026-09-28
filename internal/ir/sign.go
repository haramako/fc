package ir

import "github.com/haramako/fc/internal/types"

// 幅と符号: 意味が入力の幅・符号で変わる命令は、それを命令自身に持つ (2026-09-28。doc/development_notes.md「コードの構造」)。
//
//	eq / lt       Width 比較の幅 (バイト)。入力はこの幅で読む (変数はゼロ拡張、リテラルは値のバイト: codegen の byte と同じ)
//	lt            Sign  符号付きの比較か
//	div / mod     Sign  符号付きの床除算か (幅は Dst)
//	shift_right   Sign  算術シフトか (幅は Dst)
//
// 以前は各段 (opt/ssa の畳み込み、unroll の回数、codegen、regalloc の常駐・フラグの選択、carry / split の判断) が入力の型から
// 「比較は広い方の幅、どちらかが符号付きなら符号付き」「除算は Dst の符号」「右シフトは入力の符号」をそれぞれ導いていて、
// 入力を差し替える段が型を変えると意味が変わった (常駐の差し替えが cast を落として比較が符号付きに: TestResidentKeepsCast、
// propagateBytes が sint8 の一時変数を uint8 のリテラルに置き換えて符号なしの比較に: TestPropagateBytesKeepsType)。
// 今は sema が命令を作るとき (Hlc.emit) に InferWidthSign で決め、以後の段は Op.Width / Op.Sign だけを見る。型から導く規則は
// ここだけ (interp は差分テストの独立した判定役なので、sema の直後の IR を型から自分で解釈する)。opt が命令を作るときは、
// 元の命令の Width / Sign を写すか、新しい入力から InferWidthSign で決める。決め忘れは Verify が落とす (比較の Width が 0、
// lt / div / mod / shift_right の Sign が SignNone)。
//
// 暗黙の変換 (狭い入力のゼロ拡張、広い入力の下位の切り詰め) は命令にしない: 規則は 1 つ (codegen の byte、interp の byteOf、
// opt/ssa の castBits) で、符号拡張だけが sign_extension 命令。

// InferWidthSign は op の Width (比較) と Sign (lt / div / mod / shift_right) を入力と Dst の型から決め、op を返す:
//
//	eq / lt:     Width = 入力の大きい方。lt は どちらかの入力が符号付きなら Signed
//	div / mod:   Dst が符号付きなら Signed
//	shift_right: 入力 (Src[0]) が符号付きなら Signed (算術シフト)
//
// ほかの命令はそのまま。
func InferWidthSign(op *Op) *Op {
	switch op.Code {
	case OpEq:
		op.Width = max(ValType(op.Src[0]).Size, ValType(op.Src[1]).Size)
	case OpLt:
		t0, t1 := ValType(op.Src[0]), ValType(op.Src[1])
		op.Width = max(t0.Size, t1.Size)
		op.Sign = SignOf(isSignedInt(t0) || isSignedInt(t1))
	case OpDiv, OpMod:
		op.Sign = SignOf(isSignedInt(ValType(op.Dst)))
	case OpShiftRight:
		op.Sign = SignOf(isSignedInt(ValType(op.Src[0])))
	}
	return op
}

// Sign は命令の符号 (Op.Sign)。
type Sign uint8

const (
	SignNone Sign = iota // 符号で意味の変わらない命令 (lt / div / mod / shift_right でこれなら決め忘れ)
	Unsigned
	Signed
)

// SignOf は signed なら Signed、そうでなければ Unsigned。
func SignOf(signed bool) Sign {
	if signed {
		return Signed
	}
	return Unsigned
}

// IsSigned は符号付きの比較・床除算・算術シフトか。
func (op *Op) IsSigned() bool { return op.Sign == Signed }

func isSignedInt(t *types.Type) bool { return t != nil && t.Signed }
