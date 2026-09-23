// Package interp は IR を直接実行するインタプリタ (fuzz の判定役)。
//
// 差分テスト (internal/driver の TestRandomPrograms) は -O 0 と -O 2 の emu の出力を比べるので、両方のレベルで同じように
// 間違える codegen のバグ (符号付きの変数シフトなど) は見えない。ここで sema が作った最適化前の IR を 6502 を介さずに
// 実行し、emu の出力と比べる 3 つ目の判定にする。各命令の意味は codegen (internal/codegen) が出すコードに合わせる:
//
//   - オペランドの k バイト目は codegen の byte と同じ: 整数リテラルは値のビット列 (上位は符号ごと続く)、変数は自分の
//     大きさまで (その先は 0)、cast は Width まで (その先は 0)
//   - 加減算・論理演算・シフトは Dst の幅で計算する。比較は 2 つの入力の大きい方の幅、符号はどちらかが符号付きなら符号付き
//   - 乗除算は Dst の幅と符号で (share/runtime.asm の __div_8s などと同じ床除算)
//
// メモリは 64KB の平らな配列で、emu と同じく $FFFE (EMU_PRINT) / $FFFF (EMU_EXIT) への書き込みで出力・終了する
// (stdio は FC のコードのまま実行する)。大域変数と定数表はシンボルごとに番地を割り当て、ローカル変数は呼び出しごとに
// スタック領域から取る (再帰してもよい)。関数には偽の番地を振り、関数ポインタはその番地で呼び先を引く。
//
// 最適化後の IR も実行できる (fuzz の失敗をどの opt のパスが起こしたか切り分けるため): C フラグは shift_left /
// shift_right が最後に押し出したビットで、直後の if_carry / rolc / rorc が受ける (opt.carryBranch と splitWords が
// 作る形。間に C を変える命令は無い)。asm は扱わない (ErrUnsupported)。
package interp

import (
	"errors"
	"fmt"
	"strings"

	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// ErrUnsupported はインタプリタが扱わない命令・値に出会った (判定を飛ばす)。
var ErrUnsupported = errors.New("interp: unsupported")

// ErrStepLimit は命令数の上限に達した (止まらない)。
var ErrStepLimit = errors.New("interp: step limit exceeded")

// Result は実行の結果。
type Result struct {
	Out  string
	Exit int
}

const (
	dataBase  = 0x0200 // 大域変数・定数表
	stackBase = 0x4000 // ローカル変数 (呼び出しごと)
	stackEnd  = 0x7f00
	codeBase  = 0x8000 // 関数の偽の番地 (中身は無い)
	portAddr  = 0xfff0 // EMU_ADDR (2 バイト)
	portData  = 0xfff2 // EMU_DATA (2 バイト)
	portPrint = 0xfffe // EMU_PRINT
	portExit  = 0xffff // EMU_EXIT
)

type machine struct {
	mem      [0x10000]byte
	syms     map[string]int         // 全体のシンボル (モジュール名つきの大域変数・関数など)
	local    map[any]map[string]int // 関数 / モジュールの中だけのシンボル (`_2` のような定数データ。.proc の中の名前で、別の関数やモジュールにも同じ名前がある)
	lambdas  map[int]*ir.Lambda     // 関数の偽の番地 → 関数
	codeAddr map[*ir.Lambda]int
	labels   map[*ir.Lambda]map[string]int
	sp       int
	out      strings.Builder
	exited   bool
	exit     int
	carry    bool // 直前の shift / rolc / rorc が押し出したビット
	steps    int64
	maxSteps int64
}

// frame は関数の 1 回の呼び出し (ローカル変数の番地)。
type frame struct {
	lmd  *ir.Lambda
	vars map[*ir.Value]int
}

// call は push_result から call までの積みかけの呼び出し。
type call struct {
	args [][]byte
}

// Run は modules の _main を実行する。maxSteps は実行する IR 命令数の上限 (0 なら無制限)。
func Run(modules []*ir.Module, maxSteps int64) (res Result, err error) {
	m := &machine{syms: map[string]int{}, local: map[any]map[string]int{}, lambdas: map[int]*ir.Lambda{}, codeAddr: map[*ir.Lambda]int{},
		labels: map[*ir.Lambda]map[string]int{}, sp: stackBase, maxSteps: maxSteps}
	m.mem[portPrint], m.mem[portExit] = 255, 255
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok && (errors.Is(e, ErrUnsupported) || errors.Is(e, ErrStepLimit)) {
				err = e
				return
			}
			if e, ok := r.(runtimeError); ok {
				err = e
				return
			}
			panic(r)
		}
	}()
	main := m.layout(modules)
	if main == nil {
		return Result{}, fmt.Errorf("%w: no _main", ErrUnsupported)
	}
	m.invoke(main, nil)
	if !m.exited {
		return Result{Out: m.out.String()}, runtimeError("main returned without exit")
	}
	return Result{Out: m.out.String(), Exit: m.exit}, nil
}

