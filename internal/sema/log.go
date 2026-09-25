package sema

// fc 3 の @log (doc/v3_plan.md §9)。`@log("HP: {} / {}", hp, max_hp);` は命令を出さず、次に出す命令への注釈
// (ir.Op.Logs) になる。表示はエミュレータ側 (Mesen の Lua、fcc run) がする。書式は `{}` (順番) / `{0}` (位置) と
// `{:x}` / `{:04X}` / `{:b}` / `{:c}` / `{:d}`、`{{` / `}}`。引数は変数 (グローバル・ローカル・引数)・定数・struct の
// フィールド・定数の添字の要素だけ (副作用が無く、ホストがメモリとレジスタから読める値)。
//
// 注釈を付けるのは fcc build -g のときだけ (Program.LogEnabled)。書式と引数の検査は常にする。

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// registerLogBuiltin は @log を登録する。
func registerLogBuiltin(h *Hlc) {
	h.defmacro("@log", func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		if h.lmd == nil {
			panic(&diag.Error{Msg: "@log can only be used inside a function"})
		}
		if len(args) == 0 {
			panic(&diag.Error{Msg: `@log needs a format string (@log("x = {}", x))`})
		}
		format := mustString(args[0])
		p := &ir.LogPoint{Pos: h.curPos, Format: format}
		for i, a := range args[1:] {
			v, text := h.logOperand(a)
			t := ir.ValType(v)
			if err := checkLogType(t); err != "" {
				panic(&diag.Error{Msg: fmt.Sprintf("@log: argument %d (%s): %s", i+1, text, err)})
			}
			p.Args = append(p.Args, &ir.LogArg{Expr: text, Type: t, Val: v})
		}
		parts, err := parseLogFormat(format, p.Args)
		if err != nil {
			panic(&diag.Error{Msg: "@log: " + err.Error()})
		}
		p.Parts = parts
		if h.prog.LogEnabled {
			h.prog.nextLogID++
			p.ID = h.prog.nextLogID
			h.pendingLogs = append(h.pendingLogs, p)
		}
		return macroResult{}
	})
}

// logEveryStatement は LogEveryStatement のときの、文の前の @log (その関数の引数と変数のうち表示できるもの全部)。
func (h *Hlc) logEveryStatement() {
	p := &ir.LogPoint{Pos: h.curPos, Format: "stmt"}
	var parts []ir.LogPart
	for _, v := range h.lmd.Vars {
		if v.Kind != ir.KindLocal || (v.LocalType != ir.LTNone && v.LocalType != ir.LTArg) || checkLogType(v.Type) != "" {
			continue
		}
		if h.scope.Find(v.Name, true) != v {
			continue // スコープの外 (終わったループの変数など)
		}
		parts = append(parts, ir.LogPart{Text: " " + v.Name + "=", Arg: -1}, ir.LogPart{Arg: len(p.Args)})
		p.Args = append(p.Args, &ir.LogArg{Expr: v.Name, Type: v.Type, Val: v})
	}
	h.prog.nextLogID++
	p.ID = h.prog.nextLogID
	p.Parts = append([]ir.LogPart{{Text: fmt.Sprintf("%s:%d#%d", h.curPos.Filename, h.curPos.Line, p.ID), Arg: -1}}, parts...)
	h.pendingLogs = append(h.pendingLogs, p)
}

// warnBranchEndLog は分岐の最後の @log を警告する: 命令を出さないので、地点は分岐の後ろ (合流点) になり、分岐しなかった
// ときにも表示される。
func (h *Hlc) warnBranchEndLog() {
	for _, p := range h.pendingLogs {
		h.prog.Warnings = append(h.prog.Warnings, diag.Warning{Msg: "@log at the end of an if / else branch marks the code after the if (it is also shown when the branch is not taken); move it before the last statement", Pos: p.Pos})
	}
}

// checkLogType は @log で表示できる型か ("" なら表示できる)。
func checkLogType(t *types.Type) string {
	switch {
	case t.Kind == types.Int && t.Size <= 2, t.Kind == types.Bool:
		return ""
	case t.Kind == types.Pointer, t.Kind == types.Func && !t.IsFarFunc():
		return ""
	}
	return fmt.Sprintf("cannot show a value of type %s (integers, bool, enum and pointers only)", t)
}

