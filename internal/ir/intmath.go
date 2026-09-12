package ir

// fc 言語の定数演算の整数セマンティクス (Ruby 由来: floor 除算・floor 剰余・負のシフト量は逆方向)。

// FloorDiv は floor 除算。
func FloorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// FloorMod は floor 剰余 (結果の符号は除数に従う)。
func FloorMod(a, b int) int {
	m := a % b
	if m != 0 && (m < 0) != (b < 0) {
		m += b
	}
	return m
}

// Shl は左シフト (負の量は右シフト)。
func Shl(a, b int) int {
	if b < 0 {
		return Shr(a, -b)
	}
	return a << uint(b)
}

// Shr は右シフト (負の量は左シフト)。
func Shr(a, b int) int {
	if b < 0 {
		return Shl(a, -b)
	}
	return a >> uint(b)
}
