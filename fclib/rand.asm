;;; rand.asm: 16 ビットの xorshift (7・9・8) の 1 段 (rand.fc の next_u16。abi "frame")。John Metcalf の 6502 の書き方:
;;; x ^= x << 7; x ^= x >> 9; x ^= x << 8 を、バイトの並べ替えと 1 ビットのずらしで。状態 0 は 1 に読み替える。

_rand_next_u16:
	lda _rand_state
	ora _rand_state+1
	bne @go
	lda #1				; 0 なら 1 に
	sta _rand_state
@go:
	lda _rand_state+1
	lsr a
	lda _rand_state
	ror a
	eor _rand_state+1
	sta _rand_state+1		; 上位 ^= (x << 7) の上位
	ror a				; (x >> 9) と、x << 7 の下位のビット
	eor _rand_state
	sta _rand_state			; 下位 ^= (x >> 9) と (x << 7) の下位
	eor _rand_state+1
	sta _rand_state+1		; 上位 ^= x << 8
	sta F_rand_next_u16+1
	lda _rand_state
	sta F_rand_next_u16+0
	rts
