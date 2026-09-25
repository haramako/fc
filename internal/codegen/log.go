package codegen

// fc 3 の @log の地点 (doc/v3_plan.md §9、ir/log.go)。fcc build -g のとき、注釈 (Op.Logs) の付いた命令ごとに:
//
//   - IR のコメント行に印 `;@fclog N` を付けておき、ピープホールなどの後で、その行の前に地点のラベル `__fclog_N:` を置く
//     (印はコメントなので最適化には見えない。ラベルは ca65 の `@` ローカルラベルのスコープを切るので、地点のある関数だけ
//     `@x` を普通のラベル `__fcl_x` に書き換える。どちらもバイト列は変わらない)
//   - 引数の値の所在 (定数 / メモリの番地 / スタックフレーム (S + X) / レジスタ) を調べる。番地は関数の後ろに
//     `__fclog_N_K = 式` として書き、リンク後の値を dbgfile から引く (driver)。その地点で値が取れなければ「?」
//
// 値が取れるのは、その地点で生きている (後で読まれる) か、直前の命令から分岐・ラベルを挟まずにその変数を触っていて、
// 間に同じ場所を書く別の値が無いとき。値を生かすためにコードは変えない。

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/haramako/fc/internal/ir"
)

// LogSite は @log の地点 1 つ (同じ @log がインライン展開などで複数の地点になることもある)。
type LogSite struct {
	Label string // 地点のラベル (dbgfile のシンボル名)
	Point *ir.LogPoint
	Func  string // 関数のシンボル
	Locs  []LogLoc
	// Prevs は地点が合流点 (同じ番地の後ろのラベルへほかの経路から来る) のとき、この地点の経路の直前の命令のラベル:
	// 上から落ちてくる命令と、地点より前の同じ番地のラベルへの分岐。表示は直前にそのどれかを実行したときだけ (ループの
	// 先頭の @log を後ろからの辺で来た周では出さない、何も出さない else の地点を then の後で出さない)。nil なら毎回
	Prevs []string
	// Taken は分岐が成立したときだけの地点 (ir.LogPoint.OnTaken)。地点は飛び先 Target のラベルの直後、Prevs は分岐の命令
	Taken  bool
	Target string
}

// LogLoc は引数 1 つの、その地点での所在。
type LogLoc struct {
	Kind  string   // "const" / "mem" (Sym の番地) / "stack" (Sym の番地 + X) / "addr" (Sym の値そのもの) / "reg" / "bytes" / "none"
	Value int      // const の値
	Sym   string   // mem / stack: 番地のシンボル (リンク後の値は dbgfile)
	Expr  string   // その式 (.s に書く)
	Reg   string   // reg: "a" / "x" / "y"
	Why   string   // none の理由
	Parts []LogLoc // bytes: 下位から 1 バイトずつの所在
}

// logMarkers は op の注釈から地点を作り、IR のコメント行に付ける印を返す。
func (l *Llc) logMarkers(lmd *ir.Lambda, opNo int, op *ir.Op, live func() *ir.Liveness) string {
	var b strings.Builder
	for _, p := range op.Logs {
		n := len(l.LogSites)
		site := &LogSite{Label: fmt.Sprintf("__fclog_%d", n), Point: p, Func: lmd.Id}
		if p.OnTaken && op.Code != ir.OpJump && op.Code != ir.OpSwitch && op.Label != "" {
			site.Taken, site.Target = true, op.Label
		}
		for k, a := range p.Args {
			var loc LogLoc
			if len(a.Bytes) > 0 {
				// バイトごとの変数に分けた 2 バイトの値 (opt.splitWords)
				loc = LogLoc{Kind: "bytes"}
				for j, b := range a.Bytes {
					bl := l.logLoc(lmd, opNo, op, b, live, p.Moved)
					if bl.Kind == "mem" || bl.Kind == "stack" || bl.Kind == "addr" {
						bl.Sym = fmt.Sprintf("%s_%d_%d", site.Label, k, j)
					}
					loc.Parts = append(loc.Parts, bl)
				}
			} else {
				loc = l.logLoc(lmd, opNo, op, a.Val, live, p.Moved)
				if loc.Kind == "mem" || loc.Kind == "stack" || loc.Kind == "addr" {
					loc.Sym = fmt.Sprintf("%s_%d", site.Label, k)
				}
			}
			if site.Taken && loc.Kind == "reg" {
				// 成立したときの地点は分岐の後: 分岐の前の比較 (lda など) がレジスタを書き換えているかもしれない
				loc = LogLoc{Kind: "none", Why: "in a register"}
			}
			site.Locs = append(site.Locs, loc)
		}
		l.LogSites = append(l.LogSites, site)
		if site.Taken {
			fmt.Fprintf(&b, " ;@fclogt %d", n)
		} else {
			fmt.Fprintf(&b, " ;@fclog %d", n)
		}
	}
	return b.String()
}

