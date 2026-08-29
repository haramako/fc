;; // 敵の弾の処理
;; function en_bul_process():void
;; {
;;   var i = 0;
;;   while( i<EN_BUL_MAX ){
;;     if( en_bul_type[i] ){
;;       en_bul_y[i] += (en_bul_vy[i]+giff16) / 16;
;;       en_bul_x[i] += (en_bul_vx[i]+giff16) / 16;
;;       gr_sprite( en_bul_x[i]-4, en_bul_y[i]-4, SPR_EN_BUL+anim, 1 );
;;       // 自機との当たり判定
;;       if( my_muteki == 0 && my_x + 4 - en_bul_x[i] < 8 && my_y + 4 - en_bul_y[i] < 8 ){
;;         my_bang = 1;
;;         en_bul_type[i] = 0;
;;       }
;;       // 死亡判定
;;       if( en_bul_y[i] < 8 || en_bul_y[i] > 248 || en_bul_x[i] < 8 || en_bul_x[i] > 248 ){
;;           en_bul_type[i] = 0;
;;       }
;;     }
;;     i += 1;
;;   }
;; }
;; USING: X,Y
.proc _en_bul_process
    ldy #0						; i = 0
@loop:							; while(...

    lda _en_bul_type,y			; if( en_bul_type[i] ){
    bne @then4
    jmp @end4
@then4:
    lda _en_bul_vy,y       ; en_bul_y[i] += en_bul_vy[i] / 4;
	clc
	adc _common_giff16
	cmp #$80
    ror a
	cmp #$80
    ror a
	cmp #$80
    ror a
	cmp #$80
    ror a
    clc
    adc _en_bul_y,y
    sta _en_bul_y,y

    lda _en_bul_vx,y       ; en_bul_x[i] += en_bul_vx[i] + giff16 / 16;
	clc
	adc _common_giff16
	cmp #$80
    ror a
	cmp #$80
    ror a
	cmp #$80
    ror a
	cmp #$80
    ror a
    clc
    adc _en_bul_x,y
    sta _en_bul_x,y
    lda _en_bul_x,y        ; gr_sprite( en_bul_x[i]-4, en_bul_y[i]-4, SPR_EN_BUL+anim, 1 );
    sec
    sbc #4
    sta S+0,x
    lda _en_bul_y,y        
    sec
    sbc #4
    sta S+1,x
    lda #$e0
    clc
    adc _common_anim
    sta S+2,x
    lda #1                  
    sta S+3,x
	tya
	pha
    jsr _ppu_sprite
	pla
	tay
    lda _common_my_muteki         ; if( my_muteki == 0 && my_x + 8 - en_bul_x[i] < 16 && my_y + 8 - en_bul_y[i] < 16 ){
    bne @end3
    lda _common_my_x              
    clc
    adc #8
    sec
    sbc _en_bul_x,y
    cmp #16
    bcs @end3
    lda _common_my_y
    clc
    adc #8
    sec
    sbc _en_bul_y,y
    cmp #16
    bcs @end3
    lda #1					; my_bang = 1;
    sta _common_my_bang
	jsr @killed
	jmp @loop_end			; break
@end3:
    lda _en_bul_y,y			; if( en_bul_y[i] < 8 || en_bul_y[i] > 248 || en_bul_x[i] < 8 || en_bul_x[i] > 248 ){
    cmp #8
    bcc @kill
    cmp #240
    bcs @kill
    lda _en_bul_x,y
    cmp #8
    bcc @kill
    cmp #240
    bcs @kill
    jmp @loop_end
@kill:
	jsr @killed
@end4:
@loop_end:        
    iny						; i += 2;
    cpy #_en_BUL_MAX		; while( i<EN_BUL_MAX ){
    bcs @loop_end2          
    jmp @loop
@loop_end2:
    rts
	
@killed:
    lda #0                  ; en_bul_type[i] = 0;
    sta _en_bul_type,y
	lda _en_bul_free		; en_bul_x[i] = _en_bul_free
	sta _en_bul_x,y
	sty _en_bul_free		; en_bul_free = i
	rts
.endproc        
        
