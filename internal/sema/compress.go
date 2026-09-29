package sema

// コンパイル時の圧縮の組み込み (doc/v4_stdlib.md §8 の 10)。

import (
	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/lz4"
	"github.com/haramako/fc/internal/rle"
)

func registerCompressBuiltins(h *Hlc) {
	// @lz4(data) は data (u8 の定数の配列: @incbin("map.bin")・配列リテラル・文字列) を LZ4 のブロック形式に圧縮した u8 の配列の
	// 定数 (fclib/lz4.fc の unpack が展開する)。const MAP = @lz4(@incbin("map.bin"));
	h.defconstmacro("@lz4", func(h *Hlc, args []*cexpr) *cexpr {
		if len(args) != 1 {
			panic(&diag.Error{Msg: "@lz4 takes 1 argument (a constant byte array: @lz4(@incbin(\"map.bin\")))"})
		}
		return h.byteArray(lz4.Compress(h.constBytes(args[0], "@lz4")))
	})

	// @rle(data) は NES Screen Tool の RLE の形式 (先頭が印、印 + n で直前の値を n 回、印 + 0 で終わり) に圧縮した u8 の配列の定数
	// (fclib/rle.fc の unpack、fclib/nes/vram.fc の write_rle_now / put_rle が展開する)。
	h.defconstmacro("@rle", func(h *Hlc, args []*cexpr) *cexpr {
		if len(args) != 1 {
			panic(&diag.Error{Msg: "@rle takes 1 argument (a constant byte array: @rle(@incbin(\"title.nam\")))"})
		}
		b, err := rle.Compress(h.constBytes(args[0], "@rle"))
		if err != nil {
			panic(&diag.Error{Msg: "@rle: " + err.Error()})
		}
		return h.byteArray(b)
	})
}

// constBytes は定数の評価済みの c (u8 の配列リテラルか文字列) のバイト列。
func (h *Hlc) constBytes(c *cexpr, what string) []byte {
	if c.kind == cValue {
		v := c.val
		if v.IsString {
			return []byte(v.Str)
		}
		if v.Kind == ir.KindArrayLiteral {
			b := make([]byte, len(v.Elems))
			for i, e := range v.Elems {
				x := ir.ValLiteral(e)
				if x == nil || !x.IsInt || x.Int < 0 || x.Int > 255 {
					panic(&diag.Error{Msg: what + ": the array elements must be constant bytes (0..255)"})
				}
				b[i] = byte(x.Int)
			}
			return b
		}
	}
	panic(&diag.Error{Msg: what + " takes a constant byte array (@incbin(\"file\"), an array literal or a string)"})
}

// byteArray は b の u8 の配列の定数。
func (h *Hlc) byteArray(b []byte) *cexpr {
	elems := make([]*cexpr, len(b))
	for i, x := range b {
		elems[i] = cint(int(x))
	}
	return h.constEval(carray(elems))
}
