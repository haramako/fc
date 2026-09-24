    .include "macro.asm"

;;; 全BGMトラックの __Sequence_ptr 上位バイトが 0 なら演奏終了 (NSD の内部構造を見るので asm)
;;; 戻り値: A=0 演奏中 / A=1 終了 (cc65 規約)
.proc   _sound_bgm_finished
        ldy     #nsd::TR_BGM1
@loop:  lda     __Sequence_ptr + 1,y
        bne     @playing
        iny
        iny
        cpy     #nsd::TR_BGM1 + nsd::BGM_Track * 2
        bne     @loop
        lda     #1
        rts
@playing:
        lda     #0
        rts
.endproc

;;; NMI から呼ばれる (ppu.on_vsync)。割り込み中なので sei/cli も pbank_bak の更新も不要。
;;; fc の関数にすると静的フレームがメインスレッドの関数と重なるので asm のまま
_sound_main_nsd:
    ldx #_common_PBANK_SOUND_DATA  ; pbankをSOUND_DATAに変更
    mmc3_pbank 0
    ldx #_common_PBANK_SOUND_DATA+1
    mmc3_pbank 1

    jsr _nsd_main
    lda __flag
    sta _sound_nsd_flag

    ldx _mmc3_pbank_bak    ; pbankを復帰
    mmc3_pbank 0
    ldx _mmc3_pbank_bak+1 
    mmc3_pbank 1
    rts
