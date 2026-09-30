package syntax

import (
	"fmt"
	"strings"
)

// QuoteString は値 v の fc 4 の文字列リテラル (`"..."`。エスケープは unescape の fc 4 の規則)。fc 3 → 4 の書き換えが使う。
func QuoteString(v string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(v); i++ {
		switch ch := v[i]; {
		case ch == '\\' || ch == '"':
			b.WriteByte('\\')
			b.WriteByte(ch)
		case ch == '\n':
			b.WriteString(`\n`)
		case ch == '\t':
			b.WriteString(`\t`)
		case ch == 0:
			b.WriteString(`\0`)
		case ch < 0x20 || ch == 0x7f:
			fmt.Fprintf(&b, `\x%02X`, ch)
		default:
			b.WriteByte(ch)
		}
	}
	b.WriteByte('"')
	return b.String()
}