// runtimeError はプログラムの実行時の異常 (番地の外など)。
type runtimeError string

func (e runtimeError) Error() string { return "interp: " + string(e) }

func unsupported(format string, args ...any) {
	panic(fmt.Errorf("%w: %s", ErrUnsupported, fmt.Sprintf(format, args...)))
}

// ---------------------------------------------------------------
// 配置
// ---------------------------------------------------------------

// layout はシンボルに番地を割り当て、定数表の中身を書く。_main を返す。
func (m *machine) layout(modules []*ir.Module) *ir.Lambda {
	type scoped struct {
		def   *ir.Def
		scope any // *ir.Lambda か *ir.Module
	}
	var defs []scoped
	var main *ir.Lambda
	next := codeBase
	for _, mod := range modules {
		for _, d := range mod.Defs {
			defs = append(defs, scoped{d, mod})
		}
		for _, l := range mod.Lambdas {
			for _, d := range l.Defs {
				defs = append(defs, scoped{d, l})
			}
			m.codeAddr[l] = next
			m.lambdas[next] = l
			m.syms[l.Id] = next
			next += 4
			if l.Id == "_main" {
				main = l
			}
		}
	}
	define := func(sd scoped, a int) {
		if m.local[sd.scope] == nil {
			m.local[sd.scope] = map[string]int{}
		}
		m.local[sd.scope][sd.def.Sym] = a
		if _, dup := m.syms[sd.def.Sym]; !dup {
			m.syms[sd.def.Sym] = a
		}
	}
	data := dataBase
	for _, sd := range defs {
		d := sd.def
		switch d.Kind {
		case ir.DefBss, ir.DefBlock:
			define(sd, data)
			size := d.Type.Size
			if size < 0 {
				size = 0
			}
			data += size
			if data >= stackBase {
				unsupported("data too large")
			}
		case ir.DefCode:
			if d.Lambda != nil {
				if a, ok := m.codeAddr[d.Lambda]; ok {
					define(sd, a)
				}
			}
		}
	}
	// equ は別のシンボルを指すことがあるので、引けるものから順に
	for changed := true; changed; {
		changed = false
		for _, sd := range defs {
			d := sd.def
			if d.Kind != ir.DefEqu || d.Equ == nil {
				continue
			}
			if _, done := m.local[sd.scope][d.Sym]; done {
				continue
			}
			if d.Equ.IsInt {
				define(sd, d.Equ.Int&0xffff)
				changed = true
			} else if a, ok := m.lookup(sd.scope, d.Equ.Symbol); ok {
				define(sd, a)
				changed = true
			}
		}
	}
	for _, sd := range defs {
		if sd.def.Kind == ir.DefBlock {
			m.writeBlock(sd.scope, m.local[sd.scope][sd.def.Sym], sd.def.Type, sd.def.Elems)
		}
	}
	return main
}

