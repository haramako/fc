package ir

// IR / alloc-IR のダンプ (golden 比較用)。形式は移植期の tools/dumper.rb に由来する。
// R1-e で内部表現が型付きになった際、Ruby の Symbol/String の区別に由来していた
// `:name` / `"name"` の出し分けはシンボルをすべて `:name` に統一した (golden は再生成済み)。

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/haramako/fc/internal/types"
)

// EscStr は文字列をバイト単位でエスケープする。
func EscStr(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == 0x22:
			b.WriteString("\\\"")
		case c == 0x5c:
			b.WriteString("\\\\")
		case c == 0x0a:
			b.WriteString("\\n")
		case c >= 0x20 && c <= 0x7e:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "\\x%02X", c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func typeS(t *types.Type) string {
	return "#" + EscStr(t.String())
}

type irCtx struct {
	varIndex map[*Value]int
}

func makeCtx(lmd *Lambda) *irCtx {
	m := map[*Value]int{}
	for i, v := range lmd.Vars {
		m[v] = i
	}
	return &irCtx{varIndex: m}
}

func symS(s string) string { return ":" + s }

func nameS(name string) string {
	if name == "" {
		return "nil"
	}
	return name
}

// dumpOperand は命令のオペランドを出力する。
func dumpOperand(v Operand, ctx *irCtx) string {
	switch x := v.(type) {
	case nil:
		return "nil"
	case *CastedValue:
		return fmt.Sprintf("{cast %s %d %s}", typeS(x.Type), x.Offset, dumpOperand(x.From, ctx))
	case *PointeredArray:
		return fmt.Sprintf("{pa %s}", dumpOperand(x.From, ctx))
	case *Value:
		if ctx != nil {
			if idx, ok := ctx.varIndex[x]; ok {
				return fmt.Sprintf("{l%d %s}", idx, nameS(x.Name))
			}
		}
		return dumpValueFull(x, ctx)
	}
	panic(fmt.Sprintf("cannot dump operand %T: %v", v, v))
}

func dumpValueFull(v *Value, ctx *irCtx) string {
	bs := ""
	if v.IsString {
		bs = " " + EscStr(v.Str)
	}
	switch v.Kind {
	case KindLiteral:
		return fmt.Sprintf("{lit %s %s %s%s}", nameS(v.Name), dumpGval(v, ctx), typeS(v.Type), bs)
	case KindArrayLiteral:
		return fmt.Sprintf("{arr %s %s %s%s}", nameS(v.Name), typeS(v.Type), dumpElems(v.Elems, ctx), bs)
	case KindGlobal:
		return fmt.Sprintf("{g %s %s %s%s}", nameS(v.Name), typeS(v.Type), dumpGval(v, ctx), bs)
	case KindLocal:
		// 他のLambdaのローカルなど、表にない場合
		return fmt.Sprintf("{l? %s %s}", nameS(v.Name), typeS(v.Type))
	default:
		panic(fmt.Sprintf("cannot dump value kind %s", v.Kind))
	}
}

func dumpElems(elems []Operand, ctx *irCtx) string {
	parts := make([]string, len(elems))
	for i, e := range elems {
		parts[i] = dumpOperand(e, ctx)
	}
	return "(" + strings.Join(parts, " ") + ")"
}

// dumpGval は Value の「値」部分 (旧 Value#val)。
func dumpGval(v *Value, ctx *irCtx) string {
	switch {
	case v.Kind == KindLiteral && v.IsInt:
		return strconv.Itoa(v.Int)
	case v.Kind == KindArrayLiteral:
		return dumpElems(v.Elems, ctx)
	case v.Module != nil:
		return "mod:" + v.Module.Id
	case v.Type.Kind == types.Macro:
		return "macro"
	case v.Symbol != "":
		return symS(v.Symbol)
	}
	return "nil"
}

func dumpOptionValue(v OptionValue) string {
	switch v.Kind {
	case OptInt:
		return strconv.Itoa(v.Int)
	case OptStr:
		return EscStr(v.Str)
	case OptIdent:
		return symS(v.Str)
	}
	panic("invalid option value")
}

func dumpOptions(opts Options) string {
	parts := make([]string, 0, len(opts))
	for _, e := range opts {
		parts = append(parts, e.Key+" "+dumpOptionValue(e.Value))
	}
	return "{" + strings.Join(parts, " ") + "}"
}

// dumpLambdaOpt は旧 Lambda#opt (id → options の各エントリ → extern) と同じ並びで出力する。
func dumpLambdaOpt(lmd *Lambda) string {
	parts := []string{}
	if lmd.Name != "" {
		parts = append(parts, "id "+symS(lmd.Name))
	}
	for _, e := range lmd.Options {
		parts = append(parts, e.Key+" "+dumpOptionValue(e.Value))
	}
	if lmd.Extern {
		parts = append(parts, "extern true")
	}
	return "{" + strings.Join(parts, " ") + "}"
}

func dumpVar(v *Value, i int, ctx *irCtx, alloc bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "(var %d %s %s %s", i, nameS(v.Name), v.Kind, typeS(v.Type))
	if v.Kind != KindLocal {
		fmt.Fprintf(&b, " val=%s", dumpGval(v, ctx))
	}
	if v.LocalType != LTNone {
		fmt.Fprintf(&b, " lt=%s", v.LocalType)
	}
	if v.Public {
		b.WriteString(" pub")
	}
	if alloc {
		if v.Location != LocNone {
			fmt.Fprintf(&b, " loc=%s", v.Location)
		} else {
			b.WriteString(" loc=nil")
		}
		if v.HasAddress() {
			fmt.Fprintf(&b, " addr=%d", v.Address)
		}
		if v.CondReg != CondNone {
			fmt.Fprintf(&b, " cond=%s,%t", v.CondReg, v.CondPositive)
		}
		if v.Unuse {
			b.WriteString(" unuse")
		}
	}
	b.WriteString(")")
	return b.String()
}

