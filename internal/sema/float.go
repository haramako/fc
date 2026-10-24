package sema

// 浮動小数 (f16 / f32 = IEEE 754 binary16 / binary32) の規則・定数の畳み込み・実行時の下ろし方 (Agent/wiki/design/float-fixed.md)。

import (
	"math/big"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

func errNoFloat() *diag.Error {
	return &diag.Error{Msg: "floating-point types are not implemented yet"}
}

// adaptFloat は型のない定数 x を浮動小数の相手 other に合わせる。
func (h *Hlc) adaptFloat(x, other exprInfo) (adaptedLit, bool) { panic(errNoFloat()) }

// planFloat は浮動小数の二項の算術の計画。
func (h *Hlc) planFloat(op cop, a, b exprInfo) (arithPlan, bool) { panic(errNoFloat()) }

// foldFloat は浮動小数の定数の二項の演算。
func (h *Hlc) foldFloat(c *cexpr, vals []*ir.Value, infos []exprInfo) (*cexpr, bool) {
	panic(errNoFloat())
}

// foldFloatUnary は浮動小数の定数の単項の演算。
func (h *Hlc) foldFloatUnary(op cop, v *ir.Value) (*cexpr, bool) { panic(errNoFloat()) }

// constFloatCast は浮動小数の定数の `as`。
func (h *Hlc) constFloatCast(v *ir.Value, to *types.Type) (*ir.Value, bool) { panic(errNoFloat()) }

// ratToFloatLit は正確な値 r を浮動小数 t の定数にする。
func (h *Hlc) ratToFloatLit(r *big.Rat, t *types.Type) *ir.Value { panic(errNoFloat()) }

// floatCast は浮動小数が関わる実行時の `as`。
func (h *Hlc) floatCast(v ir.Operand, to *types.Type) ir.Operand { panic(errNoFloat()) }

// emitFloatArith は浮動小数の二項の算術の値。
func (h *Hlc) emitFloatArith(op cop, p arithPlan, left, right ir.Operand) ir.Operand {
	panic(errNoFloat())
}
