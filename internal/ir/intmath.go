package ir

// fc 言語の整数セマンティクス (定数畳み込みと実行時で共通): 除算・剰余は床 (商は負の無限大方向、余りの符号は除数に従う)。

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

// Shl は左シフト (b >= 0)。
func Shl(a, b int) int {
	return a << uint(b)
}

// Shr は算術右シフト (b >= 0)。
func Shr(a, b int) int {
	return a >> uint(b)
}
