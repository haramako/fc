package ir

// fc 3 の @log (エミュレータ側で表示するログ地点。doc/v3_plan.md §9)。
//
// @log は命令を出さない。sema が次に出す命令に注釈 (Op.Logs) として付け、「その命令を実行する直前」の地点を表す。
// 注釈は最適化の判断に数えない (隣り合う命令・ラベル・値の生存のどれにも出てこない) ので、`@log` があっても無くても
// 生成コードは同じ。命令が消えたり作り直されたりしたら KeepLogs が後ろの命令へ付け替える。引数の値 (LogArg.Val) は
// 弱い参照で、使用に数えない。コード生成がその地点での所在 (メモリ / レジスタ / 定数) を調べ、取れなければ「?」にする。

import (
	"fmt"
	"strings"

	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// LogPoint はソースの @log 1 つ。インライン展開・ループ展開で写されると、写しごとに別の LogPoint (同じ ID) になる。
type LogPoint struct {
	ID     int             // プログラム全体の通し番号 (写しでも同じ)
	Pos    syntax.Position // @log の位置
	Format string          // 書式の元の文字列 (表示用)
	Parts  []LogPart
	Args   []*LogArg
	// OnTaken は条件分岐に付いた注釈で、分岐が成立したときだけの地点 (飛び先の直前。jumps が素通りしたブロックの注釈)
	OnTaken bool
	// Moved は最適化で元の命令から別の命令へ付け替えた注釈。コード生成は、その地点で生きている値だけを読む (「直前に
	// 触った」の規則は元の命令の地点でしか確かでない)
	Moved bool
}

// LogPart は書式の 1 片: 文字列 (Arg < 0) か、引数の表示 (Arg 番目を Spec で)。
type LogPart struct {
	Text string
	Arg  int
	Spec LogSpec
}

// LogSpec は `{:04x}` の `:` の後ろ。
type LogSpec struct {
	Verb  byte // 0 (型に従う) / 'd' / 'x' / 'X' / 'b' / 'c'
	Width int
	Zero  bool // 0 で埋める
}

// LogArg は @log の引数 1 つ。
type LogArg struct {
	Expr string      // ソースの綴り (「?」の警告に使う)
	Type *types.Type // 表示に使う型 (整数 / bool / enum / ポインタ)
	Val  Operand     // 値 (弱い参照。定数ならリテラル)
	// Bytes は 2 バイトの変数がバイトごとの変数に分けられたとき (opt.splitWords) の、下位・上位の値。あれば Val より優先
	Bytes []Operand
}

// cloneLog は写した命令用に LogPoint を写す (引数の値は mapOperand で付け替える)。
func cloneLog(p *LogPoint, mapOperand func(Operand) Operand) *LogPoint {
	np := *p
	np.Args = make([]*LogArg, len(p.Args))
	for i, a := range p.Args {
		na := *a
		if mapOperand != nil {
			na.Val = mapOperand(a.Val)
			if len(a.Bytes) > 0 {
				na.Bytes = make([]Operand, len(a.Bytes))
				for k, b := range a.Bytes {
					na.Bytes[k] = mapOperand(b)
				}
			}
		}
		np.Args[i] = &na
	}
	return &np
}

// CloneLogs は命令を写すとき (インライン展開・ループ展開) の注釈の写し。値を付け替えないなら mapOperand は nil。
func CloneLogs(logs []*LogPoint, mapOperand func(Operand) Operand) []*LogPoint {
	if len(logs) == 0 {
		return nil
	}
	r := make([]*LogPoint, len(logs))
	for i, p := range logs {
		r[i] = cloneLog(p, mapOperand)
	}
	return r
}

// HasLogs は関数に @log の注釈があるか。
func HasLogs(ops []*Op) bool {
	for _, op := range ops {
		if op != nil && len(op.Logs) > 0 {
			return true
		}
	}
	return false
}

// SnapshotLogs は命令列を書き換える前の並び (KeepLogs に渡す)。注釈が無ければ nil (何もしない)。
func SnapshotLogs(lmd *Lambda) []*Op {
	if !HasLogs(lmd.Ops) {
		return nil
	}
	return append([]*Op(nil), lmd.Ops...)
}

// KeepLogs は書き換えの後、消えた命令の注釈を「元の位置の前で残った命令の直後」へ付け替える (パスが DropOp などで
// 付け替えなかったもの)。prev は書き換える前の並び (SnapshotLogs)。書き換えで作り直された命令 (融合など) は元の位置に
// 来るのでそれが引き取り、消えただけなら次の命令が引き取る。残った命令の注釈は動かさない (命令を移すパスは自分で残す)。
func KeepLogs(lmd *Lambda, prev []*Op) {
	if prev == nil {
		return
	}
	pos := map[*Op]int{}
	present := map[*LogPoint]bool{} // 書き換えの後も残っている注釈 (パスが DropOp などで付け替えた先で)
	for i, op := range lmd.Ops {
		if op != nil {
			pos[op] = i
			for _, p := range op.Logs {
				present[p] = true
			}
		}
	}
	// k より前で残った命令の新しい位置 (無ければ -1)
	survivorBefore := func(k int) int {
		for j := k - 1; j >= 0; j-- {
			if q := prev[j]; q != nil {
				if n, ok := pos[q]; ok {
					return n
				}
			}
		}
		return -1
	}
	type move struct {
		logs  []*LogPoint
		after int // この位置 (新しい並び) の後ろの最初の命令へ
	}
	var moves []move
	for k, op := range prev {
		if op == nil || len(op.Logs) == 0 {
			continue
		}
		if _, ok := pos[op]; ok {
			continue // 命令が残っている (ブロックごと並べ替えられても、注釈は命令と一緒に動く)
		}
		var lost []*LogPoint
		for _, p := range op.Logs {
			if !present[p] { // パスが自分で付け替えた (DropOp など) ものは除く
				lost = append(lost, p)
			}
		}
		if len(lost) == 0 {
			continue
		}
		moves = append(moves, move{logs: lost, after: survivorBefore(k)})
	}
	// 後ろの地点から付け替える (同じ命令に付くときの順番を保つ: 前の @log が先)
	for m := len(moves) - 1; m >= 0; m-- {
		mv := moves[m]
		target := -1
		for i := mv.after + 1; i < len(lmd.Ops); i++ {
			if lmd.Ops[i] != nil {
				target = i
				break
			}
		}
		if target < 0 {
			for i := len(lmd.Ops) - 1; i >= 0; i-- {
				if lmd.Ops[i] != nil {
					target = i
					break
				}
			}
		}
		if target < 0 {
			continue // 命令が 1 つも無い
		}
		PrependLogs(lmd.Ops[target], mv.logs)
	}
}

// logKey は注釈の同一性 (同じ @log で引数の値も同じ)。ループ展開の写しがジャンプの連鎖の畳み込みなどで同じ命令に
// 集まったとき、同じものは 1 つにする (同じ地点で 2 回出さない)。引数の値が違う写し (畳まれたループの各周) は残す。
func logKey(p *LogPoint) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d", p.ID)
	for _, a := range p.Args {
		b.WriteString("|" + OperandString(a.Val))
		for _, x := range a.Bytes {
			b.WriteString("," + OperandString(x))
		}
	}
	return b.String()
}

