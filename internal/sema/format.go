package sema

// @format(dst, "書式", 引数...) と fc 4 の printf("書式", 引数...) (Agent/wiki/plans/v4-stdlib.md §4)。
//
// @format は dst ([]u8) に書式どおりに書き、書いた部分の slice を返す (snprintf に当たる)。書式は定数の文字列で、コンパイル時に
// 分解して fmt モジュールの関数の呼び出しの並びにする (`fmt.begin(dst); fmt.str("HP "); fmt.dec_u8(hp, 3); ...` と、fmt.at を
// 長さにした dst の slice。実行時に書式を読まない)。書式は @log と同じ (parseLogFormat): {} / {0} / {:x} / {:04X} / {:b} /
// {:c} / {:d} / {:5}、{{ / }}。文字の部分と、定数の引数 (整数・bool・文字列) はコンパイル時に文字にしてまとめる。引数は begin の
// 前に全部評価する (引数の中の @format が書き先の状態を壊さないように)。dst が足りなければ fmt の関数が止まる。
//
// printf は console に出すラッパー: 実行時に長さの決まる文字列 (slice・*u8) の引数は console.write / write_z に直に渡し、その間の
// 部分は、長さの最大をコンパイル時に数えた一時バッファに @format と同じく書いてから console.write に渡す (定数だけの部分は文字の
// まま渡す)。fc 2 / fc 3 のモジュールの printf は今までどおり (引数を並べる形で stdio に出す: builtins.go)。
//
// 引数の型と書き方: 整数は型の符号で 10 進 (x / X / b は同じ大きさの符号なしとして)、bool は true / false ({:d} なら 0 / 1)、
// enum は数、c は 1 バイトの整数を 1 文字、u8 の slice はそのまま、u8 の配列は中の最初の 0 まで、*u8 は終端 0 まで。幅は数だけ。
// fmt / console は使う所で読み込む (`use` が無くてもよい)。
//
// 独自のフォント: 書式に textmap の変換器の呼び出しを書くと (`@format(buf, _T("HP {:3}"), hp)`)、原文を .po で翻訳してから
// `{…}` を解析し、文字の部分だけを文字コードにする (textmap は ASCII の記号を全角にするので、先に解析する)。数字などは変換器で
// 作った 24 文字の表を begin の後に fmt.codes に置いて、実行時の数もフォントのコードで書く。

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

func registerFormatBuiltins(h *Hlc) {
	h.defmacroTyped("@format", formatMacro, func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		return macroResult{expr: cv(h.format(args, "@format"))}
	})
	// @try_format(dst, "書式", 引数...): @format と同じだが、dst が足りなければ止まらず長さ 0 の slice を返す (書式を空にしない
	// 限り失敗と区別できる。`vram.put(a, @try_format(buf, ...))` は失敗なら何も書かない)
	h.defmacroTyped("@try_format", formatMacro, func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		return macroResult{expr: cv(h.format(args, "@try_format"))}
	})
}

// fclib/fmt.fc の spec のビット (幅は下位 5 ビット)
const (
	fmtMaxWidth = 0x1f
	fmtZero     = 0x20
	fmtLower    = 0x40
)

// fmtArgs は評価した書式の引数と分解した書式。
type fmtArgs struct {
	what  string
	parts []ir.LogPart
	vals  []ir.Operand
	texts []*string // 定数の文字列の引数はその綴り
	codes []byte    // textmap の変換器の書式なら、fmtDigits の文字のコード (fmt.codes に置く。nil なら ASCII)
	tm    *textmapConv
}

// fmtDigits は fmt.codes の表の文字の並び (fclib/fmt.fc の ASCII と同じ)。
const fmtDigits = "0123456789ABCDEFabcdef- "

// builtinModule は組み込みが使うモジュール (読み込んでいなければここで読み込む)。今のモジュールが use したことにする (asm が
// そのモジュールの .inc を取り込み、シンボルを import するように)。
func (h *Hlc) builtinModule(name string) *ModuleInterface {
	var mi *ModuleInterface
	if m, ok := h.prog.Modules.Get(name); ok {
		mi = h.prog.iface(m)
	} else {
		mi = h.useModule(name)
	}
	if h.module != nil && h.module.Id != name {
		h.module.AddUse(mi.Id)
	}
	return mi
}