// logLoc は値 v の地点 opNo (命令を実行する直前) での所在。
func (l *Llc) logLoc(lmd *ir.Lambda, opNo int, op *ir.Op, v ir.Operand, live func() *ir.Liveness, moved bool) LogLoc {
	if lit := ir.ValLiteral(v); lit != nil && ir.ValKind(v) == ir.KindLiteral {
		if lit.IsInt {
			return LogLoc{Kind: "const", Value: lit.Int}
		}
		return LogLoc{Kind: "addr", Expr: mangle(lit.Symbol)} // 関数などのアドレスそのもの
	}
	u := ir.UnderlyingValue(v)
	if u == nil {
		return LogLoc{Kind: "none", Why: "not a variable"}
	}
	// ループ内の常駐 (この命令の入口でレジスタにある。グローバルも、Home がそのグローバルの一時変数が常駐する)
	for _, r := range []struct {
		res *ir.Value
		in  bool
		reg string
	}{{op.Resident, op.ResIn, "a"}, {op.ResidentY, op.ResYIn, "y"}, {op.ResidentX, op.ResXIn, "x"}} {
		if r.res == nil || !r.in || ir.ValOffset(v) != 0 {
			continue
		}
		if r.res == u || (r.res.Home != nil && ir.UnderlyingValue(r.res.Home) == u && ir.ValOffset(r.res.Home) == 0) {
			return LogLoc{Kind: "reg", Reg: r.reg}
		}
	}
	// 常駐のメモリ側 (Home) の変数: 常駐の値がレジスタで更新され、死んだ後は書き戻されないので、メモリ側が正しいのは
	// 常駐の値が生きていてここでは常駐していないとき (メモリ側に退避している) か、メモリ側の変数そのものが生きているとき
	if rs := residentsOf(lmd, u); len(rs) > 0 {
		ok := live().LiveIn(opNo, u)
		for _, r := range rs {
			ok = ok || live().LiveIn(opNo, r)
		}
		if !ok {
			return LogLoc{Kind: "none", Why: "optimized out"}
		}
		if u.Kind == ir.KindGlobal {
			return LogLoc{Kind: "mem", Expr: l.toAsm(v)}
		}
		return l.memLoc(v)
	}
	if u.Kind == ir.KindGlobal {
		return LogLoc{Kind: "mem", Expr: l.toAsm(v)}
	}
	if u.Kind != ir.KindLocal {
		return LogLoc{Kind: "none", Why: "not a variable"}
	}
	isLive := live().LiveIn(opNo, u)
	// 付け替えた注釈の地点は元の地点と違う: 間の代入が消えている (LogStale) と、生きていても元の地点の値ではない
	if u.LogNoValue || (moved && u.LogStale) || (!isLive && (u.LogStale || moved || !recentlyTouched(lmd.Ops, opNo, u))) {
		return LogLoc{Kind: "none", Why: "optimized out"}
	}
	switch ir.ValLocation(u) {
	case ir.LocA, ir.LocY, ir.LocX:
		if u.Home == nil {
			// レジスタに割り付けた変数: 生きている間はそのレジスタにある (死んだ後は壊れているかもしれない)
			if !isLive || ir.ValOffset(v) != 0 {
				return LogLoc{Kind: "none", Why: "optimized out"}
			}
			return LogLoc{Kind: "reg", Reg: map[ir.Location]string{ir.LocA: "a", ir.LocY: "y", ir.LocX: "x"}[ir.ValLocation(u)]}
		}
		// 常駐していない命令: 値はメモリ側
		if cv, ok := v.(*ir.CastedValue); ok {
			v = ir.RebaseCast(cv, u.Home)
		} else {
			v = u.Home
		}
		return l.memLoc(v)
	case ir.LocFrame, ir.LocReg, ir.LocFastcallReg, ir.LocStatic:
		return l.memLoc(v)
	}
	return LogLoc{Kind: "none", Why: "in a register"}
}

// residentsOf は u をメモリ側 (Home) に持つ常駐の値 (regalloc.AllocateResident)。
func residentsOf(lmd *ir.Lambda, u *ir.Value) []*ir.Value {
	var r []*ir.Value
	for _, v := range lmd.Vars {
		if v.Home != nil && ir.UnderlyingValue(v.Home) == u {
			r = append(r, v)
		}
	}
	return r
}

// memLoc はメモリにある値の番地 (toAsm の表記から `<` と `,x` を除いたもの)。
func (l *Llc) memLoc(v ir.Operand) LogLoc {
	a := l.toAsm(v)
	kind := "mem"
	if strings.HasSuffix(a, ",x") {
		kind, a = "stack", strings.TrimSuffix(a, ",x")
	}
	return LogLoc{Kind: kind, Expr: strings.TrimPrefix(a, "<")}
}

