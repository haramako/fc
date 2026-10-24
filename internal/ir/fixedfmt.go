package ir

import (
	"math/big"
	"strings"
)

// FormatFixed は固定小数 (中身 raw、小数のビット数 frac) を書式 sp の文字 (ASCII) にする。sema の @format の定数・fclog の
// @log が使い、fclib/fmt.fc の fixdec と同じ結果。桁の指定が
// 無ければ正確な値 (小数部の末尾の 0 を省き、小数部が 0 なら小数点も無し)、`{:.N}` なら小数点以下 N 桁に四捨五入 (絶対値で)。
func FormatFixed(raw, frac int, sp LogSpec) string {
	neg := raw < 0
	mag := new(big.Int).Abs(big.NewInt(int64(raw)))
	one := new(big.Int).Lsh(big.NewInt(1), uint(frac))
	ip := new(big.Int).Rsh(mag, uint(frac))
	f := new(big.Int).Sub(mag, new(big.Int).Lsh(ip, uint(frac)))
	var digs []byte
	digit := func() {
		f.Mul(f, big.NewInt(10))
		d := new(big.Int).Rsh(f, uint(frac))
		digs = append(digs, byte('0'+d.Int64()))
		f.Sub(f, new(big.Int).Lsh(d, uint(frac)))
	}
	if sp.Prec == 0 {
		for f.Sign() != 0 {
			digit()
		}
	} else {
		for k := 0; k < sp.Prec-1; k++ {
			digit()
		}
		if new(big.Int).Lsh(f, 1).Cmp(one) >= 0 {
			// 四捨五入の繰り上がり (小数部から整数部へ)
			i := len(digs) - 1
			for ; i >= 0 && digs[i] == '9'; i-- {
				digs[i] = '0'
			}
			if i >= 0 {
				digs[i]++
			} else {
				ip.Add(ip, big.NewInt(1))
			}
		}
	}
	digits := ip.String()
	if len(digs) > 0 {
		digits += "." + string(digs)
	}
	body := len(digits)
	if neg {
		body++
	}
	pad := max(sp.Width-body, 0)
	switch {
	case sp.Zero && neg:
		return "-" + strings.Repeat("0", pad) + digits
	case sp.Zero:
		return strings.Repeat("0", pad) + digits
	case neg:
		return strings.Repeat(" ", pad) + "-" + digits
	}
	return strings.Repeat(" ", pad) + digits
}