// fmtPrepare は書式 (定数の文字列の式) を取り出し、引数を左から評価して、書式を分解する。
func (h *Hlc) fmtPrepare(what string, format *cexpr, args []*cexpr) *fmtArgs {
	a := &fmtArgs{what: what, vals: make([]ir.Operand, len(args)), texts: make([]*string, len(args))}
	var formatText string
	if format.kind == cOp && format.op == opCall && len(format.args) >= 2 {
		a.tm = h.constEval(format.args[0]).textmapOf()
	}
	if a.tm != nil {
		formatText = a.tm.text(h, format.args[1:]) // 翻訳は `{…}` を含んだ書式の全体で
	} else {
		fc := h.constEval(format)
		if fc.kind != cValue || !fc.val.IsString {
			panic(&diag.Error{Msg: what + ": the format must be a constant string (a string literal or a const; it is split at compile time)"})
		}
		formatText = fc.val.Str
	}
	largs := make([]*ir.LogArg, len(args))
	for i, c := range args {
		if x := h.constEval(c); x.kind == cValue && x.val.IsString {
			s := x.val.Str
			a.texts[i] = &s
		}
		v := h.rval(c)
		h.checkMixedUse(v, "printing it")
		if t := ir.ValType(v); t.Kind != types.Array {
			v = h.freeze(v)
		}
		a.vals[i] = v
		largs[i] = &ir.LogArg{Expr: fmt.Sprintf("#%d", i), Type: ir.ValType(v)}
	}
	parts, err := parseLogFormat(formatText, largs)
	if err != nil {
		panic(&diag.Error{Msg: fmt.Sprintf("%s: %v", what, err)})
	}
	if a.tm != nil {
		dc := a.tm.codes(h, fmtDigits)
		if len(dc) != len(fmtDigits) {
			panic(&diag.Error{Msg: fmt.Sprintf("%s: the character table must map each of %q to one code (for the digits)", what, fmtDigits)})
		}
		a.codes = codeBytes(what, dc)
		for i := range parts {
			if parts[i].Arg < 0 {
				parts[i].Text = string(codeBytes(what, a.tm.codes(h, parts[i].Text))) // 文字の部分だけを文字コードに
			} else if t := ir.ValType(a.vals[parts[i].Arg]); t.Kind == types.Bool && parts[i].Spec.Verb == 0 {
				panic(&diag.Error{Msg: fmt.Sprintf("%s: {} of a bool writes true / false in ASCII; use {:d} with a character table", what)})
			}
		}
	}
	a.parts = parts
	return a
}

// codeBytes は文字コードの並びをバイトにする (1 バイトに入らなければエラー)。
func codeBytes(what string, codes []int) []byte {
	b := make([]byte, len(codes))
	for i, c := range codes {
		if c < 0 || c > 255 {
			panic(&diag.Error{Msg: fmt.Sprintf("%s: character code %d does not fit in a byte", what, c)})
		}
		b[i] = byte(c)
	}
	return b
}

// setCodes は fmt.codes を textmap の数字などの表にする (begin の後。ASCII なら何もしない)。表はモジュールに 1 つ。
func (h *Hlc) setCodes(a *fmtArgs) {
	if a.codes == nil {
		return
	}
	key := h.module.Id + "\x00" + string(a.codes)
	sym := h.prog.fmtCodes[key]
	if sym == "" {
		sym = fmt.Sprintf("_%s__fmt_codes%d", h.module.Id, len(h.prog.fmtCodes))
		u8 := h.prog.Types.IntType(1, false)
		elems := make([]ir.Operand, len(a.codes))
		for i, c := range a.codes {
			elems[i] = ir.NewIntLiteral("", u8, int(c))
		}
		h.addDefModule(&ir.Def{Kind: ir.DefBlock, Sym: sym, Type: h.prog.Types.ArrayOf(u8, len(elems)), Elems: elems})
		h.prog.fmtCodes[key] = sym
	}
	fi := h.builtinModule("fmt")
	codes := h.moduleFunc(fi, "codes")
	h.emit(&ir.Op{Code: ir.OpLoad, Dst: codes, Src: []ir.Operand{ir.NewSymbolLiteral("", ir.ValType(codes), sym)}})
}

// noWidth は幅を持てない引数 (文字列・文字) に幅が付いていればエラー。
func (a *fmtArgs) noWidth(p ir.LogPart, kind string) {
	if p.Spec.Width != 0 || p.Spec.Zero {
		panic(&diag.Error{Msg: fmt.Sprintf("%s: a width is for numbers (argument %d is %s)", a.what, p.Arg, kind)})
	}
}

