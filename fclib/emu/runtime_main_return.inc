;;; main から戻った後 (share/runtime.asm の start が .include する)。終了コード 0 で終える:
;;; 終了の番地 $ffff (internal/emu の PortExit) は 255 で始まり、255 以外が書かれたら終わるので、1 足して 0 にする
;;; (nes の jmp * と同じ 3 バイトにして、後ろのランタイムの番地を変えない)
	inc $ffff