// lookup はシンボルの番地 (関数の中 → そのモジュールの中 → 全体の順)。scope は *ir.Lambda か *ir.Module (nil 可)。
func (m *machine) lookup(scope any, sym string) (int, bool) {
	if l, ok := scope.(*ir.Lambda); ok {
		if a, ok := m.local[l][sym]; ok {
			return a, true
		}
		scope = l.Module
	}
	if mod, ok := scope.(*ir.Module); ok && mod != nil {
		if a, ok := m.local[mod][sym]; ok {
			return a, true
		}
	}
	a, ok := m.syms[sym]
	return a, ok
}

// scopeOf は f の関数 (f が nil なら全体)。
func scopeOf(f *frame) any {
	if f == nil {
		return nil
	}
	return f.lmd
}

// writeBlock は定数データ (配列・struct) を書く。codegen の emitData と同じく、struct はフィールドごと、配列は要素ごとに
// 型をたどって平らに並べる (以前は要素を全部 1 バイトとして書いていて、struct のリテラル `var ls:S = {…}` の値が
// ずれていた)。
func (m *machine) writeBlock(scope any, addr int, t *types.Type, elems []ir.Operand) {
	f := &frame{lmd: &ir.Lambda{}}
	switch x := scope.(type) {
	case *ir.Lambda:
		f.lmd = x
	case *ir.Module:
		f.lmd = &ir.Lambda{Module: x}
	}
	var put func(t *types.Type, v ir.Operand)
	put = func(t *types.Type, v ir.Operand) {
		switch t.Kind {
		case types.Struct, types.Array:
			lv := ir.ValLiteral(v)
			if lv == nil || lv.Kind != ir.KindArrayLiteral {
				unsupported("constant %s for %s", ir.OperandString(v), t)
			}
			for i, e := range lv.Elems {
				if t.Kind == types.Struct {
					put(t.Fields[i].Type, e)
				} else {
					put(t.Base, e)
				}
			}
		default:
			n := t.Size
			if t.IsFarFunc() {
				n = 3
			}
			for k := 0; k < n; k++ {
				m.mem[addr&0xffff] = m.byteOf(f, v, k)
				addr++
			}
		}
	}
	if t.Kind == types.Struct {
		put(t, ir.NewArrayLiteral("", t, elems))
		return
	}
	for _, e := range elems {
		put(t.Base, e)
	}
}

// ---------------------------------------------------------------
// 値の読み書き
// ---------------------------------------------------------------

func size(o ir.Operand) int { return ir.ValType(o).Size }

// addrOf はメモリ上の値 (変数・配列・cast したその一部) の番地。
func (m *machine) addrOf(f *frame, o ir.Operand) int {
	switch x := o.(type) {
	case *ir.Value:
		switch x.Kind {
		case ir.KindLocal:
			if a, ok := f.vars[x]; ok {
				return a
			}
			unsupported("local %s not in frame of %s", x.Name, f.lmd.Id)
		case ir.KindGlobal:
			if a, ok := m.lookup(scopeOf(f), x.Symbol); ok {
				return a
			}
			unsupported("unknown symbol %q", x.Symbol)
		}
		unsupported("address of %s", ir.OperandString(o))
	case *ir.CastedValue:
		return (m.addrOf(f, x.From) + x.Offset) & 0xffff
	}
	unsupported("address of %s", ir.OperandString(o))
	return 0
}

// symAddr はリテラルのシンボル (関数・データ) の番地。
func (m *machine) symAddr(f *frame, sym string) int {
	if a, ok := m.lookup(scopeOf(f), sym); ok {
		return a
	}
	unsupported("unknown symbol %q", sym)
	return 0
}

