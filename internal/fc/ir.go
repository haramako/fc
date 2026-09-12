package fc

// 中間表現 (IR) の型定義 (doc/v2_plan.md R1-d)。
//
// HLC が生成し、レジスタ割付 (allocator.go) とコード生成 (llc.go) が消費する。
// 旧実装では命令は []any (先頭が Sym の opcode、以降が位置引数) だった。ここでは
// opcode を enum、オペランドを意味ごとのフィールド (Dst / Src / Label / Type / Text) に分ける。
// ダンプ (irdump.go) は旧形式と同じ位置引数の並びで出力するので golden は変わらない。

import "fmt"

// OpCode は IR 命令の種類。
type OpCode uint8

const (
	opInvalid OpCode = iota
	OpLabel                 // Label:
	OpIf                    // if Src[0] == 0 then goto Label
	OpJump                  // goto Label
	OpReturn                // return [Src[0]]
	OpPushResult            // 戻り値領域を予約 (Type)
	OpPushArg               // 引数を積む (Type, Src[0])
	OpCall                  // Dst = call Src[0]
	OpPushFastcallResult    // fastcall 版
	OpPushFastcallArg       //
	OpFastcall              //
	OpLoad                  // Dst = Src[0]
	OpSignExtension         // Dst = sign_extend(Src[0])
	OpAdd                   // Dst = Src[0] op Src[1]
	OpSub                   //
	OpAnd                   //
	OpOr                    //
	OpXor                   //
	OpMul                   //
	OpDiv                   //
	OpMod                   //
	OpShiftLeft             //
	OpShiftRight            //
	OpUminus                // Dst = -Src[0]
	OpEq                    // Dst = (Src[0] == Src[1])
	OpLt                    // Dst = (Src[0] < Src[1])
	OpNot                   // Dst = !Src[0]
	OpAsm                   // インラインアセンブラ (Text)
	OpIndex                 // Dst = &Src[0][Src[1]]
	OpRef                   // Dst = &Src[0]
	OpPget                  // Dst = *Src[0]
	OpPset                  // *Src[0] = Src[1]
	OpIndexPget             // Dst = Src[0][Src[1]]        (ピープホール最適化で生成)
	OpIndexPset             // Src[0][Src[1]] = Src[2]     (同上)
	opCodeCount
)

var opCodeNames = [...]string{
	OpLabel: "label", OpIf: "if", OpJump: "jump", OpReturn: "return",
	OpPushResult: "push_result", OpPushArg: "push_arg", OpCall: "call",
	OpPushFastcallResult: "push_fastcall_result", OpPushFastcallArg: "push_fastcall_arg", OpFastcall: "fastcall",
	OpLoad: "load", OpSignExtension: "sign_extension",
	OpAdd: "add", OpSub: "sub", OpAnd: "and", OpOr: "or", OpXor: "xor",
	OpMul: "mul", OpDiv: "div", OpMod: "mod", OpShiftLeft: "shift_left", OpShiftRight: "shift_right",
	OpUminus: "uminus", OpEq: "eq", OpLt: "lt", OpNot: "not", OpAsm: "asm",
	OpIndex: "index", OpRef: "ref", OpPget: "pget", OpPset: "pset",
	OpIndexPget: "index_pget", OpIndexPset: "index_pset",
}

// String は旧 IR の opcode 名 (Sym の綴り) を返す。
func (c OpCode) String() string {
	if c > opInvalid && c < opCodeCount {
		return opCodeNames[c]
	}
	return fmt.Sprintf("OpCode(%d)", int(c))
}

// copToOpCode は HLC 内部の演算名 (cexpr.go) から OpCode への対応。
var copToOpCode = map[cop]OpCode{
	opLoad: OpLoad, opAdd: OpAdd, opSub: OpSub, opMul: OpMul, opDiv: OpDiv, opMod: OpMod,
	opAnd: OpAnd, opOr: OpOr, opXor: OpXor, opShiftLeft: OpShiftLeft, opShiftRight: OpShiftRight,
	opNot: OpNot, opUminus: OpUminus, opEq: OpEq, opLt: OpLt,
}

// Operand は IR 命令の値オペランド。*Value / *CastedValue / *PointeredArray が実装する。
type Operand interface {
	operandNode()
}

func (*Value) operandNode()          {}
func (*CastedValue) operandNode()    {}
func (*PointeredArray) operandNode() {}

