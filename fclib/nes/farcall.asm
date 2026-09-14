	.import FC_FARCALL
	.export farcall

.segment "CODE"

;;; farcall: 別バンクの関数への呼び出し (doc/v2_farcall.md §3.4)。
;;; バンク切替の無いマッパー (MMC0) 用: FC_FARCALL に置かれたアドレスへそのまま飛ぶだけ。
;;; MMC3 などバンク切替のあるマッパーでは、プロジェクトが farcall を用意する (farcall_mmc3.asm を参考に)。
farcall:
	jmp (FC_FARCALL)
