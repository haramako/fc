package ir

// 命令の種類ごとの性質の表。
//
// 「副作用が無い」「基本ブロックの終端」「呼び出し」のような命令の分類は、以前は opt / regalloc / codegen / interp の
// 各所が switch で列挙していた (isPure と DeleteUnuse、isBranch と isCondBranch、isCall と isCallOp、直線区間の障壁の
// リスト 5 種など)。同じ集合のはずのものが少しずつ違い、命令を足したときに全部を直す必要があった。
// ここに 1 度だけ書き、各所は OpCode のメソッドで引く。

type opFlags uint16

const (
	// fTerminator は基本ブロックの終端 (この命令の直後はブロックの先頭)。
	fTerminator opFlags = 1 << iota
	// fCondBranch は条件分岐 (Label へ飛ぶか落ちるか)。
	fCondBranch
	// fBranch は Label へ飛びうる命令 (条件分岐・jump。switch は Labels を持つので別扱い)。
	fBranch
	// fPure は副作用が無く、結果 (Dst) が使われなければ消してよい命令。
	fPure
	// fReadsBeforeWrite は「全ての入力を読んでから結果を書く」ことが codegen で保証されている命令 (Dst と Src が同じ場所でもよい)。
	fReadsBeforeWrite
	// fCommutative は 2 入力を入れ替えても結果が変わらない演算。
	fCommutative
	// fCall は関数の呼び出し (call / fastcall)。
	fCall
	// fPushArg は引数を積む命令、fPushResult は戻り値の領域を予約する命令。
	fPushArg
	fPushResult
	// fTouchesGlobals はオペランドに現れないグローバル変数を読み書きしうる命令 (呼び出し・asm・ポインタ経由の書き込み)。
	fTouchesGlobals
	// fUsesCarry は直前の命令が残した C フラグを読む命令 (前の命令との間に他の命令を入れられない)。
	fUsesCarry
	// fOpaque は中身を解析しない命令 (インラインアセンブラ)。
	fOpaque
	// fHasSign は意味が Op.Sign で変わる命令 (符号付きの比較・床除算・算術シフト。sign.go)。
	fHasSign
	// fCompare は比較 (Op.Width が比較の幅。sign.go)。
	fCompare
)

var opInfo = [opCodeCount]opFlags{
	OpLabel:              0,
	OpIf:                 fTerminator | fCondBranch | fBranch,
	OpIfTrue:             fTerminator | fCondBranch | fBranch,
	OpIfCarry:            fTerminator | fCondBranch | fBranch | fUsesCarry,
	OpIfNotCarry:         fTerminator | fCondBranch | fBranch | fUsesCarry,
	OpJump:               fTerminator | fBranch,
	OpSwitch:             fTerminator,
	OpReturn:             fTerminator,
	OpPushResult:         fPushResult,
	OpPushArg:            fPushArg,
	OpCall:               fCall | fTouchesGlobals,
	OpPushFastcallResult: fPushResult,
	OpPushFastcallArg:    fPushArg,
	OpFastcall:           fCall | fTouchesGlobals,
	OpLoad:               fPure | fReadsBeforeWrite,
	OpSignExtension:      fPure | fReadsBeforeWrite,
	OpAdd:                fPure | fReadsBeforeWrite | fCommutative,
	OpSub:                fPure | fReadsBeforeWrite,
	OpAnd:                fPure | fReadsBeforeWrite | fCommutative,
	OpOr:                 fPure | fReadsBeforeWrite | fCommutative,
	OpXor:                fPure | fReadsBeforeWrite | fCommutative,
	OpMul:                fPure | fReadsBeforeWrite,
	OpDiv:                fPure | fReadsBeforeWrite | fHasSign,
	OpMod:                fPure | fReadsBeforeWrite | fHasSign,
	OpShiftLeft:          fPure | fReadsBeforeWrite,
	OpShiftRight:         fPure | fReadsBeforeWrite | fHasSign,
	OpRolC:               fPure | fUsesCarry,
	OpRorC:               fPure | fUsesCarry,
	OpUminus:             fPure | fReadsBeforeWrite,
	OpEq:                 fPure | fReadsBeforeWrite | fCommutative | fCompare,
	OpLt:                 fPure | fReadsBeforeWrite | fHasSign | fCompare,
	OpNot:                fPure | fReadsBeforeWrite,
	OpBitNot:             fPure | fReadsBeforeWrite,
	OpAsm:                fOpaque | fTouchesGlobals,
	OpIndex:              fPure | fReadsBeforeWrite,
	OpRef:                fPure,
	OpLoadMem:            fPure | fReadsBeforeWrite,
	OpStoreMem:           fTouchesGlobals,
}