// byteOf は o の k バイト目 (codegen の byte と同じ規則)。
func (m *machine) byteOf(f *frame, o ir.Operand, k int) byte {
	switch x := o.(type) {
	case *ir.Value:
		switch x.Kind {
		case ir.KindLiteral:
			if x.IsInt {
				return byte(x.Int >> (8 * k))
			}
			a := m.symAddr(f, x.Symbol)
			switch k {
			case 0:
				return byte(a)
			case 1:
				return byte(a >> 8)
			}
			return 0 // far 関数ポインタのバンク (インタプリタには無い)
		case ir.KindLocal, ir.KindGlobal:
			if x.Type.Kind == types.Array {
				// 配列はその番地 (ポインタへの変換)
				a := m.addrOf(f, x)
				return byte(a >> (8 * k))
			}
			if k >= x.Type.Size {
				return 0
			}
			return m.mem[(m.addrOf(f, x)+k)&0xffff]
		}
		unsupported("value %s", ir.OperandString(o))
	case *ir.CastedValue:
		if k >= x.Width {
			return 0
		}
		if lv, ok := x.From.(*ir.Value); ok && lv.Kind == ir.KindLiteral {
			return m.byteOf(f, lv, k+x.Offset)
		}
		if pa, ok := x.From.(*ir.PointeredArray); ok {
			return m.byteOf(f, pa, k+x.Offset)
		}
		return m.mem[(m.addrOf(f, x)+k)&0xffff]
	case *ir.PointeredArray:
		a := m.addrOf(f, x.From)
		return byte(a >> (8 * k))
	}
	unsupported("operand %T", o)
	return 0
}

// bytesOf は o の下位 n バイト (struct のコピーのように 8 バイトを超えることがある)。
func (m *machine) bytesOf(f *frame, o ir.Operand, n int) []byte {
	b := make([]byte, n)
	for k := range b {
		b[k] = m.byteOf(f, o, k)
	}
	return b
}

// writeBytes は o (変数・cast したその一部) に b を書く。
func (m *machine) writeBytes(f *frame, o ir.Operand, b []byte) {
	a := m.addrOf(f, o)
	for k, x := range b {
		m.store((a+k)&0xffff, x)
	}
}

func (m *machine) loadBytes(p, n int) []byte {
	b := make([]byte, n)
	for k := range b {
		b[k] = m.mem[(p+k)&0xffff]
	}
	return b
}

func (m *machine) storeBytes(p int, b []byte) {
	for k, x := range b {
		m.store((p+k)&0xffff, x)
	}
}

// read は o の下位 n バイト (n ≤ 8。演算の入力)。
func (m *machine) read(f *frame, o ir.Operand, n int) uint64 {
	var v uint64
	for k := 0; k < n; k++ {
		v |= uint64(m.byteOf(f, o, k)) << (8 * k)
	}
	return v
}

// write は o (変数・cast したその一部) に v の下位 n バイトを書く。
func (m *machine) write(f *frame, o ir.Operand, v uint64, n int) {
	a := m.addrOf(f, o)
	for k := 0; k < n; k++ {
		m.store((a+k)&0xffff, byte(v>>(8*k)))
	}
}

// store は 1 バイト書く。emu と同じく EMU_PRINT / EMU_EXIT への書き込みで出力・終了する。
func (m *machine) store(a int, b byte) {
	m.mem[a] = b
	switch a {
	case portPrint:
		switch b {
		case 1:
			p := int(m.mem[portAddr]) | int(m.mem[portAddr+1])<<8
			for m.mem[p&0xffff] != 0 {
				m.out.WriteByte(m.mem[p&0xffff])
				p++
			}
		case 2:
			fmt.Fprint(&m.out, int(m.mem[portData])|int(m.mem[portData+1])<<8)
		case 3:
			fmt.Fprint(&m.out, int(m.mem[portData])|int(m.mem[portData+1])<<8, " ")
		}
		m.mem[portPrint] = 255
	case portExit:
		if b != 255 {
			m.exited, m.exit = true, int(b)
		}
	}
}

func mask(n int) uint64 {
	if n >= 8 {
		return ^uint64(0)
	}
	return 1<<(8*n) - 1
}

// signed は n バイトの v を符号付きで読む。
func signed(v uint64, n int) int64 {
	v &= mask(n)
	if n < 8 && v&(1<<(8*n-1)) != 0 {
		return int64(v) - int64(1)<<(8*n)
	}
	return int64(v)
}

// ---------------------------------------------------------------
// 実行
// ---------------------------------------------------------------

