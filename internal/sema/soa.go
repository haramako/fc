package sema

import (
	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

func (h *Hlc) compileSoaDecl(s *syntax.SoaDecl) {
	panic(&diag.Error{Msg: "soa is not implemented yet"})
}

func (h *Hlc) soaField(ref ir.Operand, lv bool, t *types.Type, name string) ir.Operand {
	panic(&diag.Error{Msg: "soa is not implemented yet"})
}
