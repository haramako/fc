// Package fc は FCコンパイラの Go 実装。
// Ruby版 (lib/fc/, タグ ruby-frozen) の厳密クローンであり、
// AST は Ruby と同様の動的構造 ([]any / Sym / int / string / *OMap / nil) で表現する。
package fc

import (
	"fmt"
	"strconv"
)

// Sym は Ruby の Symbol に相当する。
type Sym string

// OMap は挿入順を保持するマップ (Ruby の Hash 相当)。
// キーは構造的等値 (正規形S式が同じなら同じキー) で比較する。
type OMap struct {
	entries []OMapEntry
	index   map[string]int
}

type OMapEntry struct {
	Key any
	Val any
}

func NewOMap() *OMap {
	return &OMap{index: map[string]int{}}
}

// Set はキーに値を設定する。既存キーは挿入位置を保ったまま値を上書きする(RubyのHashと同じ)。
func (m *OMap) Set(k, v any) {
	ck := Canon(k)
	if i, ok := m.index[ck]; ok {
		m.entries[i].Val = v
		return
	}
	m.index[ck] = len(m.entries)
	m.entries = append(m.entries, OMapEntry{k, v})
}

func (m *OMap) Get(k any) (any, bool) {
	if i, ok := m.index[Canon(k)]; ok {
		return m.entries[i].Val, true
	}
	return nil, false
}

// GetOr は存在しない場合 nil を返す (Ruby の hash[k] 相当)。
func (m *OMap) GetOr(k any) any {
	v, _ := m.Get(k)
	return v
}

func (m *OMap) Len() int {
	if m == nil {
		return 0
	}
	return len(m.entries)
}

func (m *OMap) Entries() []OMapEntry {
	if m == nil {
		return nil
	}
	return m.entries
}

// OMapFromPairs は [k1, v1, k2, v2, ...] の平坦リストから OMap を作る (Ruby の Hash[*list] 相当)。
func OMapFromPairs(pairs []any) *OMap {
	m := NewOMap()
	for i := 0; i+1 < len(pairs); i += 2 {
		m.Set(pairs[i], pairs[i+1])
	}
	return m
}

// CompileError は Fc::CompileError 相当。
type CompileError struct {
	Msg      string
	Filename string
	LineNo   int
}

func (e *CompileError) Error() string { return e.Msg }

// ToS は Ruby の to_s 相当 (エラーメッセージや文字列化で使用)。
func ToS(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case Sym:
		return string(x)
	case string:
		return x
	case int:
		return strconv.Itoa(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case interface{ String() string }:
		return x.String()
	default:
		return fmt.Sprintf("%v", x)
	}
}

// cons は Ruby の `ary + [x...]` 相当 (新しいスライスを作る)。
func cons(a []any, xs ...any) []any {
	r := make([]any, 0, len(a)+len(xs))
	r = append(r, a...)
	r = append(r, xs...)
	return r
}
