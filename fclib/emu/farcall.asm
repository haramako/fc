	.import FC_FARCALL
	.export farcall_ay

.segment "CODE"

;;; farcall_ay: 別バンクの関数への呼び出し (Agent/wiki/design/farcall.md §3.4)。
;;; emu にはバンクが無いので、FC_FARCALL に置かれたアドレスへそのまま飛ぶだけ (A / X / Y はそのまま呼び先へ渡る)。
farcall_ay:
	jmp (FC_FARCALL)
