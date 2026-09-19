package opt

// SSA 形式による定数伝播 / コピー伝播 / 死んだ定義の除去 (doc/v2_ssa.md)。
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
	lmd     *ir.Lambda
	cfg     *ir.CFG
	vars    map[*ir.Value]bool // 対象の変数
	blockOf []*ir.Block        // 命令 → ブロック (到達できない命令は nil)

	cur        map[*ir.Block]map[*ir.Value]*ssaVal // ブロック内の最後の定義
	entry      map[*ir.Block]map[*ir.Value]*ssaVal // ブロックの入口の版
	incomplete map[*ir.Block]map[*ir.Value]*ssaVal // 封印前に作った φ
	sealed     map[*ir.Block]bool
	filled     map[*ir.Block]bool
	reach      map[*ir.Block]bool

	useAt map[int][]*ssaVal // 命令の各入力 (Src の添字順) の版。対象外の入力は nil
	defAt map[int]*ssaVal   // 命令が定義する版
	undef *ssaVal
}

// propagateSSA は定数 / コピー伝播と死んだ定義の除去を、変化が無くなるまで繰り返す。
func propagateSSA(lmd *ir.Lambda) {
	for n := 0; n < 16; n++ {
		s := buildSSA(lmd)
		if s == nil {
			return
		}
		changed := s.rewrite()
		compact(lmd)
		s = buildSSA(lmd)
		if s == nil {
			return
		}
		if s.eliminateDead() {
			changed = true
		}
		compact(lmd)
		if !changed {
			return
		}
	}
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

// tryRemoveTrivialPhi は入力が (自分自身と未定義を除いて) 1 種類しかない φ をその入力で置き換える。
func (s *ssaForm) tryRemoveTrivialPhi(phi *ssaVal) *ssaVal {
	var same *ssaVal
	for _, a := range phi.args {
		a = resolve(a)
		if a == same || a == phi || a.undef {
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
	off := 8 * uint(ir.ValOffset(o))
	if n, ok := ir.ValIntLiteral(o); ok {
		return n >> off, true
	}
	us := s.useAt[i]
	if k >= len(us) || us[k] == nil {
		return 0, false
	}
	val := resolve(us[k])
	if !s.konst(val) {
		return 0, false
	}
	return bitsOf(val.bits>>off, t.Size), true
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
		return bitsOf(normInt(a, t0), size), true
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
		a, b = normInt(a, dt), normInt(b, dt)
		if b == 0 {
			return 0, false
		}
		if op.Code == ir.OpDiv {
			return bitsOf(ir.FloorDiv(a, b), size), true
		}
		return bitsOf(ir.FloorMod(a, b), size), true
	case ir.OpShiftLeft, ir.OpShiftRight:
		a, b = normBits(a, dt.Size, t0.Signed), normInt(b, t1)
		if b < 0 || b > 64 {
			return 0, false
		}
		if op.Code == ir.OpShiftLeft {
			return bitsOf(ir.Shl(a, b), size), true
		}
		return bitsOf(ir.Shr(a, b), size), true
	case ir.OpEq, ir.OpLt:
		w, signed := max(t0.Size, t1.Size), t0.Signed || t1.Signed
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
	var dels []int // ループの後で消す (ループ中は他の版の定義として参照されうる)
	for i, op := range ops {
		if op == nil || s.blockOf[i] == nil {
			continue
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
			}
		}
		// 定数になった定義はリテラルの load に (使用が全部リテラルに置き換わっていれば次の DCE で消える)
		if d := s.defAt[i]; d != nil && s.konst(d) && isIntLike(ir.ValType(op.Dst)) {
			if _, lit := ir.ValIntLiteral(op.In(0)); op.Code != ir.OpLoad || !lit {
				ops[i] = &ir.Op{Code: ir.OpLoad, Dst: op.Dst, Src: []ir.Operand{ir.NewIntLiteral("", ir.ValType(op.Dst), normInt(d.bits, ir.ValType(op.Dst)))}, Pos: op.Pos}
				changed = true
			}
		}
		op = ops[i]
		switch op.Code {
		case ir.OpLoad:
			if s.defAt[i] != nil && sameStorage(op.Dst, op.Src[0]) && ir.ValType(op.Dst) == ir.ValType(op.Src[0]) {
				dels = append(dels, i) // load x = x
			}
		case ir.OpIf, ir.OpIfTrue:
			if n, ok := ir.ValIntLiteral(op.Src[0]); ok {
				if (n != 0) == (op.Code == ir.OpIfTrue) {
					ops[i] = &ir.Op{Code: ir.OpJump, Label: op.Label, Pos: op.Pos}
				} else {
					ops[i] = nil
				}
				changed = true
			}
		case ir.OpSwitch:
			if n, ok := ir.ValIntLiteral(op.Src[0]); ok {
				minV, _ := ir.ValIntLiteral(op.Src[1])
				if k := n - minV; k >= 0 && k < len(op.Labels) {
					ops[i] = &ir.Op{Code: ir.OpJump, Label: op.Labels[k], Pos: op.Pos}
				} else {
					ops[i] = nil
				}
				changed = true
			}
		}
	}
	for _, i := range dels {
		ops[i] = nil
		changed = true
	}
	return changed
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
	if op.Dst != nil && ir.UnderlyingValue(op.Dst) == y && !readsBeforeWrite(op.Code) {
		return nil
	}
	return y
}

// rebase は o (CastedValue の連鎖かもしれない) の元の変数を y に差し替えたものを返す。
func rebase(o ir.Operand, y *ir.Value) ir.Operand {
	if cv, ok := o.(*ir.CastedValue); ok {
		return ir.NewCastedValue(rebase(cv.From, y), cv.Type, cv.Offset)
	}
	return y
}

// eliminateDead は使われない版の定義 (副作用の無い命令) を消す。
func (s *ssaForm) eliminateDead() bool {
	live := map[*ssaVal]bool{}
	var work []*ssaVal
	for i, op := range s.lmd.Ops {
		if op == nil || s.blockOf[i] == nil {
			continue
		}
		if d := s.defAt[i]; d != nil && isPure(op.Code) {
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
		if d := s.defAt[i]; d != nil && isPure(op.Code) && !live[d] {
			s.lmd.Ops[i] = nil
			changed = true
		}
	}
	return changed
}

// isPure は結果が使われなければ消してよい命令か (regalloc.DeleteUnuse と同じ集合)。
func isPure(c ir.OpCode) bool {
	switch c {
	case ir.OpPget, ir.OpLoad,
		ir.OpAdd, ir.OpSub, ir.OpAnd, ir.OpOr, ir.OpXor, ir.OpMul, ir.OpDiv, ir.OpMod,
		ir.OpShiftLeft, ir.OpShiftRight, ir.OpUminus, ir.OpEq, ir.OpLt, ir.OpNot, ir.OpBitNot,
		ir.OpIndex, ir.OpRef, ir.OpSignExtension, ir.OpIndexPget, ir.OpFieldPget, ir.OpRolC, ir.OpRorC:
		return true
	}
	return false
}
