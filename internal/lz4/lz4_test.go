package lz4

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

// samples は圧縮の効き方の違うデータ。
func samples(r *rand.Rand) map[string][]byte {
	m := map[string][]byte{
		"empty":  nil,
		"one":    {7},
		"short":  []byte("hello"),
		"twelve": []byte("abcabcabcabc"),
		"text":   []byte(strings.Repeat("The quick brown fox jumps over the lazy dog. ", 40)),
		"zeros":  make([]byte, 5000),
		"long":   bytes.Repeat([]byte{1, 2, 3}, 30000),
	}
	for _, n := range []int{13, 100, 1000, 70000} {
		b := make([]byte, n)
		r.Read(b)
		m["random"+string(rune('0'+len(m)%10))] = b
		// 少ない種類の文字 (一致が多い)
		c := make([]byte, n)
		for i := range c {
			c[i] = byte(r.Intn(4))
		}
		m["few"+string(rune('0'+len(m)%10))] = c
	}
	return m
}

// TestRoundTrip: 圧縮して展開すると元に戻り、標準の決まり (最後の 5 バイトは文字、最後の一致は終わりの 12 バイトより前から) を守る。
func TestRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for name, src := range samples(r) {
		c := Compress(src)
		d, err := Decompress(c)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(d, src) {
			t.Fatalf("%s: 元に戻らない (%d → %d → %d)", name, len(src), len(c), len(d))
		}
		checkRules(t, name, c, len(src))
		t.Logf("%s: %d → %d", name, len(src), len(c))
	}
	if c := Compress(bytes.Repeat([]byte("ab"), 1000)); len(c) > 20 {
		t.Errorf("繰り返しが縮まない: %d", len(c))
	}
}

// checkRules は列を辿って、一致が最後の 12 バイトより前から始まり最後の 5 バイトより前で終わるかを見る。
func checkRules(t *testing.T, name string, c []byte, n int) {
	t.Helper()
	i, pos := 0, 0
	for i < len(c) {
		tok := c[i]
		i++
		ll := int(tok >> 4)
		if ll == 15 {
			for {
				b := int(c[i])
				i++
				ll += b
				if b != 255 {
					break
				}
			}
		}
		i += ll
		pos += ll
		if i >= len(c) {
			break
		}
		i += 2
		ml := int(tok & 15)
		if ml == 15 {
			for {
				b := int(c[i])
				i++
				ml += b
				if b != 255 {
					break
				}
			}
		}
		ml += minMatch
		if pos >= n-mfLimit || pos+ml > n-lastLiterals {
			t.Errorf("%s: 一致 [%d, %d) が終わり (%d) に近すぎる", name, pos, pos+ml, n)
		}
		pos += ml
	}
}

// TestDecompressCorrupt: 壊れたデータはエラー (距離 0、書いた所より前、途中で終わる)。
func TestDecompressCorrupt(t *testing.T) {
	for _, c := range [][]byte{
		{0x10, 'a', 0, 0},    // 距離 0
		{0x10, 'a', 2, 0},    // 書いた所より前
		{0xf0},               // 文字の数の続きが無い
		{0x50, 'a', 'b'},     // 文字が足りない
		{0x10, 'a', 1},       // 距離が 1 バイトだけ
	} {
		if _, err := Decompress(c); err == nil {
			t.Errorf("% x: エラーにならない", c)
		}
	}
}