func (c OpCode) has(f opFlags) bool { return opInfo[c]&f != 0 }

// IsTerminator は基本ブロックの終端 (条件分岐・jump・switch・return) か。
func (c OpCode) IsTerminator() bool { return c.has(fTerminator) }

// IsCondBranch は条件分岐 (if / if_true / if_carry / if_not_carry) か。
func (c OpCode) IsCondBranch() bool { return c.has(fCondBranch) }

// IsBranch は Label へ飛びうる命令 (条件分岐・jump) か。switch は含まない (Labels を見る)。
func (c OpCode) IsBranch() bool { return c.has(fBranch) }

// IsBlockBoundary はラベルか終端か: 直線の区間 (基本ブロックの中) を辿るときの境界。
func (c OpCode) IsBlockBoundary() bool { return c == OpLabel || c.has(fTerminator) }

// IsPure は副作用が無く、結果が使われなければ消してよい命令か。
func (c OpCode) IsPure() bool { return c.has(fPure) }

// ReadsBeforeWrite は全ての入力を読んでから結果を書く命令か (Dst と Src が同じ場所でもよい)。
func (c OpCode) ReadsBeforeWrite() bool { return c.has(fReadsBeforeWrite) }

// IsCommutative は 2 入力を入れ替えられる演算か。
func (c OpCode) IsCommutative() bool { return c.has(fCommutative) }

// IsCall は関数の呼び出し (call / fastcall) か。
func (c OpCode) IsCall() bool { return c.has(fCall) }

// IsPushArg は引数を積む命令 (push_arg / push_fastcall_arg) か。
func (c OpCode) IsPushArg() bool { return c.has(fPushArg) }

// IsPushResult は戻り値の領域を予約する命令 (push_result / push_fastcall_result) か。
func (c OpCode) IsPushResult() bool { return c.has(fPushResult) }

// MayTouchGlobals はオペランドに現れないグローバル変数を読み書きしうる命令か。
func (c OpCode) MayTouchGlobals() bool { return c.has(fTouchesGlobals) }

// UsesCarry は直前の命令が残した C フラグを読む命令か (rolc / rorc / if_carry / if_not_carry)。
func (c OpCode) UsesCarry() bool { return c.has(fUsesCarry) }

// FeedsCarry は ops[i] の次の命令 (nil を飛ばす) が ops[i] の残した C を読むか。その命令の結果が使われなくても、
// C のために消せない (16 ビットの >> 1 を分けた `shift_right hi; rorc lo` で、使われない hi の shift を消して、rorc が
// 前の式の C を拾っていた。fuzz の TestRandomConstFold で発覚)。
func FeedsCarry(ops []*Op, i int) bool {
	for j := i + 1; j < len(ops); j++ {
		if ops[j] != nil {
			return ops[j].Code.UsesCarry() && !ops[i].Code.IsBranch()
		}
	}
	return false
}

// IsOpaque は中身を解析しない命令 (インラインアセンブラ) か。
func (c OpCode) IsOpaque() bool { return c.has(fOpaque) }

// HasSign は意味が Op.Sign で変わる命令 (lt / div / mod / shift_right) か。
func (c OpCode) HasSign() bool { return c.has(fHasSign) }

// IsCompare は比較 (eq / lt。Op.Width が比較の幅) か。
func (c OpCode) IsCompare() bool { return c.has(fCompare) }

// 以下は *Op を受ける形 (nil なら false)。命令列を i+1 のように覗くところで nil の穴を気にせず書ける。

// IsCondBranch は op が条件分岐か。
func IsCondBranch(op *Op) bool { return op != nil && op.Code.IsCondBranch() }

// IsBranch は op が条件分岐か jump か。
func IsBranch(op *Op) bool { return op != nil && op.Code.IsBranch() }

// IsCall は op が call / fastcall か。
func IsCall(op *Op) bool { return op != nil && op.Code.IsCall() }

// IsPushArg は op が push_arg / push_fastcall_arg か。
func IsPushArg(op *Op) bool { return op != nil && op.Code.IsPushArg() }

// IsPushResult は op が push_result / push_fastcall_result か。
func IsPushResult(op *Op) bool { return op != nil && op.Code.IsPushResult() }

// MayTouchGlobals は op が (オペランドに現れない) グローバル変数を読み書きしうるか。
func MayTouchGlobals(op *Op) bool { return op != nil && op.Code.MayTouchGlobals() }
