package ir

import (
	"math/big"
	"strings"
)

// FloatValue は浮動小数のビット bits (size 2 = binary16、4 = binary32) の正確な値。nan / inf なら r は nil。
func FloatValue(bits uint64, size int) (r *big.Rat, neg, nan, inf bool) {
	eb, mb := 8, 23
	if size == 2 {
		eb, mb = 5, 10
	}
	neg = bits>>uint(eb+mb)&1 != 0
	e := int(bits>>uint(mb)) & (1<<eb - 1)
	m := int64(bits & (1<<uint(mb) - 1))
	bias := 1<<(eb-1) - 1
	if e == 1<<eb-1 {
		return nil, neg, m != 0, m == 0
	}
	q := e - bias - mb
	if e == 0 {
		q = 1 - bias - mb
	} else {
		m |= 1 << uint(mb)
	}
	r = new(big.Rat).SetInt64(m)
	if q >= 0 {
		r.Mul(r, new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), uint(q))))
	} else {
		r.Quo(r, new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), uint(-q))))
	}
	return r, neg, false, false
}

// FormatFloat は浮動小数のビットを書式 sp の文字 (ASCII) にする。sema の @format の定数・fclog の @log・テストの模型が使い、
// fclib/fmt.fc の f32 と同じ結果:
//   - NaN は "nan"、無限大は "inf" / "-inf" (0 埋めはしない)
//   - `{}`: 7 桁に四捨五入 (絶対値で) した指数の形 d.dddddde+XX の指数が -4〜8 (と 0) なら、小数点以下 6 桁に四捨五入して
//     末尾の 0 を省く (小数部が 0 なら小数点も)。ほかは指数の形で末尾の 0 を省く (指数は 2 桁以上。C の %g と同じ決め方)
//   - `{:.N}`: 1e9 未満は小数点以下 N 桁、1e9 以上は指数の形で小数点以下 N 桁 (どちらも末尾の 0 を省かない)
func FormatFloat(bits uint64, size int, sp LogSpec) string {
	r, neg, nan, inf := FloatValue(bits, size)
	var body string
	switch {
	case nan:
		body, neg = "nan", false
	case inf:
		body = "inf"
	default:
		body = floatDigits(r, sp.Prec)
	}
	n := len(body)
	if neg {
		n++
	}
	pad := max(sp.Width-n, 0)
	switch {
	case sp.Zero && !nan && !inf && neg:
		return "-" + strings.Repeat("0", pad) + body
	case sp.Zero && !nan && !inf:
		return strings.Repeat("0", pad) + body
	case neg:
		return strings.Repeat(" ", pad) + "-" + body
	}
	return strings.Repeat(" ", pad) + body
}

// floatDigits は 0 以上の正確な値 a の 10 進 (FormatFloat の符号と幅を除いた部分)。prec は `{:.N}` の N + 1 (0 は `{}`)。
func floatDigits(a *big.Rat, prec int) string {
	ten := big.NewRat(10, 1)
	pow := func(k int) *big.Rat {
		return new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(k)), nil))
	}
	big9 := pow(9)
	n, trim := prec-1, false
	if prec == 0 {
		n, trim = 6, true
	}
	// 四捨五入 (絶対値で): floor(x + 1/2)
	roundHalf := func(x *big.Rat) *big.Int {
		y := new(big.Rat).Add(x, big.NewRat(1, 2))
		return new(big.Int).Quo(y.Num(), y.Denom())
	}
	// 指数の形にしたときの指数 k と、小数点以下 n 桁の仮数 (n+1 桁の整数)
	sci := func(n int) (*big.Int, int) {
		k := 0
		x := new(big.Rat).Set(a)
		for x.Cmp(ten) >= 0 {
			x.Quo(x, ten)
			k++
		}
		for x.Cmp(big.NewRat(1, 1)) < 0 {
			x.Mul(x, ten)
			k--
		}
		v := roundHalf(new(big.Rat).Mul(x, pow(n)))
		if v.Cmp(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n+1)), nil)) >= 0 {
			v.Quo(v, big.NewInt(10)) // 9.99… が 10.0… に繰り上がった
			k++
		}
		return v, k
	}
	var fixed bool
	if prec != 0 {
		fixed = a.Cmp(big9) < 0
	} else if a.Sign() == 0 {
		fixed = true
	} else {
		_, k := sci(6) // C の %g と同じく、7 桁に丸めた後の指数で決める
		fixed = k >= -4 && k <= 8
	}
	if fixed {
		v := roundHalf(new(big.Rat).Mul(a, pow(n)))
		s := v.String()
		if len(s) <= n {
			s = strings.Repeat("0", n-len(s)+1) + s
		}
		ip, fp := s[:len(s)-n], s[len(s)-n:]
		if trim {
			fp = strings.TrimRight(fp, "0")
		}
		if fp == "" {
			return ip
		}
		return ip + "." + fp
	}
	v, k := sci(n)
	s := v.String()
	fp := s[1:]
	if trim {
		fp = strings.TrimRight(fp, "0")
	}
	m := s[:1]
	if fp != "" {
		m += "." + fp
	}
	es := "+"
	if k < 0 {
		es, k = "-", -k
	}
	ks := big.NewInt(int64(k)).String()
	if len(ks) < 2 {
		ks = "0" + ks
	}
	return m + "e" + es + ks
}