// invoke は l を args (引数ごとのバイト列) で呼び、戻り値のバイト列を返す。
func (m *machine) invoke(l *ir.Lambda, args [][]byte) []byte {
	if l.Extern || l.Ops == nil && l.Body == nil {
		unsupported("extern function %s", l.Id)
	}
	f := &frame{lmd: l, vars: map[*ir.Value]int{}}
	saved := m.sp
	defer func() { m.sp = saved }()
	for _, v := range l.Vars {
		f.vars[v] = m.sp
		sz := v.Type.Size
		if sz < 0 {
			sz = 0
		}
		m.sp += sz
		if m.sp > stackEnd {
			panic(runtimeError("stack overflow"))
		}
	}
	for i, a := range l.Args {
		if i >= len(args) {
			break
		}
		for k := 0; k < a.Type.Size; k++ {
			var b byte
			if k < len(args[i]) {
				b = args[i][k]
			}
			m.mem[(f.vars[a]+k)&0xffff] = b
		}
	}
	m.run(f)
	if l.Result == nil || m.exited {
		return nil
	}
	n := l.Result.Type.Size
	r := make([]byte, n)
	copy(r, m.mem[f.vars[l.Result]:f.vars[l.Result]+n])
	return r
}

func (m *machine) labelIndex(l *ir.Lambda) map[string]int {
	if li, ok := m.labels[l]; ok {
		return li
	}
	li := map[string]int{}
	for i, op := range l.Ops {
		if op != nil && op.Code == ir.OpLabel {
			li[op.Label] = i
		}
	}
	m.labels[l] = li
	return li
}