// PrependLogs は op の注釈の前に logs を足す (同じもの (logKey) が既にあれば足さない)。
func PrependLogs(op *Op, logs []*LogPoint) {
	have := map[string]bool{}
	for _, p := range op.Logs {
		have[logKey(p)] = true
	}
	var add []*LogPoint
	for _, p := range logs {
		if k := logKey(p); !have[k] {
			have[k] = true
			p.Moved = true
			add = append(add, p)
		}
	}
	if len(add) > 0 {
		op.Logs = append(add, op.Logs...)
	}
}

// AppendLogs は op の注釈の後ろに logs を足す (同じものは足さない)。
func AppendLogs(op *Op, logs []*LogPoint) {
	have := map[string]bool{}
	for _, p := range op.Logs {
		have[logKey(p)] = true
	}
	for _, p := range logs {
		if k := logKey(p); !have[k] {
			have[k] = true
			p.Moved = true
			op.Logs = append(op.Logs, p)
		}
	}
}

// DropOp は ops[i] を消す (nil にする)。注釈は次に実行される命令へ移す: 無条件のジャンプなら飛び先のラベル、それ以外は
// 次に残っている命令 (後ろに無ければ前の命令)。
func DropOp(ops []*Op, i int) {
	op := ops[i]
	ops[i] = nil
	if op == nil || len(op.Logs) == 0 {
		return
	}
	switch op.Code {
	case OpIf, OpIfTrue, OpIfCarry, OpIfNotCarry:
		// 消す条件分岐 (成立しないと分かった): 成立したときの注釈は出さない
		var rest []*LogPoint
		for _, p := range op.Logs {
			if !p.OnTaken {
				rest = append(rest, p)
			}
		}
		op.Logs = rest
		if len(rest) == 0 {
			return
		}
	}
	if op.Code == OpJump {
		for _, t := range ops {
			if t != nil && t.Code == OpLabel && t.Label == op.Label {
				PrependLogs(t, op.Logs)
				return
			}
		}
	}
	// ラベルとラベルの間 (何もしない else の本体など): 後ろのラベルは合流点かもしれないので、前のラベル (この経路の入口) へ
	next, prev := -1, -1
	for j := i + 1; j < len(ops); j++ {
		if ops[j] != nil {
			next = j
			break
		}
	}
	for j := i - 1; j >= 0; j-- {
		if ops[j] != nil {
			prev = j
			break
		}
	}
	if next >= 0 && prev >= 0 && ops[next].Code == OpLabel && ops[prev].Code == OpLabel {
		AppendLogs(ops[prev], op.Logs)
		return
	}
	for j := i + 1; j < len(ops); j++ {
		if ops[j] != nil {
			PrependLogs(ops[j], op.Logs)
			return
		}
	}
	for j := i - 1; j >= 0; j-- {
		if ops[j] != nil {
			AppendLogs(ops[j], op.Logs)
			return
		}
	}
}

// DiscardOp は到達しない命令を消す (注釈も捨てる: その地点には来ないので。KeepLogs も付け替えない)。
func DiscardOp(ops []*Op, i int) {
	if op := ops[i]; op != nil {
		op.Logs = nil
	}
	ops[i] = nil
}

// MergeDrop は ops[j] を消し、その注釈を ops[i] (j を取り込んだ命令。融合など) へ移す。
func MergeDrop(ops []*Op, i, j int) {
	if op := ops[j]; op != nil && len(op.Logs) > 0 && ops[i] != nil {
		AppendLogs(ops[i], op.Logs)
	}
	ops[j] = nil
}

// ReplaceOp は ops[i] を op に置き換える (元の命令の注釈は op が引き取る)。
func ReplaceOp(ops []*Op, i int, op *Op) {
	if old := ops[i]; old != nil && len(old.Logs) > 0 && old != op {
		op.Logs = append(append([]*LogPoint(nil), old.Logs...), op.Logs...)
	}
	if op.Code == OpJump {
		for _, p := range op.Logs {
			p.OnTaken = false // 必ず成立する分岐になった
		}
	}
	ops[i] = op
}
