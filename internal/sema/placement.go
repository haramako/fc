package sema

import (
	"strings"
	"unicode"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
)

func validateBss(v ir.OptionValue) {
	if v.Kind != ir.OptStr || strings.TrimSpace(v.Str) == "" {
		panic(&diag.Error{Msg: "bss must be a non-empty segment name string"})
	}
	for _, c := range v.Str {
		if unicode.IsControl(c) || c == '"' || c == '\\' {
			panic(&diag.Error{Msg: "bss segment name cannot contain control characters, quotes or backslashes"})
		}
	}
}

func (h *Hlc) placementBss(s *syntax.PlacementBlock) string {
	h.mustInModule()
	for _, e := range s.Options.Entries {
		if e.Key.Name != "bss" {
			h.updatePos(e.Key)
			panic(&diag.Error{Msg: "placement blocks only accept options(bss: ...); segment remains an individual declaration or code option"})
		}
	}
	expr := s.Options.Get("bss")
	if expr == nil {
		panic(&diag.Error{Msg: "placement block requires options(bss: ...)"})
	}
	h.updatePos(expr)
	value := optionValueOf(mustValue(h.constEval(toC(expr))))
	validateBss(value)
	return value.Str
}

func validatePlacementChildren(s *syntax.PlacementBlock) {
	// Restrict the initial form to storage declarations. In particular, module
	// options and executable statements must not acquire block-local semantics.
	for _, child := range s.Body.Stmts {
		allowed := false
		switch d := child.(type) {
		case *syntax.VarDecl:
			allowed = !d.Const
		case *syntax.SoaDecl:
			allowed = !d.Const
		case *syntax.PlacementBlock, *syntax.EmptyStmt:
			allowed = true
		}
		if !allowed {
			panic(&diag.Error{Msg: "placement blocks may only contain var, mutable soa, and nested placement blocks"})
		}
	}
}

func (h *Hlc) compilePlacementBlock(s *syntax.PlacementBlock) {
	h.mustInModule()
	value := h.placementBss(s)
	validatePlacementChildren(s)
	previous := h.groupBss
	h.groupBss = value
	defer func() { h.groupBss = previous }()
	h.compileStmts(s.Body.Stmts)
}
