package sema

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// The declaration has its own name, type and visibility. Its uses lower to
// casts of the SAME global Value, so every analysis sees the shared storage.
// Bindings never enter the IR variable list or allocate an assembler symbol.
func (h *Hlc) compileStorageAlias(s *syntax.VarDecl) {
	if h.lmd != nil && s.PublicPos.IsValid() {
		panic(&diag.Error{Msg: "public alias is only allowed at module scope"})
	}
	sp := s.Specs[0]
	typ := h.typeEval(sp.Type)
	h.checkComplete(typ, "alias "+sp.Name.Name)
	if !storageAliasType(typ) {
		panic(&diag.Error{Msg: "alias requires a non-empty, fixed-size storage type"})
	}
	c := toC(sp.Init)
	var target *ir.Value
	switch c.kind {
	case cIdent:
		target = h.scope.FindMust(c.name, true)
	case cDot:
		if c.args[0].kind == cIdent {
			m := h.scope.FindMust(c.args[0].name, true)
			if m.Module != nil {
				target = m.Module.LookupMust(c.name)
			}
		}
	}
	if target == nil {
		panic(&diag.Error{Msg: "alias target must name a global variable or storage alias (no fields, indexing or pointer dereference)"})
	}
	if target.Type.Kind == types.Bad {
		panic(&diag.Error{Suppressed: true})
	}
	root := target
	if original := h.prog.storageAliases[target]; original != nil {
		root = original
	}
	if !h.prog.storageGlobals[root] || !storageAliasType(target.Type) {
		panic(&diag.Error{Msg: "alias target must be mutable global storage, not a local, constant or soa"})
	}
	if typ.Size > target.Type.Size {
		panic(&diag.Error{Msg: fmt.Sprintf("alias %s needs %d bytes but target %s has %d bytes", sp.Name.Name, typ.Size, target.Name, target.Type.Size)})
	}
	binding := ir.NewGlobal(sp.Name.Name, typ, "")
	binding.Public = s.PublicPos.IsValid()
	h.scope.Declare(binding)
	h.prog.storageAliases[binding] = root
}

func storageAliasType(t *types.Type) bool {
	if t == nil || t.Size <= 0 || t.IsSoa {
		return false
	}
	switch t.Kind {
	case types.Int, types.Bool, types.Pointer, types.Func, types.Struct:
		return true
	case types.Array:
		return t.Length > 0 && storageAliasType(t.Base)
	}
	return false
}