// constText は p がコンパイル時に文字にできるなら (文字の部分・定数の文字列・定数の整数 / bool) その文字を返す。
func (a *fmtArgs) constText(p ir.LogPart) (string, bool) {
	if p.Arg < 0 {
		return p.Text, true
	}
	if a.texts[p.Arg] != nil {
		a.noWidth(p, "a string")
		return *a.texts[p.Arg], true
	}
	v := a.vals[p.Arg]
	t := ir.ValType(v)
	if k, lit := ir.ValIntLiteral(v); lit && p.Spec.Verb != 'c' && (t.Kind == types.Int || t.Kind == types.Bool) {
		s := formatConst(k, t, p.Spec)
		if a.codes != nil { // 数字などを文字表のコードに
			b := []byte(s)
			for i, c := range b {
				b[i] = a.codes[strings.IndexByte(fmtDigits, c)]
			}
			s = string(b)
		}
		return s, true
	}
	return "", false
}

// fmtEmit は parts を書き先 dst (長さが u8 の slice の値) に書く呼び出しを出し、書いた長さ (一時変数) を返す。
func (h *Hlc) fmtEmit(a *fmtArgs, parts []ir.LogPart, dst *ir.Value, try bool) *ir.Value {
	fi := h.builtinModule("fmt")
	begin := "begin"
	if try {
		begin = "begin_try" // 足りなくても止まらない
	}
	h.lval(ccall(cv(h.moduleFunc(fi, begin)), cv(dst)))
	h.setCodes(a)
	h.fmtEmitParts(a, parts)
	n := h.newTmp(h.prog.Types.IntType(1, false)) // 書いた長さ (今の fmt.at。後の書き込みが変える前に写す)
	if try {
		h.emit(&ir.Op{Code: ir.OpLoad, Dst: n, Src: []ir.Operand{h.rval(ccall(cv(h.moduleFunc(fi, "end_try"))))}}) // 足りなければ 0
		return n
	}
	h.emit(&ir.Op{Code: ir.OpLoad, Dst: n, Src: []ir.Operand{h.moduleFunc(fi, "at")}})
	return n
}

// fmtEmitParts は parts を fmt の今の書き先に書く呼び出しを出す (begin の後)。
func (h *Hlc) fmtEmitParts(a *fmtArgs, parts []ir.LogPart) {
	fi := h.builtinModule("fmt")
	fn := func(name string) *cexpr { return cv(h.moduleFunc(fi, name)) }
	call := func(name string, args ...*cexpr) { h.lval(ccall(fn(name), args...)) }
	var text strings.Builder
	flush := func() {
		switch s := text.String(); len(s) {
		case 0:
		case 1:
			call("chr", cint(int(s[0])))
		default:
			call("str", cstr(s))
		}
		text.Reset()
	}
	u8 := h.prog.Types.IntType(1, false)
	for _, p := range parts {
		if s, ok := a.constText(p); ok {
			text.WriteString(s)
			continue
		}
		v, sp := a.vals[p.Arg], p.Spec
		t := ir.ValType(v)
		flush()
		switch {
		case t.IsSlice() && t.SliceOf == u8:
			a.noWidth(p, "a string")
			call("str", cv(h.operandValue(v)))
		case t.Kind == types.Array && t.Base == u8:
			a.noWidth(p, "a string")
			call("str_z_in", cv(h.operandValue(v))) // 配列は中の最初の 0 まで (文字列で初期化した配列は終端の 0 を含む)
		case t.Kind == types.Pointer && t.Base == u8:
			a.noWidth(p, "a string")
			call("str_z", cv(h.operandValue(v)))
		case t.Kind == types.Bool && sp.Verb == 0:
			call("boolean", cv(h.operandValue(v)))
		case sp.Verb == 'c':
			a.noWidth(p, "a character")
			call("chr", cv(h.operandValue(v)))
		case t.Kind == types.Int || t.Kind == types.Bool:
			n := cv(h.operandValue(v))
			size, signed := t.Size, t.Signed
			if t.Kind == types.Bool {
				size, signed = 1, false
			}
			if t.Enum != nil {
				n = &cexpr{kind: cCast, args: []*cexpr{n}, ty: h.prog.Types.IntType(size, signed), ck: syntax.CastAs}
			}
			if sp.Width > fmtMaxWidth {
				panic(&diag.Error{Msg: fmt.Sprintf("%s: the width is at most %d (argument %d)", a.what, fmtMaxWidth, p.Arg)})
			}
			name, spec := "dec_", sp.Width
			if sp.Zero {
				spec |= fmtZero
			}
			switch sp.Verb {
			case 'x':
				name, spec = "hex_", spec|fmtLower
			case 'X':
				name = "hex_"
			case 'b':
				name = "bin_"
			}
			if name == "dec_" && signed {
				name += "i"
			} else {
				name += "u"
			}
			name += strconv.Itoa(8 * size)
			call(name, n, cint(spec))
		default:
			panic(&diag.Error{Msg: fmt.Sprintf("%s: cannot format a value of type %s (integers, bool, enums, []u8 / *u8 strings)", a.what, t)})
		}
	}
	flush()
}