func dumpDef(d *Def, ctx *irCtx) string {
	var vs string
	switch d.Kind {
	case DefEqu:
		vs = dumpGval(d.Equ, ctx)
	case DefBss:
		if d.Segment != "" {
			vs = "{segment " + EscStr(d.Segment) + "}"
		} else {
			vs = "{segment nil}"
		}
	case DefBlock:
		vs = dumpElems(d.Elems, ctx)
	case DefCode:
		vs = fmt.Sprintf("{lambda %s}", d.Lambda.Id)
	default:
		panic(fmt.Sprintf("cannot dump def kind %s", d.Kind))
	}
	return fmt.Sprintf("(def %s %s %s %s)", d.Sym, d.Kind, typeS(d.Type), vs)
}

// DumpOp は命令を旧 IR と同じ位置引数の並び (Op.positional) で出力する (ctx は nil 可)。
func DumpOp(op *Op, ctx *irCtx) string {
	if op == nil {
		return "nil"
	}
	parts := []string{symS(op.Code.String())}
	for _, e := range op.positional() {
		switch x := e.(type) {
		case string:
			parts = append(parts, EscStr(x))
		case *types.Type:
			parts = append(parts, typeS(x))
		case Operand:
			parts = append(parts, dumpOperand(x, ctx))
		case nil:
			parts = append(parts, "nil")
		default:
			panic(fmt.Sprintf("cannot dump op element %T", e))
		}
	}
	return "(" + strings.Join(parts, " ") + ")"
}

// DumpProgram は HLC 完了直後の IR (グローバル options と全モジュール) を出力する。
func DumpProgram(opts Options, mods []*Module) string {
	var r []string
	for _, e := range opts {
		r = append(r, fmt.Sprintf("(option %s %s)", e.Key, dumpOptionValue(e.Value)))
	}
	for _, mod := range mods {
		r = append(r, fmt.Sprintf("(module %s", mod.Id))
		r = append(r, fmt.Sprintf(" (options %s)", dumpOptions(mod.Options)))
		r = append(r, fmt.Sprintf(" (include_asms (%s))", joinEsc(mod.IncludeAsms)))
		r = append(r, fmt.Sprintf(" (include_chrs (%s))", joinEsc(mod.IncludeChrs)))
		keys := make([]string, 0, len(mod.Modules.List()))
		for _, m := range mod.Modules.List() {
			keys = append(keys, m.Id)
		}
		r = append(r, fmt.Sprintf(" (modules (%s))", strings.Join(keys, " ")))
		r = append(r, " (defs")
		for _, d := range mod.Defs {
			r = append(r, "  "+dumpDef(d, nil))
		}
		r = append(r, " )")
		r = append(r, " (vars")
		for i, v := range mod.Vars {
			r = append(r, "  "+dumpVar(v, i, nil, false))
		}
		r = append(r, " )")
		for _, lmd := range mod.Lambdas {
			r = append(r, dumpLambda(lmd)...)
		}
		r = append(r, ")")
	}
	return strings.Join(r, "\n") + "\n"
}

func joinEsc(ss []string) string {
	parts := make([]string, len(ss))
	for i, s := range ss {
		parts[i] = EscStr(s)
	}
	return strings.Join(parts, " ")
}

func dumpLambda(lmd *Lambda) []string {
	ctx := makeCtx(lmd)
	var r []string
	r = append(r, fmt.Sprintf(" (lambda %s %s opt=%s", lmd.Id, typeS(lmd.Type), dumpLambdaOpt(lmd)))
	args := make([]string, len(lmd.Args))
	for i, a := range lmd.Args {
		args[i] = dumpOperand(a, ctx)
	}
	r = append(r, fmt.Sprintf("  (args (%s))", strings.Join(args, " ")))
	if lmd.Result != nil {
		r = append(r, fmt.Sprintf("  (result %s)", dumpOperand(lmd.Result, ctx)))
	} else {
		r = append(r, "  (result nil)")
	}
	r = append(r, "  (vars")
	for i, v := range lmd.Vars {
		r = append(r, "   "+dumpVar(v, i, ctx, false))
	}
	r = append(r, "  )")
	r = append(r, "  (defs")
	for _, d := range lmd.Defs {
		r = append(r, "   "+dumpDef(d, ctx))
	}
	r = append(r, "  )")
	r = append(r, "  (ops")
	for i, op := range lmd.Ops {
		r = append(r, fmt.Sprintf("   %04d %s", i, DumpOp(op, ctx)))
	}
	r = append(r, "  )")
	r = append(r, " )")
	return r
}

// DumpAllocLambda は割付 + delete_unuse 直後の関数をダンプする。
func DumpAllocLambda(modId string, sym string, lmd *Lambda) string {
	ctx := makeCtx(lmd)
	var r []string
	r = append(r, fmt.Sprintf("(alloc-lambda %s %s %s frame_size=%d", modId, sym, lmd.Id, lmd.FrameSize))
	r = append(r, " (vars")
	for i, v := range lmd.Vars {
		r = append(r, "  "+dumpVar(v, i, ctx, true))
	}
	r = append(r, " )")
	r = append(r, " (ops")
	for i, op := range lmd.Ops {
		r = append(r, fmt.Sprintf("  %04d %s", i, DumpOp(op, ctx)))
	}
	r = append(r, " )")
	r = append(r, ")")
	return strings.Join(r, "\n") + "\n"
}
