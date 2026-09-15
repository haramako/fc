package codegen

import "fmt"

// 関数呼び出し (call マクロ / fastcall / far call)。

// farCallSetup は far call の呼び先アドレスとバンク番号 (ld65.cfg の bank = N を .bank で引く) を FC_FARCALL に置く。
func (l *Llc) farCallSetup(sym string) []any {
	s := mangle(sym)
	return []any{
		fmt.Sprintf("lda #<%s", s), "sta FC_FARCALL+0",
		fmt.Sprintf("lda #>%s", s), "sta FC_FARCALL+1",
		fmt.Sprintf("lda #<.bank(%s)", s), "sta FC_FARCALL+2",
	}
}

// callSubroutine はスタックポインタ(X)を進めて jsr する。
func (l *Llc) callSubroutine(addr string, frameSize int) any {
	if frameSize <= 4 {
		r := []string{}
		for i := 0; i < frameSize; i++ {
			r = append(r, "inx")
		}
		r = append(r, fmt.Sprintf("jsr %s", addr))
		for i := 0; i < frameSize; i++ {
			r = append(r, "dex")
		}
		return r
	}
	return fmt.Sprintf("call %s, #%d", addr, frameSize)
}
