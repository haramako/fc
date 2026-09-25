package migrate

import (
	"strings"

	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/sema"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

func init() {
	Rules = append(Rules,
		Rule{Name: "int-types", Doc: "整数型名を fc 3 の名前にする (int / uint8 → u8、sint → i8、int16 → u16、sint16 → i16)", Apply: renameIntTypes},
		Rule{Name: "attributes", Doc: "options(...) を @(...) にする (真偽値の属性の `: true` は省く。block { ... } options(k: v); は @(k: v) { ... })", Apply: attributes},
		Rule{Name: "infer-arrays", Doc: "長さを省いた配列型 []T を [?]T にする (fc 3 の []T は slice)", Apply: inferArrays},
		Rule{Name: "at-builtins", Doc: "組み込みを @ の形にする (sizeof / incbin / bitcast<T>(x) → @bitcast(T, x) / include(...) options(...) → @include(..., k: v) / asm / textmap / min / max / clamp / unittest_run_tests → @run_tests)", Apply: atBuiltins},
	)
}

// renameIntTypes は型の位置の fc 2 だけの整数型名 (モジュール名の付かない NamedType) を fc 3 の名前にする。
// 変数名・コメント・文字列の中の同じ綴りは書き換えない (構文木の型名だけを見る)。doc/v3_plan.md §7。
func renameIntTypes(c *Ctx) {
	syntax.Inspect(c.File, func(n syntax.Node) bool {
		nt, ok := n.(*syntax.NamedType)
		if !ok || nt.Module != nil || nt.Name == nil {
			return true
		}
		if v3, old := types.V2IntTypeNames[nt.Name.Name]; old {
			off := nt.Name.NamePos.Offset
			c.Replace(off, off+len(nt.Name.Name), v3)
		}
		return true
	})
}

// inferArrays は長さを省いた配列型 `[]T` (長さは初期値などから決まる) を fc 3 の `[?]T` にする。fc 3 の `[]T` は slice。
func inferArrays(c *Ctx) {
	syntax.Inspect(c.File, func(n syntax.Node) bool {
		if at, ok := n.(*syntax.ArrayType); ok && at.Len == nil && !at.Infer.IsValid() {
			c.Replace(at.Rbrack.Offset, at.Rbrack.Offset, "?")
		}
		return true
	})
}

// atBuiltins は fc 2 の組み込みの書き方を fc 3 の `@` の形にする (doc/v3_plan.md §5 A)。名前で引く組み込み (asm / min など) の
// 呼び出しは、同じファイルのトップレベルで同じ名前を宣言していなければ書き換える (use で取り込んだ同名の関数は見分けられない)。
func atBuiltins(c *Ctx) {
	declared := map[string]bool{}
	for _, st := range c.File.Stmts {
		switch d := st.(type) {
		case *syntax.FuncDecl:
			declared[d.Name.Name] = true
		case *syntax.VarDecl:
			for _, sp := range d.Specs {
				declared[sp.Name.Name] = true
			}
		}
	}
	syntax.Inspect(c.File, func(n syntax.Node) bool {
		switch e := n.(type) {
		case *syntax.SizeofExpr:
			c.Replace(e.Sizeof.Offset, e.Sizeof.Offset+len("sizeof"), "@sizeof")
		case *syntax.IncbinExpr:
			c.Replace(e.Incbin.Offset, e.Incbin.Offset+len("incbin"), "@incbin")
		case *syntax.CastExpr:
			if e.Kind == syntax.CastBit && e.Lt.IsValid() {
				// bitcast<T>(x) → @bitcast(T, x)。T の中は触らない (型名の書き換え int-types が当たる)
				c.Replace(e.Bitcast.Offset, e.Lt.Offset+1, "@bitcast(")
				c.Replace(e.Gt.Offset, e.Lparen.Offset+1, ", ")
			}
		case *syntax.IncludeDecl:
			if e.At {
				return true
			}
			c.Replace(e.Include.Offset, e.Include.Offset+len("include"), "@include")
			if o := e.Options; o != nil {
				// include("p") options(k: v) → @include("p", k: v)
				lp := strings.IndexByte(string(c.Src[o.Keyword.Offset:o.Rparen.Offset]), '(') + o.Keyword.Offset
				inner := strings.TrimSpace(string(c.Src[lp+1 : o.Rparen.Offset]))
				c.Replace(e.Rparen.Offset, o.Rparen.Offset+1, ", "+inner+")")
			}
		case *syntax.CallExpr:
			if id, ok := e.Fun.(*syntax.Ident); ok && !declared[id.Name] {
				if at, ok := sema.V3Builtins[id.Name]; ok {
					c.Replace(id.NamePos.Offset, id.NamePos.Offset+len(id.Name), at)
				}
			}
		}
		return true
	})
}

// attributes は fc 2 の `options(...)` を fc 3 の `@(...)` にする (doc/v3_plan.md §5 C)。真偽値の属性 (ir.FlagOptions) の
// `: true` は省いて `@(inline)` にする。`block { ... } options(k: v);` は `@(k: v) { ... }`。include の options は at-builtins が
// 名前つきの引数にまとめるので触らない。
func attributes(c *Ctx) {
	skip := map[*syntax.Options]bool{}
	syntax.Inspect(c.File, func(n syntax.Node) bool {
		if d, ok := n.(*syntax.IncludeDecl); ok && d.Options != nil {
			skip[d.Options] = true
		}
		return true
	})
	syntax.Inspect(c.File, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.PlacementBlock:
			if x.Keyword == nil || x.Options == nil {
				return true
			}
			o := x.Options
			skip[o] = true
			kw := x.Keyword.NamePos.Offset
			c.Replace(kw, kw+len(x.Keyword.Name), "@("+c.optionEntriesText(o)+")")
			c.Replace(x.Body.End().Offset, x.Semi.Offset+1, "")
		case *syntax.Options:
			if x.At || skip[x] {
				return true
			}
			c.Replace(x.Keyword.Offset, x.Keyword.Offset+len("options"), "@")
			for _, e := range x.Entries {
				if b, ok := e.Value.(*syntax.BoolLit); ok && b.Value && ir.FlagOptions[e.Key.Name] {
					end := e.Key.NamePos.Offset + len(e.Key.Name)
					c.Replace(end, b.ValuePos.Offset+len("true"), "")
				}
			}
		}
		return true
	})
}

// optionEntriesText は options の `key: value, ...` の部分のソースの綴り (真偽値の属性の `: true` は省く)。
func (c *Ctx) optionEntriesText(o *syntax.Options) string {
	var parts []string
	for _, e := range o.Entries {
		if b, ok := e.Value.(*syntax.BoolLit); ok && b.Value && ir.FlagOptions[e.Key.Name] {
			parts = append(parts, e.Key.Name)
			continue
		}
		parts = append(parts, e.Key.Name+": "+strings.TrimSpace(string(c.Src[e.Value.Pos().Offset:e.Value.End().Offset])))
	}
	return strings.Join(parts, ", ")
}
