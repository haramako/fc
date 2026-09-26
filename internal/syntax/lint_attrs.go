package syntax

// 属性 (`@(...)` / `options(...)`) の検査: 知らないキー (綴りの誤り) と、その宣言では何もしないキーを警告する
// (`@(adress: 0x6000)` が黙って普通の RAM 変数になっていた。エラーにするかは後で決める。doc/roadmap.md の v3)。

import "sort"

// knownAttrs はコンパイラが読む属性のキー (どこかの宣言で意味を持つもの)。
var knownAttrs = map[string]bool{
	"abi": true, "address": true, "bank": true, "bank_count": true, "base": true, "bss": true, "build": true,
	"char_banks": true, "farcall": true, "fastcall": true, "fastcall_reg": true, "inline": true, "interrupt": true,
	"link": true, "linker_config": true, "mapper": true, "near": true, "noinline": true, "org": true, "segment": true,
	"static_ram": true, "static_zp": true, "symbol": true, "volatile": true, "zeropage": true,
}

// attrsFor は宣言の種類ごとに意味を持つキー (nil なら種類では絞らない: モジュールの属性)。
var attrsFor = map[string]map[string]bool{
	"a function":        {"inline": true, "noinline": true, "fastcall": true, "interrupt": true, "symbol": true, "segment": true, "abi": true, "zeropage": true, "near": true},
	"a global variable": {"address": true, "symbol": true, "segment": true, "volatile": true},
	"a const":           {"symbol": true, "build": true, "address": true},
	"a soa":             {"segment": true},
	"a local variable":  {},
	"a parameter":       {},
	"an include":        {},
}

func (l *linter) attributes(f *File) {
	inFunc := map[*VarDecl]bool{}
	params := map[*VarSpec]bool{}
	markBody := func(b *Block) {
		if b == nil {
			return
		}
		Inspect(b, func(n Node) bool {
			if d, ok := n.(*VarDecl); ok {
				inFunc[d] = true
			}
			return true
		})
	}
	Inspect(f, func(n Node) bool {
		switch x := n.(type) {
		case *FuncDecl:
			for _, p := range x.Params {
				params[p] = true
			}
			markBody(x.Body)
		case *LambdaExpr:
			markBody(x.Body)
		}
		return true
	})
	Inspect(f, func(n Node) bool {
		switch x := n.(type) {
		case *OptionsStmt:
			l.checkAttrs(x.Options, "")
		case *FuncDecl:
			l.checkAttrs(x.Options, "a function")
			for _, p := range x.Params {
				l.checkAttrs(p.Options, "a parameter")
			}
		case *VarDecl:
			kind := "a global variable"
			switch {
			case x.Const:
				kind = "a const"
			case inFunc[x]:
				kind = "a local variable"
			case x.Alias:
				kind = ""
			}
			for _, sp := range x.Specs {
				if !params[sp] {
					l.checkAttrs(sp.Options, kind)
				}
			}
		case *SoaDecl:
			l.checkAttrs(x.Options, "a soa")
		case *IncludeDecl:
			l.checkAttrs(x.Options, "an include")
		}
		return true
	})
}

func (l *linter) checkAttrs(o *Options, kind string) {
	if o == nil {
		return
	}
	for _, e := range o.Entries {
		key := e.Key.Name
		if !knownAttrs[key] {
			if s := similarAttr(key); s != "" {
				l.warn(e.Key.NamePos, "unknown attribute `%s` (did you mean `%s`?); it is ignored", key, s)
			} else {
				l.warn(e.Key.NamePos, "unknown attribute `%s`; it is ignored", key)
			}
			continue
		}
		if allowed, ok := attrsFor[kind]; ok && kind != "" && !allowed[key] {
			hint := ""
			if key == "zeropage" && kind == "a global variable" {
				hint = ` (use @(segment: "ZEROPAGE"))`
			}
			l.warn(e.Key.NamePos, "attribute `%s` has no effect on %s; it is ignored%s", key, kind, hint)
		}
	}
}

// similarAttr は key に近い (編集距離 2 以下の) 知っている属性の名前 (無ければ "")。
func similarAttr(key string) string {
	var names []string
	for k := range knownAttrs {
		names = append(names, k)
	}
	sort.Strings(names)
	best, bestD := "", 3
	for _, k := range names {
		if d := editDistance(key, k); d < bestD {
			best, bestD = k, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}