// logOperand は @log の引数を、命令を出さずに値 (変数・定数・その一部) にする。text はソースの綴り (警告用)。
func (h *Hlc) logOperand(c *cexpr) (ir.Operand, string) {
	e := h.constEval(c)
	switch {
	case e.kind == cValue:
		v := e.val
		switch {
		case v.Kind == ir.KindLiteral && v.IsInt:
			return v, strconv.Itoa(v.Int)
		case v.Kind == ir.KindLiteral && v.Symbol != "" && !v.IsString:
			return v, v.Symbol // 関数のアドレスなど
		case v.Kind == ir.KindLocal || v.Kind == ir.KindGlobal:
			if v.Type.Kind == types.TypeName || v.Type.Kind == types.Macro || v.Type.Kind == types.Module || v.Type.IsSoa {
				break
			}
			if root := h.prog.storageAliases[v]; root != nil {
				return ir.NewCastedValue(root, v.Type, 0), v.Name
			}
			return v, v.Name
		}
	case e.kind == cOp && e.op == opField:
		base, text := h.logOperand(e.args[0])
		bt := ir.ValType(base)
		if bt.Kind == types.Struct && !isLogLiteral(base) {
			h.completeType(bt)
			f := h.fieldOf(bt, e.name)
			return ir.NewCastedValue(base, f.Type, f.Offset), text + "." + e.name
		}
		panic(&diag.Error{Msg: fmt.Sprintf("@log: %s.%s: only fields of struct variables can be shown (not through a pointer yet)", text, e.name)})
	case e.kind == cOp && e.op == opIndex:
		base, text := h.logOperand(e.args[0])
		bt := ir.ValType(base)
		idx := h.constEval(e.args[1])
		if bt.Kind == types.Array && !bt.IsSoa && !isLogLiteral(base) && idx.isLiteralInt() {
			n := idx.val.Int
			if n < 0 || (bt.Length >= 0 && n >= bt.Length) {
				panic(&diag.Error{Msg: fmt.Sprintf("@log: %s[%d] is out of range (length %d)", text, n, bt.Length)})
			}
			h.completeType(bt.Base)
			return ir.NewCastedValue(base, bt.Base, n*bt.Base.Size), fmt.Sprintf("%s[%d]", text, n)
		}
		panic(&diag.Error{Msg: fmt.Sprintf("@log: %s[...]: only array variables with a constant index can be shown", text)})
	}
	panic(&diag.Error{Msg: "@log can show variables, constants, struct fields and constant-index elements (not calls or other expressions)"})
}

// isLogLiteral は定数 (配列・struct のリテラル) か。
func isLogLiteral(v ir.Operand) bool {
	return ir.ValKind(v) == ir.KindLiteral || ir.ValKind(v) == ir.KindArrayLiteral
}

// parseLogFormat は @log の書式を解析して、引数の数と型を検査する。
func parseLogFormat(format string, args []*ir.LogArg) ([]ir.LogPart, error) {
	var parts []ir.LogPart
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			parts = append(parts, ir.LogPart{Text: text.String(), Arg: -1})
			text.Reset()
		}
	}
	used := make([]bool, len(args))
	next := 0
	for i := 0; i < len(format); i++ {
		ch := format[i]
		switch {
		case ch == '{' && i+1 < len(format) && format[i+1] == '{':
			text.WriteByte('{')
			i++
		case ch == '}' && i+1 < len(format) && format[i+1] == '}':
			text.WriteByte('}')
			i++
		case ch == '}':
			return nil, fmt.Errorf("unmatched `}` in the format (write `}}` for a brace)")
		case ch == '{':
			end := strings.IndexByte(format[i:], '}')
			if end < 0 {
				return nil, fmt.Errorf("unclosed `{` in the format (write `{{` for a brace)")
			}
			body := format[i+1 : i+end]
			i += end
			idxText, specText, hasSpec := strings.Cut(body, ":")
			arg := next
			if idxText == "" {
				next++
			} else {
				n, err := strconv.Atoi(idxText)
				if err != nil || n < 0 {
					return nil, fmt.Errorf("{%s}: the argument must be a number (`{0}`) or empty (`{}`)", body)
				}
				arg = n
			}
			if arg >= len(args) {
				return nil, fmt.Errorf("{%s}: there is no argument %d (%d given)", body, arg, len(args))
			}
			var spec ir.LogSpec
			if hasSpec {
				var err error
				if spec, err = parseLogSpec(specText); err != nil {
					return nil, fmt.Errorf("{%s}: %v", body, err)
				}
				if err := checkLogSpec(spec, args[arg].Type); err != "" {
					return nil, fmt.Errorf("{%s}: %s (argument %d is %s)", body, err, arg, args[arg].Type)
				}
			}
			used[arg] = true
			flush()
			parts = append(parts, ir.LogPart{Arg: arg, Spec: spec})
		default:
			text.WriteByte(ch)
		}
	}
	flush()
	for i, u := range used {
		if !u {
			return nil, fmt.Errorf("argument %d (%s) is not used in the format", i, args[i].Expr)
		}
	}
	return parts, nil
}

// parseLogSpec は `{:04x}` の `04x` (0 埋め・幅・種類)。
func parseLogSpec(s string) (ir.LogSpec, error) {
	var sp ir.LogSpec
	if strings.HasPrefix(s, "0") && len(s) > 1 {
		sp.Zero = true
		s = s[1:]
	}
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i > 0 {
		sp.Width, _ = strconv.Atoi(s[:i])
	}
	switch rest := s[i:]; rest {
	case "":
	case "d", "x", "X", "b", "c":
		sp.Verb = rest[0]
	default:
		return sp, fmt.Errorf("unknown format `%s` (use d, x, X, b or c, with an optional width like 04x)", rest)
	}
	return sp, nil
}

// checkLogSpec は見せ方の指定が型に合うか ("" なら合う)。
func checkLogSpec(sp ir.LogSpec, t *types.Type) string {
	if sp.Verb == 'c' && !(t.Kind == types.Int && t.Size == 1 && t.Enum == nil) {
		return "`c` shows a 1-byte integer as a character"
	}
	if sp.Verb != 0 && t.Kind == types.Bool && sp.Verb != 'd' {
		return fmt.Sprintf("`%c` cannot be used for bool (use d for 0 / 1)", sp.Verb)
	}
	return ""
}
