package fc

// tools/dumper.rb の IR / alloc-IR ダンプの Go 側実装 (1:1 対応)。

import (
	"fmt"
	"strings"
)

func typeS(t *Type) string {
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

// dumpValue は dumper.rb の dump_value 相当。
func dumpValue(v any, ctx *irCtx) string {
	switch x := v.(type) {
	case *CastedValue:
		return fmt.Sprintf("{cast %s %d %s}", typeS(x.Type), x.Offset, dumpValue(x.From, ctx))
	case *PointeredArray:
		return fmt.Sprintf("{pa %s}", dumpValue(x.From, ctx))
	case *Value:
		if ctx != nil {
			if idx, ok := ctx.varIndex[x]; ok {
				return fmt.Sprintf("{l%d %s}", idx, ToS(x.Id))
			}
		}
		return dumpValueFull(x, ctx)
	case *Lambda:
		return fmt.Sprintf("{lambda %s}", ToS(x.Id))
	case *Type:
		return typeS(x)
	case Sym:
		return ":" + string(x)
	case string:
		return EscStr(x)
	case int:
		return fmt.Sprintf("%d", x)
	case nil:
		return "nil"
	default:
		panic(fmt.Sprintf("cannot dump value %T: %v", v, v))
	}
}

func dumpValueFull(v *Value, ctx *irCtx) string {
	bs := ""
	if v.BaseString != nil {
		bs = " " + EscStr(v.BaseString.(string))
	}
	ids := "nil"
	if v.Id != nil {
		ids = ToS(v.Id)
	}
	switch v.Kind {
	case "literal":
		return fmt.Sprintf("{lit %s %s %s%s}", ids, dumpGval(v.Val, ctx), typeS(v.Type), bs)
	case "array_literal":
		elems := make([]string, len(v.Val.([]any)))
		for i, e := range v.Val.([]any) {
			elems[i] = dumpValue(e, ctx)
		}
		return fmt.Sprintf("{arr %s %s (%s)%s}", ids, typeS(v.Type), strings.Join(elems, " "), bs)
	case "global":
		return fmt.Sprintf("{g %s %s %s%s}", ids, typeS(v.Type), dumpGval(v.Val, ctx), bs)
	case "module":
		return fmt.Sprintf("{mod %s}", ids)
	case "local":
		// 他のLambdaのローカルなど、表にない場合
		return fmt.Sprintf("{l? %s %s}", ids, typeS(v.Type))
	default:
		panic(fmt.Sprintf("cannot dump value kind %s", v.Kind))
	}
}

func dumpGval(val any, ctx *irCtx) string {
	switch x := val.(type) {
	case nil:
		return "nil"
	case int:
		return fmt.Sprintf("%d", x)
	case Sym:
		return ":" + string(x)
	case string:
		return EscStr(x)
	case *Module:
		return "mod:" + ToS(x.Id)
	case MacroFn:
		return "macro"
	case []any:
		elems := make([]string, len(x))
		for i, e := range x {
			elems[i] = dumpValue(e, ctx)
		}
		return "(" + strings.Join(elems, " ") + ")"
	default:
		panic(fmt.Sprintf("cannot dump gval %T", val))
	}
}

func dumpOptS(opt *OMap) string {
	if opt == nil {
		return "{}"
	}
	parts := make([]string, 0, opt.Len())
	for _, e := range opt.Entries() {
		var vs string
		switch x := e.Val.(type) {
		case nil:
			vs = "nil"
		case bool:
			if x {
				vs = "true"
			} else {
				vs = "false"
			}
		case int:
			vs = fmt.Sprintf("%d", x)
		case Sym:
			vs = ":" + string(x)
		case string:
			vs = EscStr(x)
		default:
			panic(fmt.Sprintf("cannot dump opt val %T", e.Val))
		}
		parts = append(parts, fmt.Sprintf("%s %s", ToS(e.Key), vs))
	}
	return "{" + strings.Join(parts, " ") + "}"
}

func dumpVar(v *Value, i int, ctx *irCtx, alloc bool) string {
	var b strings.Builder
	ids := "nil"
	if v.Id != nil {
		ids = ToS(v.Id)
	}
	fmt.Fprintf(&b, "(var %d %s %s %s", i, ids, v.Kind, typeS(v.Type))
	if v.Kind != "local" {
		fmt.Fprintf(&b, " val=%s", dumpGval(v.Val, ctx))
	}
	if lt := v.Opt.GetOr(Sym("local_type")); lt != nil {
		fmt.Fprintf(&b, " lt=%s", ToS(lt))
	}
	if v.Public {
		b.WriteString(" pub")
	}
	if alloc {
		if v.Location != "" {
			fmt.Fprintf(&b, " loc=%s", v.Location)
		} else {
			b.WriteString(" loc=nil")
		}
		if v.Address != nil {
			fmt.Fprintf(&b, " addr=%s", ToS(v.Address))
		}
		if v.CondReg != "" {
			fmt.Fprintf(&b, " cond=%s,%s", v.CondReg, ToS(v.CondPositive))
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
	switch x := d.Val.(type) {
	case nil:
		vs = "nil"
	case int:
		vs = fmt.Sprintf("%d", x)
	case Sym:
		vs = ":" + string(x)
	case string:
		vs = EscStr(x)
	case *Lambda:
		vs = fmt.Sprintf("{lambda %s}", ToS(x.Id))
	case *OMap:
		vs = dumpOptS(x)
	case []any:
		elems := make([]string, len(x))
		for i, e := range x {
			elems[i] = dumpValue(e, ctx)
		}
		vs = "(" + strings.Join(elems, " ") + ")"
	default:
		panic(fmt.Sprintf("cannot dump def val %T", d.Val))
	}
	return fmt.Sprintf("(def %s %s %s %s)", ToS(d.Sym), d.Kind, typeS(d.Type), vs)
}

func dumpOp(op []any, ctx *irCtx) string {
	if op == nil {
		return "nil"
	}
	parts := make([]string, len(op))
	for i, e := range op {
		parts[i] = dumpValue(e, ctx)
	}
	return "(" + strings.Join(parts, " ") + ")"
}

// DumpIR は dumper.rb の dump_ir 相当 (HLC完了直後)。
func DumpIR(h *Hlc) string {
	var r []string
	for _, e := range h.Options.Entries() {
		var vs string
		if s, ok := e.Val.(string); ok {
			vs = EscStr(s)
		} else {
			vs = ToS(e.Val)
		}
		r = append(r, fmt.Sprintf("(option %s %s)", ToS(e.Key), vs))
	}
	for _, me := range h.Modules.Entries() {
		mod := me.Val.(*Module)
		r = append(r, fmt.Sprintf("(module %s", ToS(mod.Id)))
		r = append(r, fmt.Sprintf(" (options %s)", dumpOptS(mod.Options)))
		r = append(r, fmt.Sprintf(" (include_asms (%s))", joinEsc(mod.IncludeAsms)))
		r = append(r, fmt.Sprintf(" (include_chrs (%s))", joinEsc(mod.IncludeChrs)))
		keys := make([]string, 0, mod.Modules.Len())
		for _, e := range mod.Modules.Entries() {
			keys = append(keys, ToS(e.Key))
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
	r = append(r, fmt.Sprintf(" (lambda %s %s opt=%s", ToS(lmd.Id), typeS(lmd.Type), dumpOptS(lmd.Opt)))
	args := make([]string, len(lmd.Args))
	for i, a := range lmd.Args {
		args[i] = dumpValue(a, ctx)
	}
	r = append(r, fmt.Sprintf("  (args (%s))", strings.Join(args, " ")))
	if lmd.Result != nil {
		r = append(r, fmt.Sprintf("  (result %s)", dumpValue(lmd.Result, ctx)))
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
		r = append(r, fmt.Sprintf("   %04d %s", i, dumpOp(op, ctx)))
	}
	r = append(r, "  )")
	r = append(r, " )")
	return r
}

// DumpAllocLambda は dumper.rb の dump_alloc_lambda 相当 (割付+delete_unuse直後)。
func DumpAllocLambda(modId Sym, sym any, lmd *Lambda) string {
	ctx := makeCtx(lmd)
	var r []string
	r = append(r, fmt.Sprintf("(alloc-lambda %s %s %s frame_size=%d", ToS(modId), ToS(sym), ToS(lmd.Id), lmd.FrameSize))
	r = append(r, " (vars")
	for i, v := range lmd.Vars {
		r = append(r, "  "+dumpVar(v, i, ctx, true))
	}
	r = append(r, " )")
	r = append(r, " (ops")
	for i, op := range lmd.Ops {
		r = append(r, fmt.Sprintf("  %04d %s", i, dumpOp(op, ctx)))
	}
	r = append(r, " )")
	r = append(r, ")")
	return strings.Join(r, "\n") + "\n"
}
