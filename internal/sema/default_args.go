package sema

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// Defaults are resolved after signatures, in the declaration's scope. This
// permits forward constants and self/mutual function-symbol references without
// introducing a dependency cycle between otherwise independent signatures.
type functionDefaults struct {
	owner  *Hlc
	lambda *ir.Lambda
	params []lambdaParam
	values []*cexpr
	min    int
	state  resolutionState
}

func (h *Hlc) registerDefaults(lmd *ir.Lambda, params []lambdaParam) {
	first := len(params)
	for i, p := range params {
		if p.init != nil {
			if first == len(params) {
				first = i
			}
		} else if first != len(params) {
			panic(&diag.Error{Msg: fmt.Sprintf("required parameter %s cannot follow a default argument", p.name), Pos: syntax.At(h.module.Path, p.typ.Pos())})
		}
	}
	if first == len(params) {
		return
	}
	owner := &Hlc{prog: h.prog, deps: h.deps, module: h.module, scope: h.scope}
	h.prog.defaults[lmd] = &functionDefaults{owner: owner, lambda: lmd, params: params, min: first}
}

func (d *functionDefaults) resolve() {
	if d.state == resolutionComplete {
		return
	}
	if d.state == resolutionFailed {
		panic(&diag.Error{Suppressed: true})
	}
	if d.state == resolutionActive {
		panic(&diag.Error{Msg: "cyclic default argument dependency", Pos: d.lambda.Pos})
	}
	d.state = resolutionActive
	h := *d.owner
	h.curPos = d.lambda.Pos
	h.scope = ir.NewScope(h.scope)
	outer := h.prog.curModule
	h.prog.curModule = h.module.Id
	defer func() {
		h.prog.curModule = outer
		if r := recover(); r != nil {
			d.state = resolutionFailed
			if e, ok := r.(*diag.Error); ok && !e.Pos.IsValid() {
				e.Pos = h.curPos
			}
			panic(r)
		}
	}()
	// Parameters shadow outer names, but cannot supply runtime values here.
	for _, p := range d.lambda.Params {
		h.scope.Declare(ir.NewLocal(p.Name, p.Type, ir.LTArg))
	}
	d.values = make([]*cexpr, len(d.params))
	for i := d.min; i < len(d.params); i++ {
		param := d.params[i]
		h.updatePos(param.init)
		typ := d.lambda.Params[i].Type
		value := h.constEval(h.withExpected(toC(param.init), typ))
		value = h.constantDefault(value, typ)
		if !isConstLiteral(value) {
			panic(&diag.Error{Msg: fmt.Sprintf("default argument %s of %s must be a constant expression", param.name, d.lambda.Name)})
		}
		v := h.fitArrayLiteral(value.val, typ)
		h.compatibleAssign("default argument "+param.name+" of "+d.lambda.Name, typ, v.Type)
		d.values[i] = cv(v)
	}
	d.state = resolutionComplete
}

// Named aggregate constants are stored in ROM blocks. Copy their constant
// initializer for value parameters; pointer parameters retain the ROM address.
// Never read a mutable global or local variable's runtime contents.
func (h *Hlc) constantDefault(c *cexpr, typ *types.Type) *cexpr {
	if c.kind == cValue && c.val.Kind == ir.KindArrayLiteral && c.val.Type.Kind == types.Array && typ.Kind == types.Pointer {
		h.compatibleAssign("default argument", typ, c.val.Type)
		sym := h.addDef(h.tmpName("_default_"), &ir.Def{Kind: ir.DefBlock, Type: c.val.Type, Elems: c.val.Elems})
		return cv(ir.NewSymbolLiteral("", typ, sym))
	}
	if c.kind != cValue || c.val.Kind != ir.KindGlobal || c.val.Symbol == "" {
		return c
	}
	for _, m := range h.prog.Modules.List() {
		for _, def := range m.Defs {
			if def.Sym == c.val.Symbol && def.Kind == ir.DefBlock {
				if typ.Kind == types.Pointer && def.Type.Kind == types.Array {
					h.compatibleAssign("default argument", typ, def.Type)
					return cv(ir.NewSymbolLiteral("", typ, def.Sym))
				}
				return cv(ir.NewArrayLiteral("", def.Type, def.Elems))
			}
		}
	}
	return c
}

func (h *Hlc) fillDefaultArgs(callee ir.Operand, args []*cexpr) []*cexpr {
	typ := ir.ValType(callee)
	if typ.Kind != types.Func || len(args) >= len(typ.Params) {
		return args
	}
	// This decision precedes optimization. A pointer variable does not inherit
	// defaults even if constant propagation later determines its target.
	lit := ir.ValLiteral(callee)
	if lit == nil || lit.Kind != ir.KindLiteral || lit.IsInt {
		return args
	}
	lmd := h.prog.lambdas[lit.Symbol]
	if lmd == nil || !types.SameFuncSignature(lmd.Type, typ) {
		return args
	}
	d := h.prog.defaults[lmd]
	if d == nil {
		return args
	}
	d.resolve()
	if len(args) < d.min {
		panic(&diag.Error{Msg: fmt.Sprintf("%s expects %d..%d argument(s) but %d given", describe(callee), d.min, len(typ.Params), len(args))})
	}
	r := append([]*cexpr(nil), args...)
	return append(r, d.values[len(args):]...)
}