// format は @format の展開: 書いた部分の slice (dst と同じ先頭、長さは書き終えた位置) を返す。
func (h *Hlc) format(args []*cexpr, what string) *ir.Value {
	if len(args) < 2 {
		panic(&diag.Error{Msg: what + " takes a destination ([]u8), a format string and the arguments"})
	}
	u8 := h.prog.Types.IntType(1, false)
	d := h.sliceParts(args[0], what)
	switch {
	case d.elem != u8:
		panic(&diag.Error{Msg: fmt.Sprintf("%s: the destination must be a []u8 (got elements of %s)", what, d.elem)})
	case d.ro:
		panic(&diag.Error{Msg: what + ": the destination is read-only"})
	case d.wide:
		// [:u16]u8 の書き先は先頭の 255 バイトまで (buf.rest(&b) などをそのまま渡せるように)
		n := h.rval(&cexpr{kind: cOp, op: opMin, args: []*cexpr{cv(h.operandValue(d.len)), cint(255)}})
		d.len = h.rval(&cexpr{kind: cCast, args: []*cexpr{cv(h.operandValue(n))}, ty: u8, ck: syntax.CastAs})
	}
	a := h.fmtPrepare(what, args[1], args[2:])
	n := h.fmtEmit(a, a.parts, h.newSlice(u8, d.ptr, d.len, false, false), what == "@try_format")
	return h.newSlice(u8, d.ptr, n, false, false)
}

// printf4 は fc 4 の printf の展開 (console に出す)。
func (h *Hlc) printf4(args []*cexpr) {
	const what = "@printf"
	if len(args) < 1 {
		panic(&diag.Error{Msg: "@printf takes a format string and the arguments (@printf(\"HP {}\\n\", hp))"})
	}
	ci := h.builtinModule("console")
	a := h.fmtPrepare(what, args[0], args[1:])
	u8 := h.prog.Types.IntType(1, false)
	write := func(name string, v *cexpr) { h.lval(ccall(cv(h.moduleFunc(ci, name)), v)) }
	// 出力の順に部分ごとに console へ流す (printf ごとに一時バッファを取ると、printf の多い関数の静的フレームが 256 バイトを
	// 超えた)。文字の部分・定数はまとめて write_z (0 を含めば write)、文字列の引数は write / write_z / write_z_in、数などは
	// fmt の printf 用のバッファに書いて出す (fmt.begin_print(); fmt.dec_u8(x, spec); fmt.print()。呼び出し側で書き先の slice を
	// 作ると 1 か所 50 バイトほどになり、fuzz のプログラムが ROM からあふれた)
	var text strings.Builder
	flush := func() {
		if s := text.String(); s != "" {
			if strings.IndexByte(s, 0) < 0 {
				write("write_z", h.cstrZ(s)) // 終端 0 の文字列 (引数はポインタ 2 バイト)
			} else {
				write("write", cstr(s))
			}
			text.Reset()
		}
	}
	fi := h.builtinModule("fmt")
	fmtCall := func(name string) { h.lval(ccall(cv(h.moduleFunc(fi, name)))) }
	for _, p := range a.parts {
		if s, ok := a.constText(p); ok {
			text.WriteString(s)
			continue
		}
		v := a.vals[p.Arg]
		t := ir.ValType(v)
		flush()
		switch {
		case t.IsSlice() && t.SliceOf == u8:
			a.noWidth(p, "a string")
			write("write", cv(h.operandValue(v)))
		case t.Kind == types.Pointer && t.Base == u8:
			a.noWidth(p, "a string")
			write("write_z", cv(h.operandValue(v)))
		case t.Kind == types.Array && t.Base == u8:
			a.noWidth(p, "a string")
			write("write_z_in", cv(h.operandValue(v)))
		default:
			fmtCall("begin_print")
			h.setCodes(a)
			h.fmtEmitParts(a, []ir.LogPart{p})
			fmtCall("print")
		}
	}
	flush()
}

