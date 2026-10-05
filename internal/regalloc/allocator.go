package regalloc

// レジスタ割付 (opt の後、codegen の前)。
// udOrder / registerVars は挿入順を保つ必要がある (割付の結果が順序に依存する)。
// ir.CastedValue は下位の ir.Value と同一キーに合流する (ir.UnderlyingValue)。

import (
	"fmt"
	"slices"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// ---------------------------------------------------------------
// live range の計算
// ---------------------------------------------------------------

type useDefineEntry struct {
	v       *ir.Value
	defines []int
	uses    []int
}

// CalcLiveRange は Fc.calc_live_range 相当。lmd の各ローカル変数に live_range を設定する。
func CalcLiveRange(lmd *ir.Lambda) {
	// ラベルの収集
	labels := map[string]int{}
	for i, op := range lmd.Ops {
		if op != nil && op.Code == ir.OpLabel {
			labels[op.Label] = i
		}
	}

	// 変数の定義・使用、制御フローグラフの集計
	var udOrder []*useDefineEntry
	udIndex := map[*ir.Value]*useDefineEntry{}
	record := func(vAny ir.Operand, isDefine bool, i int) {
		if vAny == nil {
			return
		}
		if ir.ValKind(vAny) != ir.KindLocal {
			return
		}
		if pa, ok := vAny.(*ir.PointeredArray); ok {
			// ローカル配列をポインタとして使う (引数に渡すなど) のは配列そのものの使用
			vAny = pa.From
		}
		v := ir.UnderlyingValue(vAny)
		if v == nil {
			panic(fmt.Sprintf("cannot record %T in use_define", vAny))
		}
		e := udIndex[v]
		if e == nil {
			e = &useDefineEntry{v: v}
			udIndex[v] = e
			udOrder = append(udOrder, e)
		}
		if isDefine {
			e.defines = append(e.defines, i)
		} else {
			e.uses = append(e.uses, i)
		}
	}

	flow := make([][]int, 0, len(lmd.Ops))
	for i, op := range lmd.Ops {
		var node []int
		switch {
		case op.Code.IsBranch():
			node = append(node, labels[op.Label])
		case op.Code == ir.OpSwitch:
			// ジャンプテーブルの飛び先も後続 (無いと飛び先で使う変数の live range が切れて、別の変数と番地を共有していた。
			// fuzz の密な switch で発覚)
			for _, l := range op.Labels {
				node = append(node, labels[l])
			}
		}
		defines, uses := ir.DefUse(op)

		flow = append(flow, node)

		for _, v := range defines {
			// 変数の一部 (struct のフィールド / SoA のバイト分割) への書き込みは残りを保つので、使用でもある。ただしコンパイラの
			// 一時変数で、命令の順で最初に現れるのが部分の書き込みなら (slice を組み立てる最初のフィールド)、それより前に値は無い
			// ので使用にしない (使用にすると関数の頭から生きていることになり、文ごとの slice の一時変数がフレームを共有できな
			// かった)。名前のある変数はループの前の周の値を読むことがあるので今までどおり
			first := false
			if u := ir.UnderlyingValue(v); u != nil && u.LocalType == ir.LTTemp && udIndex[u] == nil {
				first = true
			}
			record(v, true, i)
			if ir.IsPartialDef(v) && !first {
				record(v, false, i)
			}
		}
		for _, v := range uses {
			record(v, false, i)
		}
		if op.Dst != nil && ir.FeedsCarry(lmd.Ops, i) {
			record(op.Dst, false, i+1) // 次の命令が C を読む: 結果が使われなくても消さない (ir.FeedsCarry)
		}
	}

	// 引数は、最初に定義されているものとする
	for _, e := range udOrder {
		if e.v.LocalType == ir.LTArg {
			e.defines = append(e.defines, 0)
		}
	}

	// live range を求める
	lrc := NewLiveRangeCalculator(flow)
	for _, e := range udOrder {
		e.v.LiveRange = lrc.CalcLiveRange(e.defines, e.uses)
	}
}

// ---------------------------------------------------------------
// レジスタ割付
// ---------------------------------------------------------------

// Limits はレジスタ領域の大きさ (バイト)。base.asm の FC_LOCAL / FC_FASTCALL_REG の .res と一致させる。
type Limits struct {
	Reg         int // L (FC_LOCAL): 普通の関数のレジスタ領域。あふれた変数はフレームに置く
	FastcallReg int // FC_FASTCALL_REG: fastcall 関数の引数・戻り値・ローカル・一時変数の全部。あふれたらエラー
}

// StackSize は stack 系の関数のフレームを積む FC_STACK (ゼロページ) の大きさ。フレームは `<S+k,x` で触るので、
// 1 つのフレームがこれを超えると番地がゼロページの外に出る (ld65 の Range error。-O 2 のインライン展開で再帰関数の
// フレームが膨らんで、fuzz で発覚)。
const StackSize = 0x80

// DefaultLimits は既定の大きさ (options(fastcall_reg: N) で FastcallReg を変えられる)。
// FC_FASTCALL_REG は extern の fastcall 関数だけが使う (本体を持つ関数は静的フレーム) ので 16 で足りる。
var DefaultLimits = Limits{Reg: 16, FastcallReg: 16}

// AllocateRegister は Fc.allocate_register 相当。
// 変数の address, location, unuse が設定される。
//
// 置き場所の規則:
//   - 戻り値・引数・呼び出しをまたぐ変数・& を取られた変数・配列・struct → フレーム (fastcall なら FC_FASTCALL_REG の先頭側)
//   - それ以外 → レジスタ領域。live range が重ならない変数はバイト単位で同じ場所を共有する
//   - レジスタ領域に入りきらない変数 → 普通の関数はフレームへあふれさせる (1 サイクル遅いだけ)。
//     fastcall はスタックを使えないので frame size over
func AllocateRegister(lmd *ir.Lambda, lim Limits) {
	if lmd.ABI == ir.ABIStatic {
		allocateStatic(lmd)
		return
	}
	CalcLiveRange(lmd)

	// ref(&演算子)を受けた変数を集める
	refered := map[*ir.Value]bool{}
	for _, op := range lmd.Ops {
		if op != nil && op.Code == ir.OpRef {
			refered[ir.UnderlyingValue(op.Src[0])] = true
		}
	}

	// live range を求め、{callをまたぐ|refを受ける|引数}だったら、フレームスタック上に確保する
	frameSize := lmd.Type.Base.Size // 帰り値分を予約しておく
	var registerVars []*allocEntry  // 挿入順を保つ
	fastcall := lmd.Type.Fastcall()
	frameLoc := ir.LocFrame
	if fastcall {
		frameLoc = ir.LocFastcallReg
	}
	toFrame := func(v *ir.Value) {
		v.Address = frameSize
		v.Location = frameLoc
		frameSize += v.Type.Size
	}
	homes := residentHomes(lmd)
	for _, v := range lmd.Vars {
		if isResident(v) {
			continue // ループ内で A / Y / X に常駐 (AllocateResident が決めた)
		}
		if homes[v] {
			toFrame(v) // 常駐変数の退避先。ループの中では別の名前 (vA) で使うので live range が無くても場所が要る
			continue
		}
		beyondCall := false
		if v.LiveRange != nil {
			for i := v.LiveRange.Min + 1; i <= v.LiveRange.Max-1; i++ {
				if i >= 0 && i < len(lmd.Ops) && lmd.Ops[i] != nil &&
					lmd.Ops[i].Code == ir.OpCall && !ir.ValType(lmd.Ops[i].Src[0]).Fastcall() {
					beyondCall = true
				}
			}
		}

		if v.LocalType == ir.LTResult {
			// 返り値
			lmd.Result.Location = frameLoc
			lmd.Result.Address = 0
		} else if beyondCall || v.LocalType == ir.LTArg || refered[v] ||
			(v.Kind == ir.KindLocal && (v.Type.Kind == types.Array || v.Type.Kind == types.Struct)) {
			// 引数か、関数をまたいでいるか、配列・struct なら、フレームに割り当てる
			toFrame(v)
		} else if v.LiveRange != nil {
			// それ以外の使われてる変数は、レジスターメモリに割り当てる
			registerVars = append(registerVars, &allocEntry{key: v, liveRange: v.LiveRange})
		} else {
			// 未使用フラグをたてる
			v.Location = ir.LocUnused
			v.Unuse = true
		}
	}

	registerVars = allocateCond(lmd, registerVars)
	registerVars = allocateA(lmd, registerVars)

	// レジスタ領域にバイト単位で詰める。fastcall はフレームの後ろの残り、普通の関数は L の全部
	capacity := lim.Reg
	if fastcall {
		capacity = lim.FastcallReg - frameSize
	}
	if capacity < 0 {
		capacity = 0
	}
	packer := newBytePacker(capacity)
	hints := copyHints(lmd, registerVars)
	var spilled []*ir.Value
	regUsed := 0
	for _, e := range registerVars {
		v := e.key
		addr, ok := packer.placeHinted(v, e.liveRange, hints)
		if !ok {
			spilled = append(spilled, v)
			continue
		}
		if fastcall {
			v.Location = ir.LocFastcallReg
			v.Address = frameSize + addr
		} else {
			v.Location = ir.LocReg
			v.Address = addr
		}
		regUsed = max(regUsed, addr+v.Type.Size)
	}
	if fastcall {
		if len(spilled) > 0 {
			need := frameSize + regUsed
			for _, v := range spilled {
				need += v.Type.Size
			}
			panic(&diag.Error{Msg: fmt.Sprintf("frame size over on %s: fastcall function needs about %d bytes of FC_FASTCALL_REG but only %d (%d for arguments/result/arrays); "+
				"split the function, reduce locals, or raise options(fastcall_reg: N)", lmd, need, lim.FastcallReg, frameSize)})
		}
		lmd.ZpUsed = frameSize + regUsed
	} else {
		// 入りきらなかった変数はフレームへ
		for _, v := range spilled {
			toFrame(v)
		}
		lmd.ZpUsed = regUsed
		if frameSize > StackSize {
			panic(&diag.Error{Msg: fmt.Sprintf("frame size over on %s: stack frame needs %d bytes but FC_STACK has %d (split the function or reduce locals)", lmd, frameSize, StackSize)})
		}
	}

	lmd.FrameSize = frameSize
}

// allocateStatic は静的フレーム (Agent/wiki/design/frame-alloc.md §6-2): 戻り値 (0)、引数、アドレスを取られた変数・配列・struct
// (専用の場所)、残りのローカルを live range で詰めたもの、の順に 1 つのフレームに置く。呼び先のフレームは重ならないので
// 呼び出しをまたぐかどうかは関係ない。フレームは 256 バイトまで。
func allocateStatic(lmd *ir.Lambda) {
	CalcLiveRange(lmd)
	refered := map[*ir.Value]bool{}
	for _, op := range lmd.Ops {
		if op != nil && op.Code == ir.OpRef {
			refered[ir.UnderlyingValue(op.Src[0])] = true
		}
	}
	frameSize := lmd.Type.Base.Size
	place := func(v *ir.Value) {
		v.Location = ir.LocStatic
		v.Address = frameSize
		frameSize += v.Type.Size
	}
	for _, v := range lmd.Vars {
		switch v.LocalType {
		case ir.LTResult:
			lmd.Result.Location = ir.LocStatic
			lmd.Result.Address = 0
		case ir.LTArg:
			place(v)
		}
	}
	homes := residentHomes(lmd)
	var packVars []*allocEntry
	for _, v := range lmd.Vars {
		switch {
		case isResident(v):
			// ループ内で A / Y / X に常駐 (AllocateResident が決めた)。退避先は Home
		case v.LocalType == ir.LTResult || v.LocalType == ir.LTArg:
		case homes[v]:
			place(v) // 常駐変数の退避先 (ループの中では vA の名前で使うので live range が途切れる。専用の場所を与える)
		case refered[v] || (v.Kind == ir.KindLocal && (v.Type.Kind == types.Array || v.Type.Kind == types.Struct && !v.Type.IsSlice())):
			place(v) // ポインタで触られうるので他と共有しない (slice はポインタと長さの値で、& を取られなければポインタで触られない)
		case v.LiveRange != nil:
			packVars = append(packVars, &allocEntry{key: v, liveRange: v.LiveRange})
		default:
			v.Location = ir.LocUnused
			v.Unuse = true
		}
	}
	packVars = allocateCond(lmd, packVars)
	packVars = allocateA(lmd, packVars)
	packer := newBytePacker(max(0, 256-frameSize))
	hints := copyHints(lmd, packVars)
	used := 0
	for _, e := range packVars {
		v := e.key
		addr, ok := packer.placeHinted(v, e.liveRange, hints)
		if !ok {
			panic(&diag.Error{Msg: fmt.Sprintf("frame size over on %s: static frame exceeds 256 bytes (split the function or reduce locals)", lmd)})
		}
		v.Location = ir.LocStatic
		v.Address = frameSize + addr
		used = max(used, addr+v.Type.Size)
	}
	lmd.FrameSize = frameSize + used
	lmd.ZpUsed = 0
}

// residentHomes はループ内で A に常駐する変数の退避先 (Home) の集合。
func residentHomes(lmd *ir.Lambda) map[*ir.Value]bool {
	r := map[*ir.Value]bool{}
	for _, v := range lmd.Vars {
		if isResident(v) {
			r[v.Home] = true
		}
	}
	return r
}

// isResident はループ内でレジスタに常駐する一時変数か。
func isResident(v *ir.Value) bool {
	return v.Home != nil && (v.Location == ir.LocA || v.Location == ir.LocY || v.Location == ir.LocX)
}

// bytePacker はレジスタ領域へのバイト単位の詰め込み。バイトごとに、そこを使っている変数の live range を持つ。
type bytePacker struct {
	bytes  [][]*ir.LiveRange
	addrOf map[*ir.Value]int // 置いた変数の位置 (placeHinted)
}

func newBytePacker(capacity int) *bytePacker {
	return &bytePacker{bytes: make([][]*ir.LiveRange, capacity)}
}

// place は size バイトの変数を、live range が重ならない最初の位置に置く。入らなければ ok=false。
func (p *bytePacker) place(size int, lr *ir.LiveRange) (addr int, ok bool) {
	for start := 0; start+size <= len(p.bytes); start++ {
		if p.fits(start, size, lr, nil) {
			p.take(start, size, lr)
			return start, true
		}
	}
	return 0, false
}

// fits は [start, start+size) に lr の変数を置けるか (partner の live range とは、写しの命令で接するだけなので重ならないとみなす)。
func (p *bytePacker) fits(start, size int, lr, partner *ir.LiveRange) bool {
	for i := start; i < start+size; i++ {
		for _, r := range p.bytes[i] {
			if r != partner && overlapRange(r, lr) {
				return false
			}
		}
	}
	return true
}

func (p *bytePacker) take(start, size int, lr *ir.LiveRange) {
	for i := start; i < start+size; i++ {
		p.bytes[i] = append(p.bytes[i], lr)
	}
}

// copyHint は `load d = s` で、s の live range がこの命令で終わり d の live range がこの命令で始まる組 (同じ番地に置けば写しが
// 要らない: codegen.genLoad は同じ番地の写しを出さない)。
type copyHint struct {
	partner *ir.Value
}

// copyHints は詰める変数どうしの写しの組 (両方向)。
func copyHints(lmd *ir.Lambda, vars []*allocEntry) map[*ir.Value][]copyHint {
	in := map[*ir.Value]bool{}
	for _, e := range vars {
		in[e.key] = true
	}
	hints := map[*ir.Value][]copyHint{}
	for i, op := range lmd.Ops {
		if op == nil || op.Code != ir.OpLoad {
			continue
		}
		d, ok1 := op.Dst.(*ir.Value)
		s, ok2 := op.Src[0].(*ir.Value)
		if !ok1 || !ok2 || d == s || !in[d] || !in[s] || d.Type.Size != s.Type.Size || d.LiveRange == nil || s.LiveRange == nil {
			continue
		}
		if s.LiveRange.Max != i || d.LiveRange.Min != i || writesIn(s.LiveRange, d.LiveRange) || writesIn(d.LiveRange, s.LiveRange) {
			continue
		}
		hints[d] = append(hints[d], copyHint{s})
		hints[s] = append(hints[s], copyHint{d})
	}
	return hints
}

// writesIn は a の使われない書き込みが b の live range の中にあるか。
func writesIn(a, b *ir.LiveRange) bool {
	for _, w := range a.Writes {
		if w >= b.Min && w <= b.Max {
			return true
		}
	}
	return false
}

// placeHinted は v を、写しの相手 (hints) がもう置かれていればその番地に (置ければ)、そうでなければ place と同じに置く。
func (p *bytePacker) placeHinted(v *ir.Value, lr *ir.LiveRange, hints map[*ir.Value][]copyHint) (int, bool) {
	if p.addrOf == nil {
		p.addrOf = map[*ir.Value]int{}
	}
	for _, h := range hints[v] {
		if a, ok := p.addrOf[h.partner]; ok && p.fits(a, v.Type.Size, lr, h.partner.LiveRange) {
			p.take(a, v.Type.Size, lr)
			p.addrOf[v] = a
			return a, true
		}
	}
	a, ok := p.place(v.Type.Size, lr)
	if ok {
		p.addrOf[v] = a
	}
	return a, ok
}

// allocateA は Aレジスタを割り当てられるなら割り当てる。
func allocateA(lmd *ir.Lambda, registerVars []*allocEntry) []*allocEntry {
	var aVars []*ir.Value
	for _, e := range registerVars {
		v := e.key
		if v.Type.Size != 1 {
			continue
		}
		if v.LiveRange.Max-v.LiveRange.Min == 1 {
			// Aレジスタを割り当てられる組み合わせでなければスルー
			op := lmd.Ops[v.LiveRange.Min]
			if !isSameValue(op.Dst, v) {
				continue
			}
			if op.Res[ir.RegA].Out {
				continue // ループ内の常駐変数がこの命令の後も A を塞いでいる
			}
			// 結果を A に残す命令 (codegen が storeA で書く) であること
			if !codeIn(op.Code, ir.OpLoad, ir.OpAdd, ir.OpSub, ir.OpAnd, ir.OpOr, ir.OpXor,
				ir.OpMul, ir.OpDiv, ir.OpMod, ir.OpUminus, ir.OpBitNot, ir.OpEq, ir.OpLt, ir.OpLoadMem,
				ir.OpShiftLeft, ir.OpShiftRight, ir.OpRolC, ir.OpRorC,
				ir.OpCall, ir.OpFastcall) { // 呼び出しの 1 バイトの戻り値も最後に A にある (RegResult なら lda 無しで)
				continue
			}
			if codeIn(op.Code, ir.OpShiftLeft, ir.OpShiftRight) {
				// 定数シフトだけ (変数シフトは Y を使うループで A に残らない)
				if _, lit := ir.ValIntLiteral(op.In(1)); !lit {
					continue
				}
			}

			// 直後の命令が最初の入力を最初に A へ読む (loadA) ものであること。他の入力にも同じ変数があると
			// (`l ^ l`) 2 つ目をメモリから読めないので不可 (-O 0 で残る形。fuzz で発覚)
			nextOp := lmd.Ops[v.LiveRange.Min+1]
			uses := 0
			for _, in := range nextOp.Src {
				if isSameValue(in, v) {
					uses++
				}
			}
			if uses > 1 {
				continue
			}
			switch nextOp.Code {
			case ir.OpLoadMem:
				// 添字が A なら tay で Y に写す (codegen の loadYIdx)。添字の無い参照はポインタが A (ずれの無いもの)
				if m := nextOp.Mem(); m.Index != nil {
					if !isSameValue(m.Index, v) {
						continue
					}
				} else if m.Disp != 0 || !isSameValue(m.Base, v) {
					continue
				}
			case ir.OpLoad, ir.OpSignExtension, ir.OpAdd, ir.OpAnd, ir.OpOr, ir.OpXor,
				ir.OpEq, ir.OpLt, ir.OpSub, ir.OpPushArg,
				ir.OpIf, ir.OpIfTrue, ir.OpReturn, ir.OpSwitch:
				if !isSameValue(nextOp.In(0), v) {
					continue
				}
			case ir.OpShiftLeft, ir.OpShiftRight:
				// 定数シフトは In(0) を先に A へ読む (変数シフトは先に In(1) を Y へ読むので A が壊れる)
				if _, lit := ir.ValIntLiteral(nextOp.In(1)); !lit || !isSameValue(nextOp.In(0), v) {
					continue
				}
			case ir.OpStoreMem:
				// 書く値が A (ポインタの準備が A を使うときは codegen が reg に退避する)、または添字 (tay)
				if m := nextOp.Mem(); !isSameValue(nextOp.MemValue(), v) && !(m.Index != nil && isSameValue(m.Index, v)) {
					continue
				}
			default:
				continue
			}

			aVars = append(aVars, v)
		}
	}
	for _, v := range aVars {
		v.Location = ir.LocA
		registerVars = deleteEntry(registerVars, v)
	}
	return registerVars
}

// allocateCond はコンディションレジスタを割り当てられるなら割り当てる。
// OpNot は元の値がフラグのときだけフラグにできるので、命令の順 (live range の始まりの順) に見る (インライン展開した関数の
// $result は変数の並びで計算の一時の値より前にあり、`eq t; not $result t; if_true $result` の $result を先に見てしまう)。
func allocateCond(lmd *ir.Lambda, registerVars []*allocEntry) []*allocEntry {
	var condVars []*ir.Value
	order := slices.Clone(registerVars)
	slices.SortStableFunc(order, func(a, b *allocEntry) int { return a.key.LiveRange.Min - b.key.LiveRange.Min })
	for _, e := range order {
		v := e.key
		if v.Type.Size != 1 {
			continue
		}
		if v.LiveRange.Max-v.LiveRange.Min == 1 {
			op := lmd.Ops[v.LiveRange.Min]
			if !isSameValue(op.Dst, v) {
				continue
			}
			if !codeIn(op.Code, ir.OpEq, ir.OpLt, ir.OpNot) {
				continue
			}

			nextOp := lmd.Ops[v.LiveRange.Min+1]
			switch nextOp.Code {
			case ir.OpIf, ir.OpIfTrue, ir.OpNot:
				if !isSameValue(nextOp.In(0), v) {
					continue
				}
			default:
				continue
			}

			switch op.Code {
			case ir.OpEq:
				v.Location = ir.LocCond
				v.CondPositive = true
				v.CondReg = ir.CondZero
			case ir.OpLt:
				v.Location = ir.LocCond
				v.CondPositive = true
				// codegen の OpLt: 符号なしは C クリア ⇔ 真、符号付きは (V 補正後の) N セット ⇔ 真。サイズによらない
				if op.IsSigned() {
					v.CondReg = ir.CondNegative
				} else {
					v.CondReg = ir.CondCarry
				}
			case ir.OpNot:
				if ir.ValLocation(op.Src[0]) != ir.LocCond {
					continue
				}
				v.Location = ir.LocCond
				v.CondReg = ir.UnderlyingValue(op.Src[0]).CondReg
				v.CondPositive = !ir.UnderlyingValue(op.Src[0]).CondPositive
			default:
				panic("unreachable")
			}
			condVars = append(condVars, v)
		}
	}
	for _, v := range condVars {
		registerVars = deleteEntry(registerVars, v)
	}
	return registerVars
}

func isSameValue(opElem ir.Operand, v *ir.Value) bool {
	// ir.CastedValue は元の値で比べる
	return opElem != nil && ir.UnderlyingValue(opElem) == v
}

func codeIn(c ir.OpCode, codes ...ir.OpCode) bool {
	for _, x := range codes {
		if c == x {
			return true
		}
	}
	return false
}

func deleteEntry(entries []*allocEntry, v *ir.Value) []*allocEntry {
	r := entries[:0]
	for _, e := range entries {
		if e.key != v {
			r = append(r, e)
		}
	}
	return r
}

// ---------------------------------------------------------------
// レジスタアロケータ
// ---------------------------------------------------------------

type allocEntry struct {
	key       *ir.Value
	liveRange *ir.LiveRange
}

func overlapRange(r1, r2 *ir.LiveRange) bool {
	if r1.Max >= r2.Min && r1.Min <= r2.Max {
		return true
	}
	for _, w := range r1.Writes {
		if w >= r2.Min && w <= r2.Max {
			return true
		}
	}
	for _, w := range r2.Writes {
		if w >= r1.Min && w <= r1.Max {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------
// Live Range の計算を行うクラス
// ---------------------------------------------------------------

type LiveRangeCalculator struct {
	flow [][2][]int // [後続節, 先行節]
}

// NewLiveRangeCalculator は flow (各opの後続節リスト) から構築する。
// 直後の節は自動的に後続節に追加される (最後の節を除く)。
func NewLiveRangeCalculator(flow [][]int) *LiveRangeCalculator {
	l := &LiveRangeCalculator{flow: make([][2][]int, len(flow))}
	for i, node := range flow {
		succ := append([]int{}, node...)
		if i < len(flow)-1 {
			succ = append(succ, i+1)
		}
		l.flow[i][0] = succ
	}
	for i := range l.flow {
		for _, succ := range l.flow[i][0] {
			l.flow[succ][1] = append(l.flow[succ][1], i)
		}
	}
	return l
}

// CalcLiveRange は live range を計算する。全く使われていない場合 nil を返す。
func (l *LiveRangeCalculator) CalcLiveRange(defines, uses []int) *ir.LiveRange {
	type liveInfo struct {
		define, use, liveIn, liveOut bool
	}
	lives := make([]liveInfo, len(l.flow))
	for _, i := range defines {
		lives[i].define = true
	}
	for _, i := range uses {
		lives[i].use = true
		lives[i].liveIn = true
	}
	for {
		finished := true
		for i := len(lives) - 1; i >= 0; i-- {
			// 入り口生存なら、先行節で出口生存
			if lives[i].liveIn {
				for _, pred := range l.flow[i][1] {
					if !lives[pred].liveOut {
						lives[pred].liveOut = true
						finished = false
					}
				}
			}
			// 出口生存で定義節でないなら、入り口生存
			if lives[i].liveOut && !lives[i].define {
				if !lives[i].liveIn {
					lives[i].liveIn = true
					finished = false
				}
			}
		}
		if finished {
			break
		}
	}
	// live range の算出
	maxI := 0
	minI := 1000000
	for i := range lives {
		if lives[i].liveIn || lives[i].liveOut {
			if i > maxI {
				maxI = i
			}
			if i < minI {
				minI = i
			}
		}
	}
	if minI == 1000000 {
		return nil
	}
	lr := &ir.LiveRange{Min: minI, Max: maxI}
	for _, i := range defines {
		if i < minI || i > maxI {
			lr.Writes = append(lr.Writes, i) // 使われない書き込み (overlapRange が見る)
		}
	}
	return lr
}

// ---------------------------------------------------------------
// 使っていない変数の削除
// ---------------------------------------------------------------

func DeleteUnuse(lmd *ir.Lambda) {
	for i, op := range lmd.Ops {
		if op == nil {
			continue
		}
		// 副作用のない演算は、結果が使われなければ消す (`c == 32;` のような式文)。
		// 残すと結果の一時変数に場所が割り付かず、コード生成で落ちる。呼び出しは残して戻り値だけ捨てる
		if op.Dst == nil || ir.UnderlyingValue(op.Dst) == nil || !ir.UnderlyingValue(op.Dst).Unuse {
			continue
		}
		switch {
		case op.Code.IsPure():
			ir.DropOp(lmd.Ops, i)
		case op.Code.IsCall():
			op.Dst = nil
		}
	}
}