// Op は IR の 1 命令。使うフィールドは OpCode ごとに決まっている (OpCode 定義のコメント参照)。
type Op struct {
	Code  OpCode
	Dst   Operand   // 結果の格納先 (無い命令、または削除された戻り値では nil)
	Src   []Operand // 入力
	Label string    // OpLabel / OpIf / OpJump の飛び先
	Type  *Type     // OpPushResult / OpPushArg / OpPushFastcall* の型
	Text  string    // OpAsm のアセンブラ行
}

// src は i 番目の入力 (無ければ nil)。
func (op *Op) src(i int) Operand {
	if i < len(op.Src) {
		return op.Src[i]
	}
	return nil
}

// positional は旧 IR ([]any) と同じ位置引数の並びを返す (ダンプ用)。
func (op *Op) positional() []any {
	r := []any{Sym(op.Code.String())}
	switch op.Code {
	case OpLabel, OpJump:
		r = append(r, op.Label)
	case OpIf:
		r = append(r, op.Src[0], op.Label)
	case OpReturn:
		if len(op.Src) > 0 {
			r = append(r, op.Src[0])
		}
	case OpPushResult, OpPushFastcallResult:
		r = append(r, op.Type)
	case OpPushArg, OpPushFastcallArg:
		r = append(r, op.Type, op.Src[0])
	case OpAsm:
		r = append(r, op.Text)
	case OpPset, OpIndexPset:
		for _, s := range op.Src {
			r = append(r, s)
		}
	default:
		// Dst を持つ命令 (call / fastcall の Dst は nil になりうる)
		r = append(r, op.Dst)
		for _, s := range op.Src {
			r = append(r, s)
		}
	}
	return r
}

// ValueKind は Value の種類。
type ValueKind uint8

const (
	KindLocal        ValueKind = iota + 1
	KindGlobal                 // グローバル変数 / 定数配列 / 関数 / モジュール束縛
	KindLiteral                // 整数リテラル / 関数シンボル
	KindArrayLiteral           // 配列リテラル (Def に落とす前)
	KindModule                 // モジュール
)

var valueKindNames = [...]string{
	KindLocal: "local", KindGlobal: "global", KindLiteral: "literal",
	KindArrayLiteral: "array_literal", KindModule: "module",
}

func (k ValueKind) String() string {
	if int(k) < len(valueKindNames) && k > 0 {
		return valueKindNames[k]
	}
	return fmt.Sprintf("ValueKind(%d)", int(k))
}

// Location はレジスタ割付後の変数の置き場所。
type Location uint8

const (
	LocNone        Location = iota // 未割付 (ダンプでは nil)
	LocFrame                       // スタックフレーム上 (S+addr,x)
	LocReg                         // レジスタメモリ (L+addr)
	LocMem                         // (未使用)
	LocUnused                      // 使われていない変数
	LocA                           // A レジスタ
	LocCond                        // コンディションフラグ (CondReg)
	LocFastcallReg                 // fastcall 用レジスタ (FC_FASTCALL_REG+addr)
)

var locationNames = [...]string{
	LocNone: "", LocFrame: "frame", LocReg: "reg", LocMem: "mem", LocUnused: "none",
	LocA: "a", LocCond: "cond", LocFastcallReg: "fastcall_reg",
}

func (l Location) String() string {
	if int(l) < len(locationNames) {
		return locationNames[l]
	}
	return fmt.Sprintf("Location(%d)", int(l))
}

// CondReg は LocCond のときどのフラグに結果があるか。
type CondReg uint8

const (
	CondNone CondReg = iota
	CondZero
	CondCarry
	CondNegative
)

var condRegNames = [...]string{CondNone: "", CondZero: "zero", CondCarry: "carry", CondNegative: "negative"}

func (c CondReg) String() string {
	if int(c) < len(condRegNames) {
		return condRegNames[c]
	}
	return fmt.Sprintf("CondReg(%d)", int(c))
}

// DefKind はモジュール/関数に属する定義の種類。
type DefKind uint8

const (
	DefEqu   DefKind = iota + 1 // シンボル = 値
	DefBss                      // 未初期化領域
	DefBlock                    // 定数データブロック
	DefCode                     // 関数
)

var defKindNames = [...]string{DefEqu: "equ", DefBss: "bss", DefBlock: "block", DefCode: "code"}

func (k DefKind) String() string {
	if int(k) < len(defKindNames) && k > 0 {
		return defKindNames[k]
	}
	return fmt.Sprintf("DefKind(%d)", int(k))
}
