package fc

// tools/dumper.rb と同一の正規形S式シリアライザ。
// 仕様は doc/go_port_dump_format.md (正典は dumper.rb 実装)。

import (
	"fmt"
	"strconv"
	"strings"
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

// SexpStr は AST用の汎用S式(compact形式)。
func SexpStr(x any) string {
	switch v := x.(type) {
	case nil:
		return "nil"
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(v)
	case Sym:
		return ":" + string(v)
	case string:
		return EscStr(v)
	case []any:
		parts := make([]string, len(v))
		for i, e := range v {
			parts[i] = SexpStr(e)
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *OMap:
		parts := make([]string, 0, v.Len()*2)
		for _, e := range v.Entries() {
			parts = append(parts, SexpStr(e.Key), SexpStr(e.Val))
		}
		return "{" + strings.Join(parts, " ") + "}"
	default:
		panic(fmt.Sprintf("cannot dump %T: %v", x, x))
	}
}

// Canon は OMap のキー正規形 (構造的等値の判定に使う)。
// 純粋なASTは構造で、Value等のオブジェクトは同一性(ポインタ)でキー化する
// (Ruby の Hash キー等値と同じ挙動: Array は構造、その他オブジェクトは identity)。
func Canon(x any) string {
	switch v := x.(type) {
	case nil, bool, int, Sym, string:
		return SexpStr(x)
	case []any:
		var b strings.Builder
		b.WriteByte('(')
		for i, e := range v {
			if i > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(Canon(e))
		}
		b.WriteByte(')')
		return b.String()
	case *OMap:
		var b strings.Builder
		b.WriteByte('{')
		for i, e := range v.Entries() {
			if i > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(Canon(e.Key))
			b.WriteByte(' ')
			b.WriteString(Canon(e.Val))
		}
		b.WriteByte('}')
		return b.String()
	default:
		// Value / Type / Lambda など: 同一性でキー化
		return fmt.Sprintf("#<%T:%p>", x, x)
	}
}

