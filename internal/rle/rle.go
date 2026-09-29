// Package rle は NES Screen Tool の RLE の形式 (neslib の vram_unrle が読むもの) の圧縮と展開。fc の組み込み @rle がコンパイル時に
// 圧縮し、fclib/rle.fc と fclib/nes/vram.fc が展開する (Agent/wiki/plans/v4-stdlib.md §8 の 10)。
//
// 形式: 先頭の 1 バイトが印 (データに現れない値)。続く並びで、印でないバイトはそのまま出し (直前の値として覚える)、印の次の
// バイト n が 0 なら終わり、そうでなければ直前の値を n 回出す。
package rle

import (
	"errors"
	"fmt"
)

// Compress は src を RLE にする。4 回以上続く値だけを繰り返しにする (3 回までは印を使うと縮まない)。印に使える値 (src に
// 現れない値) が無ければエラー。
func Compress(src []byte) ([]byte, error) {
	var count [256]int
	for _, b := range src {
		count[b]++
	}
	tag := -1
	for v := 255; v >= 0; v-- { // 0 は画面のデータに多いので、後ろから探す
		if count[v] == 0 {
			tag = v
			break
		}
	}
	if tag < 0 {
		return nil, fmt.Errorf("every byte value appears in the data (RLE needs an unused value as the tag)")
	}
	out := []byte{byte(tag)}
	for i := 0; i < len(src); {
		b := src[i]
		n := 1
		for i+n < len(src) && src[i+n] == b {
			n++
		}
		if n < 4 {
			for k := 0; k < n; k++ {
				out = append(out, b)
			}
		} else {
			out = append(out, b)
			for r := n - 1; r > 0; {
				k := min(r, 255)
				out = append(out, byte(tag), byte(k))
				r -= k
			}
		}
		i += n
	}
	return append(out, byte(tag), 0), nil
}

// ErrCorrupt は形式の誤り (終わりの印が無い)。
var ErrCorrupt = errors.New("rle: corrupt data")

// Decompress は RLE を展開する。
func Decompress(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, ErrCorrupt
	}
	tag := src[0]
	var out []byte
	var last byte
	for i := 1; i < len(src); {
		b := src[i]
		i++
		if b != tag {
			out = append(out, b)
			last = b
			continue
		}
		if i >= len(src) {
			return nil, ErrCorrupt
		}
		n := src[i]
		i++
		if n == 0 {
			return out, nil
		}
		for k := 0; k < int(n); k++ {
			out = append(out, last)
		}
	}
	return nil, ErrCorrupt
}
