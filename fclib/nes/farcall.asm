	.import FC_FARCALL
	.assert <FC_FARCALL <> $FF, lderror, "FC_FARCALL must not be at $xxFF: jmp (FC_FARCALL) would read the high byte from $xx00 (6502 indirect jump bug)"
	.export farcall_ay

.segment "CODE"

;;; farcall_ay: 別バンクの関数への呼び出し (Agent/wiki/design/farcall.md §3.4)。
;;; バンク切替の無いマッパー (MMC0) 用: FC_FARCALL に置かれたアドレスへそのまま飛ぶだけ (A / X / Y はそのまま呼び先へ渡る)。
;;; バンク切替のあるマッパーは fclib/nes の uxrom / mmc1 / mmc3 のモジュールがトランポリンを持つ (自前なら farcall_mmc3.asm を参考に)。
farcall_ay:
	jmp (FC_FARCALL)