// recentlyTouched は u が死んでいても値が場所に残っているか: opNo の前を分岐・ラベル・呼び出しを越えずに遡って u の定義か
// 使用に着き、その間に u と同じ場所を書く別の値が無い。
func recentlyTouched(ops []*ir.Op, opNo int, u *ir.Value) bool {
	if ops[opNo].Code == ir.OpLabel {
		return false // 地点が合流点 (ほかの経路からも来る)
	}
	for j := opNo - 1; j >= 0; j-- {
		op := ops[j]
		if op == nil {
			continue
		}
		switch op.Code {
		case ir.OpLabel, ir.OpJump, ir.OpIf, ir.OpIfTrue, ir.OpIfCarry, ir.OpIfNotCarry, ir.OpSwitch, ir.OpReturn, ir.OpAsm:
			return false
		}
		if isCallOp(op) {
			return false // 呼び先が L などを使う
		}
		defs, uses := ir.DefUse(op)
		for _, d := range defs {
			dv := ir.UnderlyingValue(d)
			if dv == u {
				return true
			}
			if dv != nil && logSameStorage(dv, u) {
				return false
			}
		}
		for _, s := range uses {
			if ir.UnderlyingValue(s) == u {
				return true
			}
		}
	}
	return false
}

// logSameStorage は a と b の場所が重なるか (同じ種類の場所で番地の範囲が重なる)。
func logSameStorage(a, b *ir.Value) bool {
	if a.Kind != ir.KindLocal || a.Location != b.Location {
		return false
	}
	switch a.Location {
	case ir.LocFrame, ir.LocReg, ir.LocFastcallReg, ir.LocStatic:
		return a.Address < b.Address+b.Type.Size && b.Address < a.Address+a.Type.Size
	case ir.LocA, ir.LocX, ir.LocY:
		return true
	}
	return false
}

var (
	reLogMark      = regexp.MustCompile(`;@fclog (\d+)`)
	reLogTakenMark = regexp.MustCompile(`;@fclogt (\d+)`)
	reIRComment    = regexp.MustCompile(`^; \d{4}: `)
	reCheapLabel   = regexp.MustCompile(`@([A-Za-z0-9_]+)`)
)

// placeLogLabels は印の付いたコメント行の前に地点のラベルを置き、番地の式を関数の後ろに書く。
// ラベルは `@` ローカルラベルのスコープを切るので、この関数の `@x` を普通のラベルに書き換える。
func (l *Llc) placeLogLabels(lines []string, sites []*LogSite) []string {
	out := make([]string, 0, len(lines)+len(sites)*2)
	for _, line := range lines {
		isComment := strings.HasPrefix(strings.TrimSpace(line), ";")
		if isComment {
			for _, m := range reLogMark.FindAllStringSubmatch(line, -1) {
				out = append(out, "__fclog_"+m[1]+":")
			}
		} else {
			line = reCheapLabel.ReplaceAllString(line, "__fcl_$1")
		}
		out = append(out, line)
	}
	byLabel := map[string]*LogSite{}
	for _, s := range sites {
		byLabel[s.Label] = s
	}
	out = markLogPrevs(out, byLabel)
	out = markTakenLogs(out, byLabel)
	for _, s := range sites {
		for _, loc := range s.Locs {
			for _, l := range append([]LogLoc{loc}, loc.Parts...) {
				if l.Sym != "" {
					out = append(out, fmt.Sprintf("%s = %s", l.Sym, l.Expr))
				}
			}
		}
	}
	return out
}

// asmLine の種類 (markLogPrevs 用)。
func asmLabel(t string) (string, bool) {
	if strings.HasSuffix(t, ":") && !strings.HasPrefix(t, ";") && !strings.Contains(t, " ") {
		return strings.TrimSuffix(t, ":"), true
	}
	return "", false
}

func asmIsInstr(t string) bool {
	if t == "" || strings.HasPrefix(t, ";") || strings.HasPrefix(t, ".") {
		return false
	}
	_, lab := asmLabel(t)
	return !lab
}

