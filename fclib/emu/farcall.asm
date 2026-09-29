	.import FC_FARCALL
	.export farcall

.segment "CODE"

;;; farcall: 別バンクの関数への呼び出し (Agent/wiki/design/farcall.md §3.4)。
;;; emu にはバンクが無いので、FC_FARCALL に置かれたアドレスへそのまま飛ぶだけ。
farcall:
	jmp (FC_FARCALL)