// run は f の関数の命令を return (または末尾) まで実行する。
func (m *machine) run(f *frame) {
	ops := f.lmd.Ops
	labels := m.labelIndex(f.lmd)
	var calls []*call
	jump := func(label string) int {
		i, ok := labels[label]
		if !ok {
			unsupported("label %s not found in %s", label, f.lmd.Id)
		}
		return i
	}
	for pc := 0; pc < len(ops); pc++ {
		if m.exited {
			return
		}
		op := ops[pc]
		if op == nil {
			continue
		}
		m.steps++
		if m.maxSteps > 0 && m.steps > m.maxSteps {
			panic(ErrStepLimit)
		}
		switch op.Code {
		case ir.OpLabel:
		case ir.OpJump:
			pc = jump(op.Label)
		case ir.OpIfCarry, ir.OpIfNotCarry:
			if m.carry == (op.Code == ir.OpIfCarry) {
				pc = jump(op.Label)
			}
		case ir.OpRolC, ir.OpRorC:
			v := uint64(m.byteOf(f, op.Src[0], 0))
			in := uint64(0)
			if m.carry {
				in = 1
			}
			if op.Code == ir.OpRolC {
				m.carry = v&0x80 != 0
				v = (v<<1 | in) & 0xff
			} else {
				m.carry = v&1 != 0
				v = v>>1 | in<<7
			}
			m.write(f, op.Dst, v, 1)
		case ir.OpIf, ir.OpIfTrue:
			zero := m.read(f, op.Src[0], size(op.Src[0])) == 0
			if zero == (op.Code == ir.OpIf) {
				pc = jump(op.Label)
			}
		case ir.OpSwitch:
			lo, _ := ir.ValIntLiteral(op.Src[1])
			k := int(m.byteOf(f, op.Src[0], 0)-byte(lo)) & 0xff
			if k < len(op.Labels) {
				pc = jump(op.Labels[k])
			}
		case ir.OpReturn:
			if op.In(0) != nil && f.lmd.Result != nil {
				m.writeBytes(f, f.lmd.Result, m.bytesOf(f, op.Src[0], f.lmd.Result.Type.Size))
			}
			return
		case ir.OpPushResult, ir.OpPushFastcallResult:
			calls = append(calls, &call{})
		case ir.OpPushArg, ir.OpPushFastcallArg:
			if len(calls) == 0 {
				unsupported("push_arg without push_result")
			}
			b := m.bytesOf(f, op.Src[0], op.Type.Size)
			c := calls[len(calls)-1]
			c.args = append(c.args, b)
		case ir.OpCall, ir.OpFastcall:
			if len(calls) == 0 {
				unsupported("call without push_result")
			}
			c := calls[len(calls)-1]
			calls = calls[:len(calls)-1]
			callee := m.callee(f, op.Src[0])
			r := m.invoke(callee, c.args)
			if m.exited {
				return
			}
			if op.Dst != nil {
				b := make([]byte, size(op.Dst))
				copy(b, r)
				m.writeBytes(f, op.Dst, b)
			}
		case ir.OpLoad:
			m.writeBytes(f, op.Dst, m.bytesOf(f, op.Src[0], size(op.Dst)))
		case ir.OpSignExtension:
			n := size(op.Dst)
			if n > 2 {
				unsupported("sign_extension to %d bytes", n)
			}
			b := uint64(m.byteOf(f, op.Src[0], 0))
			if b&0x80 != 0 {
				b |= 0xff00
			}
			m.write(f, op.Dst, b, n)
		case ir.OpAdd, ir.OpSub, ir.OpAnd, ir.OpOr, ir.OpXor:
			n := size(op.Dst)
			a, b := m.read(f, op.Src[0], n), m.read(f, op.Src[1], n)
			var r uint64
			switch op.Code {
			case ir.OpAdd:
				r = a + b
			case ir.OpSub:
				r = a - b
			case ir.OpAnd:
				r = a & b
			case ir.OpOr:
				r = a | b
			case ir.OpXor:
				r = a ^ b
			}
			m.write(f, op.Dst, r, n)
		case ir.OpMul, ir.OpDiv, ir.OpMod:
			m.write(f, op.Dst, m.mulDivMod(f, op), size(op.Dst))
		case ir.OpShiftLeft, ir.OpShiftRight:
			m.write(f, op.Dst, m.shift(f, op), size(op.Dst))
		case ir.OpUminus:
			n := size(op.Dst)
			v := m.read(f, op.Src[0], min(n, size(op.Src[0])))
			m.write(f, op.Dst, -v, n)
		case ir.OpBitNot:
			n := size(op.Dst)
			m.write(f, op.Dst, ^m.read(f, op.Src[0], n), n)
		case ir.OpEq, ir.OpLt:
			n := max(size(op.Src[0]), size(op.Src[1]))
			a, b := m.read(f, op.Src[0], n), m.read(f, op.Src[1], n)
			var t bool
			if op.Code == ir.OpEq {
				t = a == b
			} else if ir.ValType(op.Src[0]).Signed || ir.ValType(op.Src[1]).Signed {
				t = signed(a, n) < signed(b, n)
			} else {
				t = a < b
			}
			m.writeBool(f, op.Dst, t)
		case ir.OpNot:
			m.writeBool(f, op.Dst, m.read(f, op.Src[0], size(op.Src[0])) == 0)
		case ir.OpIndex:
			m.write(f, op.Dst, uint64(m.elemAddr(f, op.Src[0], op.Src[1], false)), 2)
		case ir.OpRef:
			m.write(f, op.Dst, uint64(m.addrOf(f, op.Src[0])), 2)
		case ir.OpPget:
			p := int(m.read(f, op.Src[0], 2))
			m.writeBytes(f, op.Dst, m.loadBytes(p, size(op.Dst)))
		case ir.OpPset:
			p := int(m.read(f, op.Src[0], 2))
			m.storeBytes(p, m.bytesOf(f, op.Src[1], ir.ValType(op.Src[0]).Base.Size))
		case ir.OpIndexPget:
			p := m.elemAddr(f, op.Src[0], op.Src[1], op.Scaled)
			m.writeBytes(f, op.Dst, m.loadBytes(p, size(op.Dst)))
		case ir.OpIndexPset:
			p := m.elemAddr(f, op.Src[0], op.Src[1], op.Scaled)
			m.storeBytes(p, m.bytesOf(f, op.Src[2], ir.ValType(op.Src[0]).Base.Size))
		case ir.OpFieldPget:
			off, _ := ir.ValIntLiteral(op.Src[1])
			p := int(m.read(f, op.Src[0], 2)) + off
			m.writeBytes(f, op.Dst, m.loadBytes(p, size(op.Dst)))
		case ir.OpFieldPset:
			off, _ := ir.ValIntLiteral(op.Src[1])
			p := int(m.read(f, op.Src[0], 2)) + off
			m.storeBytes(p, m.bytesOf(f, op.Src[2], op.Type.Size))
		default:
			unsupported("op %s", op.Code)
		}
	}
}