// markTakenLogs は分岐が成立したときだけの地点 (`;@fclogt N` の印の命令) を置く: 地点のラベルは飛び先のラベルの直後、
// Prevs はその命令の中の飛び先への分岐 (extendJump で jmp になったものも)。見つからなければ地点を置かない (出さない)。
func markTakenLogs(out []string, byLabel map[string]*LogSite) []string {
	type mark struct {
		at    int
		label string
	}
	var marks []mark
	for i, line := range out {
		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, ";") {
			continue
		}
		for _, m := range reLogTakenMark.FindAllStringSubmatch(t, -1) {
			s := byLabel["__fclog_"+m[1]]
			if s == nil {
				continue
			}
			target := reCheapLabel.ReplaceAllString(mangle(s.Target), "__fcl_$1")
			// この命令の範囲 (次の IR のコメント行まで) の、飛び先への分岐
			var branches []int
			for j := i + 1; j < len(out); j++ {
				u := strings.TrimSpace(out[j])
				if reIRComment.MatchString(u) {
					break
				}
				if f := strings.Fields(u); len(f) == 2 && branchMnems[strings.ToLower(f[0])] && f[1] == target {
					branches = append(branches, j)
				}
			}
			at := -1
			for j, line := range out {
				if strings.TrimSpace(line) == target+":" {
					at = j + 1
				}
			}
			if at < 0 || len(branches) == 0 {
				continue
			}
			for k, j := range branches {
				pl := fmt.Sprintf("%s_p%d", s.Label, k)
				s.Prevs = append(s.Prevs, pl)
				marks = append(marks, mark{j, pl})
			}
			marks = append(marks, mark{at, s.Label})
		}
	}
	if len(marks) == 0 {
		return out
	}
	sort.SliceStable(marks, func(a, b int) bool { return marks[a].at < marks[b].at })
	r := make([]string, 0, len(out)+len(marks))
	k := 0
	for i, line := range out {
		for k < len(marks) && marks[k].at == i {
			r = append(r, marks[k].label+":")
			k++
		}
		r = append(r, line)
	}
	for ; k < len(marks); k++ {
		r = append(r, marks[k].label+":")
	}
	return r
}

var branchMnems = map[string]bool{"jmp": true, "bcc": true, "bcs": true, "beq": true, "bne": true, "bmi": true, "bpl": true, "bvc": true, "bvs": true}

// markLogPrevs は合流点の地点 (地点と次の命令の間に別のラベルがある) に、この地点の経路の直前の命令のラベルを付ける
// (LogSite.Prevs)。直前の命令: 上から落ちてくる命令 (jmp / rts / rti / brk 以外) と、地点の前の同じ番地のラベルへの分岐。
// そのラベルが分岐以外 (ジャンプ表など) から参照されていれば、経路が分からないので付けない (毎回出す)。
func markLogPrevs(out []string, byLabel map[string]*LogSite) []string {
	type mark struct {
		at    int // この行の前にラベルを置く
		label string
	}
	var marks []mark
	for i, line := range out {
		t := strings.TrimSpace(line)
		name, ok := asmLabel(t)
		if !ok || byLabel[name] == nil {
			continue
		}
		s := byLabel[name]
		// 後ろの同じ番地のラベル (合流点) があるか
		merge := false
		for j := i + 1; j < len(out); j++ {
			u := strings.TrimSpace(out[j])
			if asmIsInstr(u) {
				break
			}
			if n, ok := asmLabel(u); ok && !strings.HasPrefix(n, "__fclog_") {
				merge = true
			}
		}
		if !merge {
			continue
		}
		// 前の同じ番地のラベル (この地点の経路の入口) と、上から落ちてくる直前の命令
		own := map[string]bool{}
		prev := -1
		for j := i - 1; j >= 0; j-- {
			u := strings.TrimSpace(out[j])
			if asmIsInstr(u) {
				prev = j
				break
			}
			if n, ok := asmLabel(u); ok && !strings.HasPrefix(n, "__fclog_") {
				own[n] = true
			}
		}
		var at []int
		if prev >= 0 {
			if f := strings.Fields(strings.TrimSpace(out[prev])); !(f[0] == "jmp" || f[0] == "rts" || f[0] == "rti" || f[0] == "brk") {
				at = append(at, prev)
			}
		}
		unknown := false
		for j, line := range out {
			u := strings.TrimSpace(line)
			if !asmIsInstr(u) && !strings.HasPrefix(u, ".") {
				continue
			}
			f := strings.Fields(u)
			for k, tok := range f {
				tok = strings.Trim(tok, "#<>(),")
				if !own[tok] {
					continue
				}
				if k == 1 && branchMnems[strings.ToLower(f[0])] {
					at = append(at, j)
				} else {
					unknown = true // ジャンプ表・アドレスとしての参照
				}
			}
		}
		if unknown {
			continue
		}
		for k, j := range at {
			pl := fmt.Sprintf("%s_p%d", s.Label, k)
			s.Prevs = append(s.Prevs, pl)
			marks = append(marks, mark{j, pl})
		}
		if len(at) == 0 {
			s.Prevs = []string{} // この経路からは来ない (出さない)
		}
	}
	if len(marks) == 0 {
		return out
	}
	sort.Slice(marks, func(a, b int) bool { return marks[a].at < marks[b].at })
	r := make([]string, 0, len(out)+len(marks))
	k := 0
	for i, line := range out {
		for k < len(marks) && marks[k].at == i {
			r = append(r, marks[k].label+":")
			k++
		}
		r = append(r, line)
	}
	return r
}
