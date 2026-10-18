	.import FC_FARCALL
	.assert <FC_FARCALL <> $FF, lderror, "FC_FARCALL must not be at $xxFF: jmp (FC_FARCALL) would read the high byte from $xx00 (6502 indirect jump bug)"
	.export farcall_ay

.segment "CODE"

;;; farcall_ay: 別バンクの関数への呼び出し (Agent/wiki/design/farcall.md §3.4)。
;;; emu にはバンクが無いので、FC_FARCALL に置かれたアドレスへそのまま飛ぶだけ (A / X / Y はそのまま呼び先へ渡る)。
farcall_ay:
	jmp (FC_FARCALL)
