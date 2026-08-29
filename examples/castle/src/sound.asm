    .include "macro.asm"

    .global _nsd_bgm_BGM0
    .global _nsd_bgm_BGM1
    .global _nsd_bgm_BGM2
    .global _nsd_bgm_BGM3
    .global _nsd_bgm_BGM4
    .global _nsd_bgm_BGM5
    .global _nsd_bgm_BGM6
    .global _nsd_bgm_BGM7
    .global _nsd_bgm_BGM8
    .global _nsd_bgm_BGM9
    .global _nsd_bgm_BGM10

    .global _nsd_se_SE0
    .global _nsd_se_SE1
    .global _nsd_se_SE2
    .global _nsd_se_SE3
    .global _nsd_se_SE4
    .global _nsd_se_SE5
    .global _nsd_se_SE6
    .global _nsd_se_SE7
    .global _nsd_se_SE8
    .global _nsd_se_SE9
    .global _nsd_se_SE10
    .global _nsd_se_SE11
    .global _nsd_se_SE12

    PBANK_SOUND_DATA = 20

;;; NSDの呼び出し準備
;;; pbank0+1の切り替え、xレジスタ保存
;;; use: FC_FASTCALL_REG+14~15
.macro nsd_begin
    txa
    pha

    ;;; pbank_bakを保存する
    lda _mmc3_pbank_bak+0
    sta FC_FASTCALL_REG+14
    lda _mmc3_pbank_bak+1
    sta FC_FASTCALL_REG+15


    sei
    ldx #PBANK_SOUND_DATA+0  ; pbankをSOUND_DATAに変更
    stx _mmc3_pbank_bak+0
    mmc3_pbank 0
    ldx #PBANK_SOUND_DATA+1
    stx _mmc3_pbank_bak+1
    mmc3_pbank 1
    cli
.endmacro

;;; nsd_beginと対をなす
.macro nsd_end
    ;;; pbank_bakを復帰する
    lda FC_FASTCALL_REG+14
    sta _mmc3_pbank_bak+0
    lda FC_FASTCALL_REG+15
    sta _mmc3_pbank_bak+1

    sei
    ldx _mmc3_pbank_bak+0   ; pbankを復帰
    mmc3_pbank 0
    ldx _mmc3_pbank_bak+1
    mmc3_pbank 1
    cli

	pla
	tax
.endmacro

;全BGMトラックの __Sequence_ptr 上位バイトが 0 なら演奏終了
;戻り値: A=0 演奏中 / A=1 終了
.proc   _sound_bgm_finished
        ldy     #nsd::TR_BGM1
@loop:  lda     __Sequence_ptr + 1,y
        bne     @playing
        iny
        iny
        cpy     #nsd::TR_BGM1 + nsd::BGM_Track * 2
        bne     @loop
        lda     #1
        sta FC_FASTCALL_REG+0
        rts
@playing:
        lda     #0
        sta FC_FASTCALL_REG+0
        rts
.endproc

_sound_main_nsd:
    ldx #PBANK_SOUND_DATA  ; pbankをSOUND_DATAに変更
    mmc3_pbank 0
    ldx #PBANK_SOUND_DATA+1
    mmc3_pbank 1

    jsr _nsd_main
    lda __flag
    sta _sound_nsd_flag

    ldx _mmc3_pbank_bak    ; pbankを復帰
    mmc3_pbank 0
    ldx _mmc3_pbank_bak+1 
    mmc3_pbank 1
    rts

_sound_bgm_table:
    .word _nsd_bgm_BGM0
    .word _nsd_bgm_BGM1
    .word _nsd_bgm_BGM2
    .word _nsd_bgm_BGM3
    .word _nsd_bgm_BGM4
    .word _nsd_bgm_BGM5
    .word _nsd_bgm_BGM6
    .word _nsd_bgm_BGM7
    .word _nsd_bgm_BGM8
    .word _nsd_bgm_BGM9
    .word _nsd_bgm_BGM10

_sound_se_table:
    .word _nsd_se_SE0
    .word _nsd_se_SE1
    .word _nsd_se_SE2
    .word _nsd_se_SE3
    .word _nsd_se_SE4
    .word _nsd_se_SE5
    .word _nsd_se_SE6
    .word _nsd_se_SE7
    .word _nsd_se_SE8
    .word _nsd_se_SE9
    .word _nsd_se_SE10
    .word _nsd_se_SE11
    .word _nsd_se_SE12

_sound_nsd_init:
    nsd_begin
	jsr _nsd_init
    nsd_end
    rts

_sound_play_bgm:
	lda S+0,x
	asl a
	tay

    nsd_begin
    
	lda _sound_bgm_table,y
	ldx _sound_bgm_table+1,y
	jsr _nsd_play_bgm

    nsd_end
    rts

_sound_stop_bgm:
    nsd_begin
	jsr _nsd_stop_bgm
    nsd_end
    rts

_sound_nsd_play_se:
	lda S+0,x
	asl a
	tay

    nsd_begin
    
	lda _sound_se_table,y
	ldx _sound_se_table+1,y
	jsr _nsd_play_se

    nsd_end
    rts

_sound_stop_se:
    nsd_begin
	jsr _nsd_stop_se
    nsd_end
    rts

_sound_set_master_volume:
    lda S+0,x
    tay

    nsd_begin

	tya
	jsr _nsd_set_master_volume

    nsd_end
    rts

.proc _sound_nsd_load
    nsd_begin

    lda FC_FASTCALL_REG+0
    ldx FC_FASTCALL_REG+1
	jsr _nsd_load

    nsd_end
    rts
.endproc

.proc _sound_nsd_save
    nsd_begin

    lda FC_FASTCALL_REG+0
    ldx FC_FASTCALL_REG+1
	jsr _nsd_save

    nsd_end
    rts
.endproc

.proc _sound_nsd_resume_bgm
    nsd_begin

	jsr _nsd_resume_bgm

    nsd_end
    rts
.endproc