// formatConst は定数 k (型 t) を書式 sp の文字 (ASCII) にする (fmt の関数と同じ結果)。
func formatConst(k int, t *types.Type, sp ir.LogSpec) string {
	if t.Kind == types.Bool {
		if sp.Verb == 0 {
			return strconv.FormatBool(k != 0)
		}
		t = &types.Type{Kind: types.Int, Size: 1}
	}
	mask := 1<<(8*t.Size) - 1
	var digits string
	neg := false
	switch sp.Verb {
	case 'x':
		digits = strconv.FormatInt(int64(k&mask), 16)
	case 'X':
		digits = strings.ToUpper(strconv.FormatInt(int64(k&mask), 16))
	case 'b':
		digits = strconv.FormatInt(int64(k&mask), 2)
	default:
		if k < 0 {
			neg, k = true, -k
		}
		digits = strconv.Itoa(k)
	}
	body := len(digits)
	if neg {
		body++
	}
	pad := max(sp.Width-body, 0)
	switch {
	case sp.Zero && neg:
		return "-" + strings.Repeat("0", pad) + digits
	case sp.Zero:
		return strings.Repeat("0", pad) + digits
	case neg:
		return strings.Repeat(" ", pad) + "-" + digits
	}
	return strings.Repeat(" ", pad) + digits
}

// rewritePrintf は fc 3 のモジュールの printf (引数を並べる形) を、fc 4 の書式文字列の形 (`@printf("HP {}\n", hp)`) に書き換える
// (migrate の規則 printf-format)。文字列リテラルの引数は書式に取り込み (`{` `}` は二重に。綴りは値から fc 4 のエスケープで作り直す:
// fc 3 の "\t" は `\` と `t`)、ほかの引数は `{}` (bool は今の 0 / 1 のまま `{:d}`) にして後ろに並べる。typs は引数の型 (fc 3 の
// printf が評価したもの)。名前も @printf にする (h.macroCallee)。
func (h *Hlc) rewritePrintf(args []*cexpr, typs []*types.Type) {
	const rule = "printf-format"
	if len(args) == 0 {
		return
	}
	src := h.prog.Sources[h.module.Id]
	if src == nil {
		h.rewriteError(rule, "the source of the module is unknown")
		return
	}
	for _, a := range args {
		if !a.pos.IsValid() || !a.end.IsValid() || a.pos.Offset < 0 || a.end.Offset > len(src.Src) || a.pos.Offset >= a.end.Offset {
			h.rewriteError(rule, "an argument of printf has no position")
			return
		}
	}
	// 名前 (`printf` / `stdio.printf` と書いた所。評価済みなら printf のマクロの値)
	if c := h.macroCallee; c != nil && c.pos.IsValid() && c.end.IsValid() && c.pos.Offset < c.end.Offset && c.end.Offset <= len(src.Src) {
		if name := string(src.Src[c.pos.Offset:c.end.Offset]); name == "printf" || strings.HasSuffix(name, ".printf") {
			h.addRewrite(rule, c.pos.Offset, c.end.Offset, "@printf")
		}
	}
	var f strings.Builder          // 書式の値 (綴りは最後に syntax.QuoteString で作る)
	lit := make([]bool, len(args)) // 書式に取り込む文字列リテラルの引数
	braces := strings.NewReplacer("{", "{{", "}", "}}")
	for i, a := range args {
		text := string(src.Src[a.pos.Offset:a.end.Offset])
		if (a.kind == cStr || a.kind == cValue && a.val.IsString) && len(text) >= 2 && (text[0] == '"' || text[0] == '\'') && text[len(text)-1] == text[0] {
			v := a.s
			if a.kind == cValue {
				v = a.val.Str
			}
			f.WriteString(braces.Replace(v))
			lit[i] = true
			continue
		}
		if typs[i] != nil && typs[i].Kind == types.Bool {
			f.WriteString("{:d}")
		} else {
			f.WriteString("{}")
		}
	}
	// 置き換えるのは文字列リテラルの引数と区切りだけ (残す引数の中の書き換え (`x as i8 < vx` など) と重ならないように):
	// 先頭のリテラルの並びは書式に (先頭がリテラルでなければ書式を挿入)、途中の並びは区切り `, ` に、末尾の並びは消す
	format := syntax.QuoteString(f.String())
	first := -1 // 最初の残す引数
	for i := range args {
		if !lit[i] {
			first = i
			break
		}
	}
	if first < 0 {
		h.addRewrite(rule, args[0].pos.Offset, args[len(args)-1].end.Offset, format)
		return
	}
	h.addRewrite(rule, args[0].pos.Offset, args[first].pos.Offset, format+", ")
	prev := first
	for i := first + 1; i <= len(args); i++ {
		if i < len(args) && lit[i] {
			continue
		}
		if i < len(args) {
			if i > prev+1 {
				h.addRewrite(rule, args[prev].end.Offset, args[i].pos.Offset, ", ")
			}
		} else if prev < len(args)-1 {
			h.addRewrite(rule, args[prev].end.Offset, args[len(args)-1].end.Offset, "")
		}
		prev = i
	}
}
