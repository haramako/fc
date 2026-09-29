// Package lz4 は LZ4 のブロック形式 (フレームの頭の無い、圧縮したデータだけの形) の圧縮と展開。fc の組み込み @lz4 が
// コンパイル時にデータを圧縮し、fclib/lz4.fc (6502 の asm) が実行時に展開する (doc/v4_stdlib.md §8 の 10)。
//
// 形式: 列の並び。1 つの列は「トークン (上位 4 ビットが文字の数、下位 4 ビットが一致の長さ - 4。15 なら続くバイトを足す:
// 255 なら更に続く)・文字・一致の距離 (2 バイト、下位が先。1〜65535)・一致の長さの続き」。最後の列は文字だけ。
// 標準の決まり (最後の 5 バイトは文字、最後の一致は終わりの 12 バイトより前から始まる) を守るので、ほかの LZ4 の展開器でも読める。
package lz4

import "errors"

const (
	minMatch     = 4
	lastLiterals = 5  // 最後の 5 バイトは文字にする
	mfLimit      = 12 // 一致は終わりの 12 バイトより前から始める
	maxOffset    = 65535
	maxChain     = 4096 // 一致を探すときに辿る候補の数 (コンパイル時なので多めに)
)

// Compress は src を LZ4 のブロック形式にする (一致を貪欲に、1 つ先を見て長いほうを選ぶ)。
func Compress(src []byte) []byte {
	n := len(src)
	var out []byte
	emitLen := func(v int) {
		for v >= 255 {
			out = append(out, 255)
			v -= 255
		}
		out = append(out, byte(v))
	}
	emit := func(lit []byte, off, mlen int) {
		ll, ml := len(lit), mlen-minMatch
		tok := byte(min(ll, 15)) << 4
		if off > 0 {
			tok |= byte(min(ml, 15))
		}
		out = append(out, tok)
		if ll >= 15 {
			emitLen(ll - 15)
		}
		out = append(out, lit...)
		if off > 0 {
			out = append(out, byte(off), byte(off>>8))
			if ml >= 15 {
				emitLen(ml - 15)
			}
		}
	}
	if n <= mfLimit {
		emit(src, 0, 0)
		return out
	}
	key := func(i int) uint32 {
		return uint32(src[i]) | uint32(src[i+1])<<8 | uint32(src[i+2])<<16 | uint32(src[i+3])<<24
	}
	head := map[uint32]int{}
	prev := make([]int, n)
	next := 0 // 次に表に入れる位置
	insertTo := func(end int) {
		for ; next < end && next+minMatch <= n; next++ {
			k := key(next)
			if p, ok := head[k]; ok {
				prev[next] = p
			} else {
				prev[next] = -1
			}
			head[k] = next
		}
	}
	matchEnd := n - lastLiterals // 一致はここまでに終わる
	find := func(i int) (int, int) {
		insertTo(i)
		p, ok := head[key(i)]
		if !ok {
			return 0, 0
		}
		best, bestOff := 0, 0
		for c := 0; p >= 0 && i-p <= maxOffset && c < maxChain; c++ {
			l := 0
			for i+l < matchEnd && src[p+l] == src[i+l] {
				l++
			}
			if l > best {
				best, bestOff = l, i-p
			}
			p = prev[p]
		}
		if best < minMatch {
			return 0, 0
		}
		return best, bestOff
	}
	anchor, i := 0, 0
	last := n - mfLimit // 一致を始められるのはここより前
	for i < last {
		l, off := find(i)
		if l == 0 {
			i++
			continue
		}
		// 1 つ先から始めるほうが長ければ、今の 1 バイトを文字にする
		if i+1 < last {
			if l2, off2 := find(i + 1); l2 > l {
				i++
				l, off = l2, off2
			}
		}
		emit(src[anchor:i], off, l)
		i += l
		anchor = i
	}
	emit(src[anchor:], 0, 0)
	return out
}

// ErrCorrupt は形式の誤り (途中で終わる、距離が 0 か書いた所より前)。
var ErrCorrupt = errors.New("lz4: corrupt data")

// Decompress は LZ4 のブロック形式を展開する (テストと検査用。fclib/lz4.asm と同じく、入力を読み終えたら終わる)。
func Decompress(src []byte) ([]byte, error) {
	var out []byte
	i := 0
	readLen := func(v int) (int, error) {
		if v != 15 {
			return v, nil
		}
		for {
			if i >= len(src) {
				return 0, ErrCorrupt
			}
			b := int(src[i])
			i++
			v += b
			if b != 255 {
				return v, nil
			}
		}
	}
	for i < len(src) {
		tok := src[i]
		i++
		ll, err := readLen(int(tok >> 4))
		if err != nil {
			return nil, err
		}
		if i+ll > len(src) {
			return nil, ErrCorrupt
		}
		out = append(out, src[i:i+ll]...)
		i += ll
		if i >= len(src) {
			break
		}
		if i+2 > len(src) {
			return nil, ErrCorrupt
		}
		off := int(src[i]) | int(src[i+1])<<8
		i += 2
		if off == 0 || off > len(out) {
			return nil, ErrCorrupt
		}
		ml, err := readLen(int(tok & 15))
		if err != nil {
			return nil, err
		}
		for k := 0; k < ml+minMatch; k++ {
			out = append(out, out[len(out)-off])
		}
	}
	return out, nil
}
