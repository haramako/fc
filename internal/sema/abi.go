package sema

// 関数の呼び出し規約の指定 (options(abi:) / scratch / fastcall) の検査 (Agent/wiki/plans/v4-plan.md §2)。
//
//	abi: "frame"  asm の関数と、asm から呼ぶ fc の関数の固定の規約: 静的フレーム F_sym に戻り値 (0) → 引数 (宣言の順)。
//	              レジスタは使わない。scratch: N で引数の後ろに作業領域 N バイト (frames.Analyze が配置する)
//	abi: "stack"  S+k,x で受け取る (X を保存する)
//	abi: "cc65"   cc65 の __fastcall__ (引数 1 個までを A / X。extern だけ)
//
// fc 4 は fastcall を廃止し、extern (本体の無い関数) は規約を書くのが必須。本体のある関数は書かなければコンパイラが決める。
// fc 3 は今のまま (extern の既定は stack、fastcall も使える。abi: "frame" は足した値なので fc 3 でも使える)。

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
)

// checkABI は関数 name の規約の指定を検査する (extern なら本体が無い)。
func (h *Hlc) checkABI(name string, opts ir.Options, extern bool) {
	abi := ""
	if v, ok := opts.Get("abi"); ok {
		abi = v.Text()
		switch abi {
		case "frame", "stack", "cc65":
		default:
			if h.v4() {
				panic(&diag.Error{Msg: fmt.Sprintf("%s: unknown abi %q (abi is \"frame\", \"stack\" or \"cc65\")", name, abi)})
			}
		}
	}
	if opts.Has("scratch") {
		n, ok := opts.Int("scratch")
		switch {
		case abi != "frame":
			panic(&diag.Error{Msg: fmt.Sprintf("%s: scratch is for abi \"frame\" functions (the work area after the arguments in the static frame)", name)})
		case !extern:
			panic(&diag.Error{Msg: fmt.Sprintf("%s: scratch is for an abi \"frame\" function written in assembler (a function with a body has its own locals)", name)})
		case !ok || n < 0 || n > 255:
			panic(&diag.Error{Msg: fmt.Sprintf("%s: scratch takes the number of bytes (0-255)", name)})
		}
	}
	if abi == "frame" && opts.Has("fastcall") {
		panic(&diag.Error{Msg: fmt.Sprintf("%s: abi \"frame\" cannot be combined with fastcall", name)})
	}
	if !h.v4() {
		return
	}
	if opts.Has("fastcall") {
		panic(&diag.Error{Msg: fmt.Sprintf("%s: fastcall is removed in fc 4 (remove it: the compiler chooses the calling convention; a function written in assembler declares @(abi: \"frame\"))", name)})
	}
	if extern && abi == "" {
		panic(&diag.Error{Msg: fmt.Sprintf("%s: a function without a body needs its calling convention: @(abi: \"frame\") (arguments in its static frame F_<sym>, the same as fc functions), @(abi: \"stack\") (S+k,x) or @(abi: \"cc65\")", name)})
	}
}