// writeBool は比較の結果 (0 / 1) を Dst の最下位バイトにだけ書く (codegen と同じ)。
func (m *machine) writeBool(f *frame, dst ir.Operand, t bool) {
	var b uint64
	if t {
		b = 1
	}
	m.write(f, dst, b, 1)
}

// elemAddr は arr[idx] の番地 (arr は配列かポインタ)。scaled なら idx はバイト単位。
func (m *machine) elemAddr(f *frame, arr, idx ir.Operand, scaled bool) int {
	t := ir.ValType(arr)
	var base int
	switch t.Kind {
	case types.Array:
		base = m.addrOf(f, arr)
	case types.Pointer:
		base = int(m.read(f, arr, 2))
	default:
		unsupported("index of %s", t)
	}
	i := int(m.read(f, idx, size(idx)))
	if !scaled {
		i *= t.Base.Size
	}
	return (base + i) & 0xffff
}

// callee は呼び出しの対象 (関数のシンボル、または関数ポインタの値)。
func (m *machine) callee(f *frame, o ir.Operand) *ir.Lambda {
	var a int
	if lv, ok := o.(*ir.Value); ok && lv.Kind == ir.KindLiteral && !lv.IsInt {
		a = m.symAddr(f, lv.Symbol)
	} else {
		a = int(m.read(f, o, 2))
	}
	l, ok := m.lambdas[a]
	if !ok {
		panic(runtimeError(fmt.Sprintf("call to non-function $%04x", a)))
	}
	return l
}

// shift は shift_left / shift_right (codegen と同じく Dst の幅で。右シフトの符号は入力の型)。
func (m *machine) shift(f *frame, op *ir.Op) uint64 {
	n := size(op.Dst)
	v := m.read(f, op.Src[0], n)
	sgn := ir.ValType(op.Src[0]).Signed
	count, lit := ir.ValIntLiteral(op.Src[1])
	if !lit {
		if n != 1 {
			unsupported("variable shift of %d bytes", n)
		}
		count = int(m.byteOf(f, op.Src[1], 0))
	}
	for k := 0; k < count; k++ {
		if op.Code == ir.OpShiftLeft {
			m.carry = v&(1<<(8*n-1)) != 0
			v = (v << 1) & mask(n)
			continue
		}
		m.carry = v&1 != 0
		if sgn {
			v = uint64(signed(v, n)>>1) & mask(n)
		} else {
			v >>= 1
		}
	}
	return v
}

// mulDivMod は mul / div / mod (Dst の幅と符号で。除算は床除算、剰余は割る数の符号)。
func (m *machine) mulDivMod(f *frame, op *ir.Op) uint64 {
	n := size(op.Dst)
	if n > 2 {
		unsupported("%s of %d bytes", op.Code, n)
	}
	a, b := m.read(f, op.Src[0], n), m.read(f, op.Src[1], n)
	if op.Code == ir.OpMul {
		return a * b & mask(n)
	}
	if b == 0 {
		panic(runtimeError("division by zero"))
	}
	if !ir.ValType(op.Dst).Signed {
		if op.Code == ir.OpDiv {
			return a / b
		}
		return a % b
	}
	x, y := signed(a, n), signed(b, n)
	q := x / y
	if (x%y != 0) && ((x < 0) != (y < 0)) {
		q--
	}
	if op.Code == ir.OpDiv {
		return uint64(q) & mask(n)
	}
	return uint64(x-q*y) & mask(n)
}
