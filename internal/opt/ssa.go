package opt

// SSA 形式による定数伝播 / コピー伝播 / 死んだ定義の除去 (Agent/wiki/design/ssa.md)。
//
// IR そのものは「変数 + 命令列」のまま変えない。関数の CFG の上に Braun らの方法 (Simple and Efficient Construction of
// SSA Form, CC 2013) で各変数の「命令ごとの版」(ssaVal) と φ を作り、それを使って命令列を書き換える:
//
//   - 版が定数なら使用位置をリテラルに (定数は定義の演算を畳んで求める。φ は全ての入力が同じ定数のとき)
//   - 版が `load x = y` のコピーで、使用位置での y の版が同じなら、使用位置を y に
//   - どこからも使われない版の定義 (副作用の無い命令) を消す
//
// 書き換えのたびに SSA を作り直す (関数は小さいので十分速い)。対象はスカラのローカル変数だけ:
// 一部だけ書かれる (struct のフィールド、2 バイトの半分) 変数、アドレスを取られた変数、配列、戻り値は対象外。

import (
	"strings"

	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// ssaVal は変数の 1 つの版: 命令による定義、φ、関数の入口 (引数)、未定義のいずれか。
type ssaVal struct {
	v     *ir.Value
	def   int       // 定義した命令の添字 (φ / 入口 / 未定義は -1)
	phi   bool      //
	block *ir.Block // φ のブロック
	args  []*ssaVal // φ の入力 (block.Preds と同じ並び。作りかけの φ では空)
	users []*ssaVal // この版を入力に持つ φ
	repl  *ssaVal   // 自明な φ だったとき、代わりになる版
	undef bool      // 定義される前の読み出し

	kstate uint8 // 定数の評価状態 (kUnknown / kBusy / kConst / kVar)
	bits   int   // kConst のときの値 (変数のサイズに切り詰めたビット列。符号は読む側の型で解釈する)
}

const (
	kUnknown uint8 = iota
	kBusy
	kConst
	kVar
)

type ssaForm struct {
	lmd        *ir.Lambda
	cfg        *ir.CFG
	vars       map[*ir.Value]bool // 対象の変数
	blockOf    []*ir.Block        // 命令 → ブロック (到達できない命令は nil)
	jumped     map[string]bool    // 飛び先になるラベル (untouched が作る)
	frozenVars map[*ir.Value]bool // どこからも書かれないローカル (frozen が作る)

	cur        map[*ir.Block]map[*ir.Value]*ssaVal // ブロック内の最後の定義
	entry      map[*ir.Block]map[*ir.Value]*ssaVal // ブロックの入口の版
	incomplete map[*ir.Block]map[*ir.Value]*ssaVal // 封印前に作った φ
	sealed     map[*ir.Block]bool
	filled     map[*ir.Block]bool
	reach      map[*ir.Block]bool

	useAt map[int][]*ssaVal // 命令の各入力 (Src の添字順) の版。対象外の入力は nil
	defAt map[int]*ssaVal   // 命令が定義する版
	undef *ssaVal
	ud    *ir.UseDef // volatileOnce が作る (simplify の間だけ。命令を置き換えても入力の変数は変わらない)
}

// propagateSSA は定数 / コピー伝播と死んだ定義の除去を、変化が無くなるまで繰り返す。
func propagateSSA(lmd *ir.Lambda) {
	untilFixed(lmd, "ssa", func() bool {
		s := buildSSA(lmd)
		if s == nil {
			return false
		}
		changed := s.rewrite()
		if s.simplify() {
			changed = true
		}
		s = buildSSA(lmd)
		if s == nil {
			return false
		}
		if s.eliminateDead() {
			changed = true
		}
		return changed
	})
}

// ssaCandidate は対象になりうる変数か (スカラのローカル変数。戻り値は return が暗黙に読むので除く)。
func ssaCandidate(v *ir.Value, lmd *ir.Lambda) bool {
	if v == nil || v.Kind != ir.KindLocal || v.LocalType == ir.LTResult || v == lmd.Result {
		return false
	}
	switch v.Type.Kind {
	case types.Int, types.Bool, types.Pointer, types.Func:
		return true
	}
	return false
}

// buildSSA は関数の SSA 形式を作る (対象の変数が無ければ nil)。
func buildSSA(lmd *ir.Lambda) *ssaForm {
	s := &ssaForm{
		lmd:        lmd,
		vars:       map[*ir.Value]bool{},
		cur:        map[*ir.Block]map[*ir.Value]*ssaVal{},
		entry:      map[*ir.Block]map[*ir.Value]*ssaVal{},
		incomplete: map[*ir.Block]map[*ir.Value]*ssaVal{},
		sealed:     map[*ir.Block]bool{},
		filled:     map[*ir.Block]bool{},
		useAt:      map[int][]*ssaVal{},
		defAt:      map[int]*ssaVal{},
		undef:      &ssaVal{def: -1, undef: true, kstate: kVar},
	}
	// 対象の変数: 候補のうち、一部への書き込み・アドレス取得・配列としての参照が無いもの
	excluded := map[*ir.Value]bool{}
	for _, op := range lmd.Ops {
		if op == nil {
			continue
		}
		defs, uses := ir.DefUse(op)
		for _, d := range defs {
			v := ir.UnderlyingValue(d)
			if ir.IsPartialDef(d) {
				excluded[v] = true
			} else if ssaCandidate(v, lmd) {
				s.vars[v] = true
			}
		}
		if op.Code == ir.OpRef {
			excluded[ir.UnderlyingValue(op.Src[0])] = true
		}
		for _, u := range uses {
			if pa, ok := u.(*ir.PointeredArray); ok {
				excluded[ir.UnderlyingValue(pa.From)] = true
			} else if v := ir.UnderlyingValue(u); ssaCandidate(v, lmd) {
				s.vars[v] = true
			}
		}
	}
	for v := range excluded {
		delete(s.vars, v)
	}
	if len(s.vars) == 0 {
		return nil
	}
	s.cfg = ir.BuildCFG(lmd)
	s.reach = s.cfg.Reachable()
	s.blockOf = make([]*ir.Block, len(lmd.Ops))
	for _, b := range s.cfg.Blocks {
		if !s.reach[b] {
			continue
		}
		for _, i := range s.cfg.Ops(b) {
			s.blockOf[i] = b
		}
	}
	// 逆後順にブロックを埋める。全ての (到達できる) 先行ブロックが埋まったブロックを封印する
	var post []*ir.Block
	seen := map[*ir.Block]bool{}
	var walk func(b *ir.Block)
	walk = func(b *ir.Block) {
		seen[b] = true
		for _, x := range b.Succs {
			if !seen[x] {
				walk(x)
			}
		}
		post = append(post, b)
	}
	walk(s.cfg.Blocks[0])
	for i := len(post) - 1; i >= 0; i-- {
		b := post[i]
		s.trySeal(b)
		s.fill(b)
		for _, x := range b.Succs {
			s.trySeal(x)
		}
	}
	for _, b := range post {
		s.trySeal(b)
	}
	return s
}

func (s *ssaForm) preds(b *ir.Block) []*ir.Block {
	var r []*ir.Block
	for _, p := range b.Preds {
		if s.reach[p] {
			r = append(r, p)
		}
	}
	return r
}

func (s *ssaForm) trySeal(b *ir.Block) {
	if s.sealed[b] {
		return
	}
	for _, p := range s.preds(b) {
		if !s.filled[p] {
			return
		}
	}
	s.sealed[b] = true
	for v, phi := range s.incomplete[b] {
		s.addPhiOperands(v, phi)
	}
	delete(s.incomplete, b)
}

// fill はブロックの命令を順に見て、使用の版を記録し、定義で新しい版を作る。
func (s *ssaForm) fill(b *ir.Block) {
	for _, i := range s.cfg.Ops(b) {
		op := s.lmd.Ops[i]
		defs, uses := ir.DefUse(op)
		if len(uses) > 0 {
			us := make([]*ssaVal, len(uses))
			for k, u := range uses {
				if v := ir.UnderlyingValue(u); v != nil && s.vars[v] {
					us[k] = s.readVariable(v, b)
				}
			}
			s.useAt[i] = us
		}
		for _, d := range defs {
			if v := ir.UnderlyingValue(d); v != nil && s.vars[v] {
				val := &ssaVal{v: v, def: i}
				s.defAt[i] = val
				s.writeVariable(v, b, val)
			}
		}
	}
	s.filled[b] = true
}

func (s *ssaForm) writeVariable(v *ir.Value, b *ir.Block, val *ssaVal) {
	m := s.cur[b]
	if m == nil {
		m = map[*ir.Value]*ssaVal{}
		s.cur[b] = m
	}
	m[v] = val
}

// readVariable はブロックの出口 (埋めている途中ならその時点) での v の版。
func (s *ssaForm) readVariable(v *ir.Value, b *ir.Block) *ssaVal {
	if val := s.cur[b][v]; val != nil {
		return val
	}
	return s.entryValue(v, b)
}

// entryValue はブロックの入口での v の版 (先行ブロックから求める。必要なら φ を作る)。
func (s *ssaForm) entryValue(v *ir.Value, b *ir.Block) *ssaVal {
	if val := s.entry[b][v]; val != nil {
		return val
	}
	var val *ssaVal
	preds := s.preds(b)
	switch {
	case !s.sealed[b]:
		val = &ssaVal{v: v, def: -1, phi: true, block: b}
		if s.incomplete[b] == nil {
			s.incomplete[b] = map[*ir.Value]*ssaVal{}
		}
		s.incomplete[b][v] = val
	case len(preds) == 0:
		val = s.undef
		for _, a := range s.lmd.Args {
			if a == v {
				val = &ssaVal{v: v, def: -1, kstate: kVar}
				break
			}
		}
	case len(preds) == 1:
		val = s.readVariable(v, preds[0])
	default:
		val = &ssaVal{v: v, def: -1, phi: true, block: b}
		s.setEntry(v, b, val) // ループで自分に戻ってきたときにこの φ を読む
		val = s.addPhiOperands(v, val)
	}
	s.setEntry(v, b, val)
	return val
}

func (s *ssaForm) setEntry(v *ir.Value, b *ir.Block, val *ssaVal) {
	m := s.entry[b]
	if m == nil {
		m = map[*ir.Value]*ssaVal{}
		s.entry[b] = m
	}
	m[v] = val
}

func (s *ssaForm) addPhiOperands(v *ir.Value, phi *ssaVal) *ssaVal {
	for _, p := range s.preds(phi.block) {
		a := resolve(s.readVariable(v, p))
		phi.args = append(phi.args, a)
		a.users = append(a.users, phi)
	}
	return s.tryRemoveTrivialPhi(phi)
}

// tryRemoveTrivialPhi は入力が (自分自身を除いて) 1 種類しかない φ をその入力で置き換える。
// 未定義の入力は無視しない: φ(未定義, v) を v にすると、v が φ を支配しないとき (ループの先頭の φ で、v がループの中の
// 代入) に、ループの後の読み出しが v の版 (`m = i` の i) の写しとして伝わって値が変わっていた (`var m; for (…) { m = i; }`
// の後の m が -O 2 で最後の i ではなくループを抜けた i。survey 2026-09-27)。定数の評価 (evalConst) は未定義を
// どの値でもよいとして無視してよい (値の同一性でなく定数だけを伝えるため)。
func (s *ssaForm) tryRemoveTrivialPhi(phi *ssaVal) *ssaVal {
	var same *ssaVal
	for _, a := range phi.args {
		a = resolve(a)
		if a == same || a == phi {
			continue
		}
		if same != nil {
			return phi
		}
		same = a
	}
	if same == nil {
		same = s.undef
	}
	phi.repl = same
	users := phi.users
	phi.users = nil
	for _, u := range users {
		if u != phi {
			same.users = append(same.users, u)
		}
	}
	for _, u := range users {
		if u != phi && u.repl == nil && len(u.args) > 0 {
			s.tryRemoveTrivialPhi(u)
		}
	}
	return same
}

// resolve は置き換えられた φ を辿る。
func resolve(v *ssaVal) *ssaVal {
	for v.repl != nil {
		if v.repl.repl != nil {
			v.repl = v.repl.repl
		}
		v = v.repl
	}
	return v
}

// valueAt は命令 i の直前での v の版。
func (s *ssaForm) valueAt(v *ir.Value, i int) *ssaVal {
	b := s.blockOf[i]
	for j := i - 1; j >= b.Start; j-- {
		if d := s.defAt[j]; d != nil && d.v == v {
			return resolve(d)
		}
	}
	return resolve(s.entryValue(v, b))
}

// ---------------------------------------------------------------
// 定数
// ---------------------------------------------------------------

// normBits はビット列 bits を size バイトの整数として読む (signed なら符号拡張)。
func normBits(bits int, size int, signed bool) int {
	if size <= 0 || size >= 8 {
		return bits
	}
	mask := 1<<(8*uint(size)) - 1
	bits &= mask
	if signed && bits>>(8*uint(size)-1) != 0 {
		bits -= mask + 1
	}
	return bits
}

// normInt は型 t の整数として読む。
func normInt(bits int, t *types.Type) int { return normBits(bits, t.Size, t.Signed) }

func bitsOf(n int, size int) int {
	if size <= 0 || size >= 8 {
		return n
	}
	return n & (1<<(8*uint(size)) - 1)
}

func isIntLike(t *types.Type) bool { return t.Kind == types.Int || t.Kind == types.Bool }

// operandBits は命令 i の入力 k のビット列 (整数型のリテラル、または定数の版のとき)。
// リテラルはその整数値そのもの (codegen はリテラルの型に関係なく値のバイトを取り出す: 255 を sint8 の演算に渡すと -1、
// -1 を 16 ビットの演算に渡すと $ffff)。変数は自分のサイズに切り詰めたビット列 (それより広い演算ではゼロ拡張される。
// 符号付きの変数を広げるときは sema が sign_extension を挟むので、暗黙に広がるのは符号なしだけ)。
// CastedValue は Offset バイト目からの読み替え。
func (s *ssaForm) operandBits(i, k int) (int, bool) {
	o := s.lmd.Ops[i].Src[k]
	t := ir.ValType(o)
	if !isIntLike(t) {
		return 0, false
	}
	if n, ok := ir.ValIntLiteral(o); ok {
		return castBits(o, n), true
	}
	us := s.useAt[i]
	if k >= len(us) || us[k] == nil {
		return 0, false
	}
	val := resolve(us[k])
	if !s.konst(val) {
		return 0, false
	}
	return castBits(o, val.bits), true
}

// castBits は値のビット列 base (変数ならそのサイズに切り詰めたもの、リテラルならその整数値) を o の cast で読み替えた
// ビット列: Offset バイト目から Width バイトを取ってゼロ拡張する (`((l0 as int) as sint16)` は l0 の下位バイト。
// 入れ子の cast の切り詰めは ir.NewCastedValue が Width に畳んでいる)。
func castBits(o ir.Operand, base int) int {
	cv, ok := o.(*ir.CastedValue)
	if !ok {
		return base
	}
	return bitsOf(base>>(8*uint(cv.Offset)), cv.Width)
}

// operandInt は入力 k の値をその型で読んだもの。
func (s *ssaForm) operandInt(i, k int) (int, bool) {
	bits, ok := s.operandBits(i, k)
	if !ok {
		return 0, false
	}
	return normInt(bits, ir.ValType(s.lmd.Ops[i].Src[k])), true
}

// konst は版が定数か (定義の演算を畳む。φ は全ての入力が同じ定数のとき)。
func (s *ssaForm) konst(val *ssaVal) bool {
	val = resolve(val)
	switch val.kstate {
	case kConst:
		return true
	case kVar, kBusy:
		return false
	}
	val.kstate = kBusy
	bits, ok := s.evalConst(val)
	if ok {
		val.kstate, val.bits = kConst, bits
	} else {
		val.kstate = kVar
	}
	return ok
}

// evalConst は版の値を定義の演算から求める。入力の読み方 (幅と符号) は codegen と同じにする:
// 加減算・論理演算・乗算は結果の幅のビット列だけで決まる。除算・剰余は結果の型の符号、シフトは第 1 入力の符号、
// 比較は「どちらかが符号付きなら符号付き」で幅は広い方。
func (s *ssaForm) evalConst(val *ssaVal) (int, bool) {
	if val.phi {
		have := false
		var bits int
		for _, a := range val.args {
			a = resolve(a)
			if a == val || a.undef {
				continue
			}
			if !s.konst(a) || (have && a.bits != bits) {
				return 0, false
			}
			have, bits = true, a.bits
		}
		return bits, have
	}
	if val.def < 0 {
		return 0, false
	}
	op := s.lmd.Ops[val.def]
	dt := ir.ValType(op.Dst)
	if !isIntLike(dt) {
		return 0, false
	}
	size := val.v.Type.Size
	t0 := ir.ValType(op.Src[0])
	a, ok := s.operandBits(val.def, 0)
	if !ok {
		return 0, false
	}
	switch op.Code {
	case ir.OpLoad:
		return bitsOf(a, size), true
	case ir.OpSignExtension:
		return bitsOf(normBits(a, 1, true), size), true // 下位 1 バイトの符号拡張 (codegen・interp と同じく入力の型を見ない)
	case ir.OpNot:
		return bitsOf(b2i(bitsOf(a, t0.Size) == 0), size), true
	case ir.OpBitNot:
		return bitsOf(^a, size), true
	case ir.OpUminus:
		return bitsOf(-a, size), true
	}
	b, ok := s.operandBits(val.def, 1)
	if !ok {
		return 0, false
	}
	t1 := ir.ValType(op.Src[1])
	switch op.Code {
	case ir.OpAdd:
		return bitsOf(a+b, size), true
	case ir.OpSub:
		return bitsOf(a-b, size), true
	case ir.OpAnd:
		return bitsOf(a&b, size), true
	case ir.OpOr:
		return bitsOf(a|b, size), true
	case ir.OpXor:
		return bitsOf(a^b, size), true
	case ir.OpMul:
		return bitsOf(a*b, size), true
	case ir.OpDiv, ir.OpMod:
		a, b = normBits(a, dt.Size, op.IsSigned()), normBits(b, dt.Size, op.IsSigned()) // Dst の幅で、命令の符号 (ir/sign.go)
		if b == 0 {
			return 0, false
		}
		if op.Code == ir.OpDiv {
			return bitsOf(ir.FloorDiv(a, b), size), true
		}
		return bitsOf(ir.FloorMod(a, b), size), true
	case ir.OpShiftLeft, ir.OpShiftRight:
		a, b = normBits(a, dt.Size, op.IsSigned()), normInt(b, t1) // 右シフトの符号は命令の (左シフトは下位が同じなので問わない)
		if b < 0 || b > 64 {
			return 0, false
		}
		if op.Code == ir.OpShiftLeft {
			return bitsOf(ir.Shl(a, b), size), true
		}
		return bitsOf(ir.Shr(a, b), size), true
	case ir.OpEq, ir.OpLt:
		w, signed := op.Width, op.IsSigned() // 比較の幅と符号は命令の (ir/sign.go)
		a, b = normBits(a, w, signed), normBits(b, w, signed)
		if op.Code == ir.OpEq {
			return bitsOf(b2i(a == b), size), true
		}
		return bitsOf(b2i(a < b), size), true
	}
	return 0, false
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------
// 書き換え
// ---------------------------------------------------------------

// rewrite は定数とコピーを使用位置に伝播し、定数になった定義と分岐を畳む。
func (s *ssaForm) rewrite() bool {
	changed := false
	ops := s.lmd.Ops
	var dels []int           // ループの後で消す (ループ中は他の版の定義として参照されうる)
	var logLive *ir.Liveness // @log の引数の書き換えに使う生存 (注釈がある関数だけ)
	if ir.HasLogs(ops) {
		logLive = ir.BuildLiveness(s.lmd)
	}
	for i, op := range ops {
		if op == nil || s.blockOf[i] == nil {
			continue
		}
		if logLive != nil {
			s.rewriteLogs(i, op, logLive)
		}
		for k, val := range s.useAt[i] {
			if val == nil {
				continue
			}
			o := op.Src[k]
			if n, ok := s.operandInt(i, k); ok {
				if _, lit := ir.ValIntLiteral(o); lit {
					continue
				}
				op.Src[k] = ir.NewIntLiteral("", ir.ValType(o), n)
				changed = true
				continue
			}
			if y := s.copySource(resolve(val), i); y != nil {
				op.Src[k] = rebase(o, y)
				changed = true
				continue
			}
			if y := s.highByteOf(resolve(val), o, i); y != nil {
				op.Src[k] = y
				changed = true
				continue
			}
			if y := s.frozenSource(resolve(val), o); y != nil {
				op.Src[k] = y
				changed = true
			}
		}
		// 番地が変数の参照 (`&g`: inline した関数に渡した struct のポインタ) のメモリの読み書きは、その変数の直の読み書きに
		if op.IsMem() && op.Src[1] == ir.Operand(ir.NoIndex) && len(s.useAt[i]) > 0 && s.useAt[i][0] != nil {
			if nop := s.directMem(op, resolve(s.useAt[i][0])); nop != nil {
				ir.ReplaceOp(ops, i, nop)
				s.useAt[i] = nil // 入力の版は古い命令のもの (simplify の replace と同じ)
				changed = true
				op = nop
			}
		}
		// 定数になった定義はリテラルの load に (使用が全部リテラルに置き換わっていれば次の DCE で消える)
		if d := s.defAt[i]; d != nil && s.konst(d) && isIntLike(ir.ValType(op.Dst)) {
			if _, lit := ir.ValIntLiteral(op.In(0)); op.Code != ir.OpLoad || !lit {
				ir.ReplaceOp(ops, i, &ir.Op{Code: ir.OpLoad, Dst: op.Dst, Src: []ir.Operand{ir.NewIntLiteral("", ir.ValType(op.Dst), normInt(d.bits, ir.ValType(op.Dst)))}, Pos: op.Pos})
				s.useAt[i] = nil
				changed = true
			}
		}
		op = ops[i]
		switch op.Code {
		case ir.OpLoad:
			if s.defAt[i] != nil && sameStorage(op.Dst, op.Src[0]) && ir.ValType(op.Dst) == ir.ValType(op.Src[0]) &&
				ir.PlainOperand(op.Src[0]) && ir.PlainOperand(op.Dst) {
				dels = append(dels, i) // load x = x
			}
		case ir.OpIf, ir.OpIfTrue:
			if n, ok := ir.ValIntLiteral(op.Src[0]); ok {
				if (n != 0) == (op.Code == ir.OpIfTrue) {
					ir.ReplaceOp(ops, i, &ir.Op{Code: ir.OpJump, Label: op.Label, Pos: op.Pos})
				} else {
					ir.DropOp(ops, i)
				}
				changed = true
			}
		case ir.OpSwitch:
			if n, ok := ir.ValIntLiteral(op.Src[0]); ok {
				minV, _ := ir.ValIntLiteral(op.Src[1])
				if k := n - minV; k >= 0 && k < len(op.Labels) {
					ir.ReplaceOp(ops, i, &ir.Op{Code: ir.OpJump, Label: op.Labels[k], Pos: op.Pos})
				} else {
					ir.DropOp(ops, i)
				}
				changed = true
			}
		}
	}
	for _, i := range dels {
		ir.DropOp(ops, i)
		changed = true
	}
	return changed
}

// directMem は添字の無い load_mem / store_mem の番地の版 val が `ref t = g` (写しを辿る) なら、g の Disp バイト目からの
// 直の load にした命令を返す (`lda (t),y` と番地の組み立てが消える: pad.poll が inline した update(&p1, a))。g は volatile で
// ないグローバル・ローカル (その一部の cast も)。読み書きの幅が g に収まるときだけ。
func (s *ssaForm) directMem(op *ir.Op, val *ssaVal) *ir.Op {
	disp := 0 // フィールドの番地 (`add t = p, #k`) のずれ
	for n := 0; n < 8 && val.def >= 0; n++ {
		d := s.lmd.Ops[val.def]
		if d == nil {
			return nil
		}
		if k, lit := ir.ValIntLiteral(d.Src[len(d.Src)-1]); d.Code == ir.OpAdd && lit && k >= 0 && ir.ValType(d.Dst).Kind == types.Pointer &&
			ir.ValType(d.Src[0]).Kind == types.Pointer && ir.PlainOperand(d.Src[0]) {
			u := s.useAt[val.def]
			if len(u) == 0 || u[0] == nil {
				return nil
			}
			disp += k
			val = resolve(u[0])
			continue
		}
		if d.Code == ir.OpLoad && ir.ValType(d.Src[0]).Kind == types.Pointer && ir.PlainOperand(d.Src[0]) {
			u := s.useAt[val.def]
			if len(u) == 0 || u[0] == nil {
				return nil
			}
			val = resolve(u[0])
			continue
		}
		if d.Code != ir.OpRef {
			return nil
		}
		g := d.Src[0]
		gv := ir.UnderlyingValue(g)
		if gv == nil || gv.Volatile || gv.Kind != ir.KindGlobal && gv.Kind != ir.KindLocal || !ir.PlainOperand(g) {
			return nil
		}
		m := op.Mem()
		disp += m.Disp
		if m.Disp < 0 || disp+m.Width > ir.ValType(g).Size {
			return nil
		}
		if op.Code == ir.OpLoadMem {
			return &ir.Op{Code: ir.OpLoad, Dst: op.Dst, Src: []ir.Operand{ir.NewCastedValue(g, ir.ValType(op.Dst), disp)}, Pos: op.Pos}
		}
		v := op.MemValue()
		if ir.ValType(v).Size != m.Width {
			return nil
		}
		return &ir.Op{Code: ir.OpLoad, Dst: ir.NewCastedValue(g, ir.ValType(v), disp), Src: []ir.Operand{v}, Pos: op.Pos}
	}
	return nil
}

// frozenSource は使用 o の版が `load t = y` (y は関数の中でどこからも書かれずアドレスも取られないローカル (slice の引数の
// ポインタ・長さの部分など) で、SSA の外の大きさのもの) の写しなら、y を読むオペランドを返す (`unpack_raw(@ptr(dst), ...)` が
// @ptr の一時変数を経て写し直していた)。
func (s *ssaForm) frozenSource(val *ssaVal, o ir.Operand) ir.Operand {
	if val.def < 0 {
		return nil
	}
	def := s.lmd.Ops[val.def]
	if def == nil || def.Code != ir.OpLoad || def.Dst != ir.Operand(val.v) || !ir.PlainOperand(def.Src[0]) ||
		ir.ValType(def.Src[0]) != val.v.Type {
		return nil
	}
	y := ir.UnderlyingValue(def.Src[0])
	if y == nil || y.Kind != ir.KindLocal || y.Volatile || s.vars[y] || !s.frozen()[y] {
		return nil
	}
	if cv, ok := o.(*ir.CastedValue); ok {
		return ir.RebaseCast(cv, def.Src[0])
	}
	return def.Src[0]
}

// frozen は関数の中でどの命令も書かず、アドレスも取られず asm からも触られないローカル (一度だけ作る)。
func (s *ssaForm) frozen() map[*ir.Value]bool {
	if s.frozenVars != nil {
		return s.frozenVars
	}
	bad := map[*ir.Value]bool{}
	s.frozenVars = map[*ir.Value]bool{}
	for _, op := range s.lmd.Ops {
		if op == nil {
			continue
		}
		if v := ir.UnderlyingValue(op.Dst); v != nil && op.Dst != nil {
			bad[v] = true
		}
		for _, o := range append([]ir.Operand{op.Dst}, op.Src...) {
			if pa, ok := o.(*ir.PointeredArray); ok {
				if v := ir.UnderlyingValue(pa.From); v != nil {
					bad[v] = true
				}
			}
			if cv, ok := o.(*ir.CastedValue); ok {
				if pa, ok := cv.From.(*ir.PointeredArray); ok {
					if v := ir.UnderlyingValue(pa.From); v != nil {
						bad[v] = true
					}
				}
			}
		}
		if op.Code == ir.OpRef || op.Code == ir.OpAsm {
			for _, o := range op.Src {
				if v := ir.UnderlyingValue(o); v != nil {
					bad[v] = true
				}
			}
		}
	}
	for _, v := range s.lmd.Vars {
		if v.Kind == ir.KindLocal && !bad[v] {
			s.frozenVars[v] = true
		}
	}
	return s.frozenVars
}

// highByteOf は使用 o が 2 バイトの x の下位 1 バイト (`(a >> 8) as u8`) で、x の版が `x = y >> 8` なら、y の上位 1 バイトを
// 読むオペランドを返す (y は i でも同じ版のとき。16 ビットのずらしと一時の値が消える: PPUADDR に番地の上位を書く所)。
func (s *ssaForm) highByteOf(val *ssaVal, o ir.Operand, i int) ir.Operand {
	cv, ok := o.(*ir.CastedValue)
	if !ok || !cv.Plain() || cv.Offset != 0 || cv.Type.Size != 1 || val.def < 0 {
		return nil
	}
	d := s.lmd.Ops[val.def]
	if d == nil || d.Code != ir.OpShiftRight || d.Dst != cv.From || ir.ValType(d.Dst).Size != 2 || ir.ValType(d.Src[0]).Size != 2 {
		return nil
	}
	if n, ok := ir.ValIntLiteral(d.Src[1]); !ok || n != 8 {
		return nil
	}
	y := s.sameOperandAt(val.def, 0, i)
	if y == nil || !ir.PlainOperand(y) && !isZeroExt(y) {
		return nil
	}
	return ir.NewCastedValue(y, cv.Type, 1) // 下位だけのゼロ拡張なら Width 0 (値は 0)
}

// exactOperand は a と b が同じ読み方の同じ値か (同じ変数、または同じ変数の同じ cast)。
func exactOperand(a, b ir.Operand) bool {
	if a == b {
		return true
	}
	ca, ok1 := a.(*ir.CastedValue)
	cb, ok2 := b.(*ir.CastedValue)
	return ok1 && ok2 && ca.From == cb.From && ca.Type == cb.Type && ca.Offset == cb.Offset && ca.Width == cb.Width
}

// isZeroExt は o が 1 バイトをゼロ拡張して読む cast か。
func isZeroExt(o ir.Operand) bool {
	cv, ok := o.(*ir.CastedValue)
	return ok && cv.Width < cv.Type.Size
}

// rewriteLogs は命令 i の @log の引数を、その地点での版の定数・コピー元に置き換える (注釈は使用に数えないので、
// 変数が消えても地点で値が分かるように。ir/log.go)。変数がその地点で生きているか、同じブロックの中に定義があるときだけ:
// 死んでいる地点には合流の φ が無く、valueAt が合流の前の片方の版を返す。
func (s *ssaForm) rewriteLogs(i int, op *ir.Op, live *ir.Liveness) {
	for _, p := range op.Logs {
		for _, a := range p.Args {
			u, ok := a.Val.(*ir.Value)
			if !ok || !s.vars[u] || u.LogStale || u.LogNoValue {
				continue // 死んだ代入を消した変数は、版の並びが元の値の並びと違う
			}
			if !live.LiveIn(i, u) && !s.definedInBlock(u, i) {
				continue
			}
			val := s.valueAt(u, i)
			if val.undef {
				continue
			}
			if isIntLike(u.Type) && s.konst(val) {
				a.Val = ir.NewIntLiteral("", u.Type, normInt(resolve(val).bits, u.Type))
			} else if y := s.copySource(val, i); y != nil {
				a.Val = y
			}
		}
	}
}

// definedInBlock は命令 i の前に、同じブロックの中で v の定義があるか。
func (s *ssaForm) definedInBlock(v *ir.Value, i int) bool {
	b := s.blockOf[i]
	for j := i - 1; j >= b.Start; j-- {
		if d := s.defAt[j]; d != nil && d.v == v {
			return true
		}
	}
	return false
}

// copySource は版 val が `load x = y` のコピーで、命令 i の位置でも y がその版のままなら y を返す。
// y はユーザーの変数か引数に限る (一時変数は coalesceCopies などが「定義の直後の 1 回の使用」の形で扱うので広げない。
// 常駐レジスタの候補も LTNone / LTArg だけなので、その形を保つ)。
func (s *ssaForm) copySource(val *ssaVal, i int) *ir.Value {
	if val.def < 0 {
		return nil
	}
	def := s.lmd.Ops[val.def]
	if def.Code != ir.OpLoad {
		return nil
	}
	y, ok := def.Src[0].(*ir.Value)
	if !ok || !s.vars[y] || y.Type != val.v.Type || (y.LocalType != ir.LTNone && y.LocalType != ir.LTArg) {
		return nil
	}
	if _, isValue := def.Dst.(*ir.Value); !isValue {
		return nil
	}
	ys := s.useAt[val.def]
	if len(ys) == 0 || ys[0] == nil || resolve(ys[0]) != s.valueAt(y, i) {
		return nil
	}
	op := s.lmd.Ops[i]
	if op.Dst != nil && ir.UnderlyingValue(op.Dst) == y && !op.Code.ReadsBeforeWrite() {
		return nil
	}
	return y
}

// rebase は o (CastedValue かもしれない) の元の変数を y に差し替えたものを返す。
func rebase(o ir.Operand, y *ir.Value) ir.Operand {
	if cv, ok := o.(*ir.CastedValue); ok {
		return ir.RebaseCast(cv, y)
	}
	return y
}

// simplify は演算の連鎖の代数的な簡約 (符号なしの整数だけ):
//
//	mul d = (div y, 2^k), 2^k   → and d = y, ~(2^k-1)   (`y / 16 * 16`。castle の bg.cell_type / my.get_cell)
//	and d = (and y, m2), m      → and d = y, m & m2
//	or  d = (or y, m2), m       → or  d = y, m | m2
//	shift d = (shift y, j), k   → shift d = y, j + k   (同じ向き)
//	add / sub d = (add / sub y, j), k → add d = y, ±j ± k
//	add / sub d = (sub j, y), k → sub d = j ± k, y   (inline した `room() - 3` が `128 - len - 3` になる)
//
// 中間の版 x の定義から y を取るので、使用位置でも y が同じ版でなければならない (valueAt)。x は他で使われていれば残る。
func (s *ssaForm) simplify() bool {
	changed := false
	// 置き換えた命令の入力の版 (useAt) は古い命令のものなので捨てる。残すと後ろの命令がこの命令を写しとして辿って
	// 古い入力の定義に着き、もう一度畳む (`((g3 + g0) - g0) - g0` の 2 つ目の sub も (a + b) - b として g3 にしていた。
	// TestSSASimplifyAfterRewrite)。版の分からない入力は sameOperandAt が nil を返し、次の周で作り直す
	replace := func(i int, nop *ir.Op) {
		ir.ReplaceOp(s.lmd.Ops, i, nop)
		s.useAt[i] = nil
		changed = true
	}
	for i, op := range s.lmd.Ops {
		// 結果の置き場所は問わない (グローバル・戻り値の slice の長さの部分も。置き換えた命令も同じ所に書く)
		if op == nil || s.blockOf[i] == nil || op.Dst == nil || len(op.Src) != 2 {
			continue
		}
		dt := ir.ValType(op.Dst)
		if !isIntLike(dt) || dt.Signed {
			continue
		}
		// 第 1 入力の版の定義 (演算)。mul は可換なので定数を第 2 入力に寄せる
		if op.Code == ir.OpMul {
			if _, lit := ir.ValIntLiteral(op.Src[0]); lit {
				op.Src[0], op.Src[1] = op.Src[1], op.Src[0]
				if len(s.useAt[i]) == 2 {
					s.useAt[i][0], s.useAt[i][1] = s.useAt[i][1], s.useAt[i][0]
				}
			}
		}
		m, ok := ir.ValIntLiteral(op.Src[1])
		if !ok && op.Code != ir.OpSub {
			continue // 定数でない第 2 入力は (a + b) - a だけ
		}
		us := s.useAt[i]
		if len(us) == 0 || us[0] == nil {
			continue
		}
		x := resolve(us[0])
		if x.def < 0 {
			continue
		}
		def := s.lmd.Ops[x.def]
		// 写し (`load $5 = room0.$result`: inline した関数の戻り値) を辿る (rewrite が写しを消すのは次の周)。命令の並びで前へ
		// 戻る一本道だけ (ループを回って自分や後ろの命令に着くと、その入力は i での版と比べられない: `n -= 1` を n + 0 にしていた)。
		// 写し先がコンパイラの作った変数 (名前に $) のときだけ: ユーザーの変数の写しを畳むとその変数が死に、@log で見えなくなる
		// (TestLogBasics の inline した f の c)
		for n := 0; n < 8 && x.def < i && def != nil && def.Code == ir.OpLoad && ir.PlainOperand(def.Src[0]) &&
			ir.ValOffset(def.Src[0]) == 0 && ir.ValType(def.Src[0]) == ir.ValType(def.Dst) &&
			strings.Contains(ir.UnderlyingValue(def.Dst).Name, "$"); n++ {
			u := s.useAt[x.def]
			if len(u) == 0 || u[0] == nil {
				break
			}
			if y := resolve(u[0]); y.def < 0 || y.def >= x.def {
				break
			}
			x = resolve(u[0])
			def = s.lmd.Ops[x.def]
		}
		if def == nil || len(def.Src) != 2 || ir.ValType(def.Dst) != ir.ValType(op.Src[0]) || ir.ValType(def.Dst).Signed {
			continue // def が nil: rewrite が消した `load x = x` (fuzz で発覚)
		}
		if !ir.PlainOperand(op.Src[0]) || ir.ValOffset(op.Src[0]) != 0 {
			// 入力が def の結果を切り詰めて読む (`cast<u16, 0/1>(x >> 1) >> 0` は下位 1 バイトだけ) なら畳めない
			// (x >> 1 に畳んで上位を残していた。TestRandomConstFold で発覚)
			continue
		}
		if op.IsSigned() || def.IsSigned() {
			continue // 符号付きの除算・算術シフトは畳まない (以下は符号なしの規則)
		}
		if !ok {
			if def.Code != ir.OpAdd {
				continue
			}
			// (a + b) - a → b、(a + b) - b → a (`q[i..i + n]` の長さ)
			for k := 0; k < 2; k++ {
				other := s.sameOperandAt(x.def, 1-k, i)
				if exactOperand(op.Src[1], def.Src[k]) && s.sameOperandAt(x.def, k, i) != nil && other != nil &&
					ir.ValType(other).Size == dt.Size {
					replace(i, &ir.Op{Code: ir.OpLoad, Dst: op.Dst, Src: []ir.Operand{other}, Pos: op.Pos})
					break
				}
			}
			continue
		}
		if k1, ok := ir.ValIntLiteral(def.Src[0]); ok && def.Code == ir.OpSub && (op.Code == ir.OpAdd || op.Code == ir.OpSub) {
			// (k1 - y) ± m → (k1 ± m) - y
			y := s.sameOperandAt(x.def, 1, i)
			if y == nil {
				y = s.volatileOnce(x.def, 1, i)
			}
			if y == nil || ir.ValType(y) != ir.ValType(def.Src[1]) || ir.ValType(y).Size != dt.Size || ir.ValType(y).Signed {
				continue
			}
			k := k1 + m
			if op.Code == ir.OpSub {
				k = k1 - m
			}
			replace(i, &ir.Op{Code: ir.OpSub, Dst: op.Dst, Src: []ir.Operand{ir.NewIntLiteral("", ir.ValType(def.Src[0]), bitsOf(k, dt.Size)), y}, Pos: op.Pos})
			continue
		}
		m2, ok := ir.ValIntLiteral(def.Src[1])
		if !ok {
			continue
		}
		y := s.sameOperandAt(x.def, 0, i)
		if y == nil {
			y = s.volatileOnce(x.def, 0, i)
		}
		if y == nil || ir.ValType(y) != ir.ValType(def.Src[0]) || ir.ValType(y).Size != dt.Size || ir.ValType(y).Signed {
			continue
		}
		bits := 8 * dt.Size
		var code ir.OpCode
		var k int
		switch {
		case op.Code == ir.OpMul && def.Code == ir.OpDiv && m == m2 && m > 0 && m&(m-1) == 0:
			code, k = ir.OpAnd, bitsOf(^(m-1), dt.Size)
		case op.Code == ir.OpAnd && def.Code == ir.OpAnd:
			code, k = ir.OpAnd, bitsOf(m&m2, dt.Size)
		case op.Code == ir.OpOr && def.Code == ir.OpOr:
			code, k = ir.OpOr, bitsOf(m|m2, dt.Size)
		case (op.Code == ir.OpShiftLeft || op.Code == ir.OpShiftRight) && def.Code == op.Code && m >= 0 && m2 >= 0 && m+m2 < bits:
			code, k = op.Code, m+m2
		case (op.Code == ir.OpAdd || op.Code == ir.OpSub) && (def.Code == ir.OpAdd || def.Code == ir.OpSub):
			// (y ± m2) ± m → y + (±m2 ± m)
			if def.Code == ir.OpSub {
				m2 = -m2
			}
			if op.Code == ir.OpSub {
				m = -m
			}
			code, k = ir.OpAdd, bitsOf(m2+m, dt.Size)
		default:
			continue
		}
		nop := &ir.Op{Code: code, Dst: op.Dst, Src: []ir.Operand{y, ir.NewIntLiteral("", ir.ValType(op.Src[1]), k)}, Pos: op.Pos}
		if code.HasSign() {
			nop.Sign = ir.Unsigned // 符号なしの連鎖だけを畳む
		}
		replace(i, nop)
	}
	return changed
}

// volatileOnce は命令 def の入力 k が volatile なグローバル (asm・NMI も触る frame.queue_len など) で、def の結果が (コンパイラの
// 写しを経て) 命令 i でだけ使われるなら、その入力を返す (畳んだ i が読み、def と写しは死んで消えるので、読む回数は 1 回の
// まま。読む時が def から i に動くが、間に副作用のある命令は無い: untouched。inline した vram の `room() - 3` =
// `(128 - frame.queue_len) - 3`)。
func (s *ssaForm) volatileOnce(def, k, i int) ir.Operand {
	d := s.lmd.Ops[def]
	o := d.Src[k]
	g := ir.UnderlyingValue(o)
	if g == nil || g.Kind != ir.KindGlobal || !g.Volatile || !ir.PlainOperand(o) || !s.untouched(g, def, i) {
		return nil
	}
	t, ok := d.Dst.(*ir.Value)
	if !ok || t.Kind != ir.KindLocal {
		return nil
	}
	if s.ud == nil {
		s.ud = ir.BuildUseDef(s.lmd)
	}
	// i の入力から写しを遡って t に着き、どの変数も 1 回だけ定義されて 1 回だけ使われる
	v, user := ir.UnderlyingValue(s.lmd.Ops[i].Src[0]), s.lmd.Ops[i]
	for n := 0; n < 8 && v != nil; n++ {
		if u, single := s.ud.SingleUse(v); !single || u != user || s.ud.NumDefs(v) != 1 {
			return nil
		}
		if v == t {
			return o
		}
		c, _ := s.ud.SingleDef(v)
		if c.Code != ir.OpLoad || !ir.PlainOperand(c.Src[0]) {
			return nil
		}
		v, user = ir.UnderlyingValue(c.Src[0]), c
	}
	return nil
}

// sameOperandAt は命令 def の入力 k が、命令 i の位置でも同じ値として読めるならその入力を返す
// (リテラル、または同じ版のローカル変数。グローバルは間で書き換わりうるので不可)。
func (s *ssaForm) sameOperandAt(def, k, i int) ir.Operand {
	if s.lmd.Ops[def] == nil {
		return nil
	}
	o := s.lmd.Ops[def].Src[k]
	if _, lit := ir.ValIntLiteral(o); lit {
		return o
	}
	if g := ir.UnderlyingValue(o); g != nil && g.Kind == ir.KindGlobal && !g.Volatile && ir.PlainOperand(o) && s.untouched(g, def, i) {
		return o // グローバルは、同じブロックの間の命令が副作用も g への書き込みも無ければ同じ値 (inline した room() - 3)
	}
	us := s.useAt[def]
	if k >= len(us) || us[k] == nil {
		return nil
	}
	v := ir.UnderlyingValue(o)
	if resolve(us[k]) != s.valueAt(v, i) {
		return nil
	}
	if op := s.lmd.Ops[i]; op.Dst != nil && ir.UnderlyingValue(op.Dst) == v && !op.Code.ReadsBeforeWrite() {
		return nil
	}
	return o
}

// untouched は命令 def から i へ一本道で (間のラベルはどこからも飛んで来ない: inline した関数の出口)、その間 (と i の書き込み)
// がグローバル g を書き換えないか (間は副作用の無い命令だけ)。
func (s *ssaForm) untouched(g *ir.Value, def, i int) bool {
	if def >= i || s.blockOf[def] == nil || s.blockOf[i] == nil {
		return false
	}
	if s.jumped == nil {
		s.jumped = map[string]bool{}
		for _, o := range s.lmd.Ops {
			if o != nil && o.Code != ir.OpLabel {
				for _, l := range append([]string{o.Label}, o.Labels...) {
					s.jumped[l] = true
				}
			}
		}
	}
	for _, o := range s.lmd.Ops[def+1 : i] {
		if o == nil || o.Code == ir.OpLabel && !s.jumped[o.Label] {
			continue
		}
		if !o.Code.IsPure() || ir.UnderlyingValue(o.Dst) == g {
			return false
		}
	}
	return true
}

// eliminateDead は使われない版の定義 (副作用の無い命令) を消す。
func (s *ssaForm) eliminateDead() bool {
	live := map[*ssaVal]bool{}
	var work []*ssaVal
	for i, op := range s.lmd.Ops {
		if op == nil || s.blockOf[i] == nil {
			continue
		}
		if d := s.defAt[i]; d != nil && op.Code.IsPure() && !ir.FeedsCarry(s.lmd.Ops, i) {
			continue
		}
		for _, u := range s.useAt[i] {
			if u != nil {
				work = append(work, u)
			}
		}
	}
	for len(work) > 0 {
		val := resolve(work[len(work)-1])
		work = work[:len(work)-1]
		if live[val] {
			continue
		}
		live[val] = true
		if val.phi {
			work = append(work, val.args...)
		} else if val.def >= 0 {
			for _, u := range s.useAt[val.def] {
				if u != nil {
					work = append(work, u)
				}
			}
		}
	}
	changed := false
	for i, op := range s.lmd.Ops {
		if op == nil {
			continue
		}
		if d := s.defAt[i]; d != nil && op.Code.IsPure() && !live[d] && !ir.FeedsCarry(s.lmd.Ops, i) {
			d.v.LogStale = true // 死んだ代入: この後の @log は、生きている地点でしか値を読まない (ir/log.go)
			ir.DropOp(s.lmd.Ops, i)
			changed = true
		}
	}
	return changed
}
