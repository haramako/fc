package sema

import (
	"fmt"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

type resolutionState uint8

const (
	resolutionPending resolutionState = iota
	resolutionActive
	resolutionComplete
	resolutionFailed
)

// A declaration is evaluated once, in its own module context. Dependencies can
// request another declaration without sharing expression memoization or scope.
type declaration struct {
	owner    *moduleDecls
	stmt     syntax.Stmt
	name     string
	public   bool
	identity *ir.Value // predeclared struct/SoA value, without triggering lookup tracing
	state    resolutionState
	group    *declaration
	bss      string
	use      *syntax.UseDecl
	imported *ir.Module
	action   func(*Hlc)
}
type moduleDecls struct {
	h       *Hlc
	entries []*declaration
	state   resolutionState
}

func (d *declaration) resolve() {
	if d.state == resolutionComplete {
		return
	}
	if d.state == resolutionFailed {
		panic(&diag.Error{Suppressed: true})
	}
	p := d.owner.h.prog
	if d.state == resolutionActive {
		var path []string
		started := false
		for _, dep := range p.resolving {
			if dep == d {
				started = true
			}
			if started {
				path = append(path, dep.label())
			}
		}
		path = append(path, d.label())
		msg := "cyclic declaration dependency: " + strings.Join(path, " -> ")
		for _, dep := range p.resolving {
			if _, ok := dep.stmt.(*syntax.StructDecl); ok {
				msg += " (struct layout is not complete yet; recursive struct must go through a pointer)"
				break
			}
		}
		panic(&diag.Error{Msg: msg})
	}
	d.state = resolutionActive
	h := &Hlc{prog: p, deps: d.owner.h.deps, module: d.owner.h.module, scope: d.owner.h.scope}
	h.updatePos(d.stmt)
	outer := p.curModule
	p.curModule = h.module.Id
	p.resolving = append(p.resolving, d)
	defer func() {
		p.curModule = outer
		p.resolving = p.resolving[:len(p.resolving)-1]
		if d.name != "" {
			defer h.scope.Complete(d.name)
		}
		if problem := recover(); problem != nil {
			d.state = resolutionFailed
			ce, ok := problem.(*diag.Error)
			if !ok || ce.Fatal {
				panic(problem)
			}
			if !ce.Pos.IsValid() {
				ce.Pos = h.curPos
			}
			h.declareBad(d.stmt)
			p.report(ce)
			panic(&diag.Error{Suppressed: true})
		}
		d.state = resolutionComplete
	}()
	if d.group != nil {
		d.group.resolve()
		h.groupBss = d.group.bss
	}
	if d.action != nil {
		d.action(h)
	} else {
		h.compileStatement(d.stmt)
	}
}
func (d *declaration) label() string {
	if d.name != "" {
		return d.owner.h.module.Id + "." + d.name
	}
	return fmt.Sprintf("%s:%d", d.owner.h.module.Path, d.stmt.Pos().Line)
}

// run reports a top-level diagnostic and continues. Dependencies propagate their
// failure to the requesting declaration without emitting a second diagnostic.
func (md *moduleDecls) run(s syntax.Stmt, fn func()) {
	defer func() {
		if problem := recover(); problem != nil {
			ce, ok := problem.(*diag.Error)
			if !ok || ce.Fatal {
				panic(problem)
			}
			if !ce.Pos.IsValid() {
				ce.Pos = syntax.At(md.h.module.Path, s.Pos())
			}
			md.h.prog.report(ce)
		}
	}()
	fn()
}
func (md *moduleDecls) collect(stmts []syntax.Stmt, group *declaration) {
	for _, s := range stmts {
		md.run(s, func() { md.collectOne(s, group) })
	}
}
func (md *moduleDecls) collectOne(s syntax.Stmt, group *declaration) {
	d := &declaration{owner: md, stmt: s, group: group}
	switch s := s.(type) {
	case *syntax.Block:
		md.collect(s.Stmts, group)
		return
	case *syntax.PlacementBlock:
		// Validate allowed children before registering any of their names.
		validatePlacementChildren(s)
		d.action = func(h *Hlc) { d.bss = h.placementBss(s) }
		md.entries = append(md.entries, d)
		md.collect(s.Body.Stmts, d)
		return
	case *syntax.VarDecl:
		if len(s.Specs) > 1 {
			for _, sp := range s.Specs {
				one := *s
				one.Specs = []*syntax.VarSpec{sp}
				md.collect([]syntax.Stmt{&one}, group)
			}
			return
		}
		d.name, d.public = s.Specs[0].Name.Name, s.PublicPos.IsValid()
	case *syntax.FuncDecl:
		d.name, d.public = s.Name.Name, s.PublicPos.IsValid()
	case *syntax.StructDecl:
		d.name, d.public = s.Name.Name, s.PublicPos.IsValid()
	case *syntax.SoaDecl:
		d.name, d.public = s.Name.Name, s.PublicPos.IsValid()
	case *syntax.UseDecl:
		if len(s.Names) > 1 {
			for _, name := range s.Names {
				one := *s
				one.Names = []*syntax.Ident{name}
				md.collect([]syntax.Stmt{&one}, group)
			}
			return
		}
		d.use, d.public = s, s.PublicPos.IsValid()
		switch {
		case s.FromAll:
		case len(s.Names) > 0:
			d.name = s.Names[0].Name
		case s.As != nil:
			d.name = s.As.Name
		default:
			d.name = s.Module.Name
		}
	}
	if d.name != "" {
		md.h.scope.Reserve(d.name, d.public, d.use != nil && len(d.use.Names) > 0, d.resolve)
	}
	var identity *types.Type
	valueType := md.h.prog.Types.TypeName()
	switch s := s.(type) {
	case *syntax.StructDecl:
		identity = md.h.prog.Types.NewStruct(md.h.module.Id + "." + d.name)
	case *syntax.SoaDecl:
		identity = md.h.prog.Types.SoaArray(md.h.module.Id+"."+d.name, nil, -1, s.Const)
		valueType = identity
	}
	if identity != nil {
		// Pointer/handle identity is available before the storage layout.
		v := ir.NewTypeValue(d.name, valueType, identity)
		v.Public = d.public
		d.identity = v
		md.h.scope.Declare(v)
		md.h.scope.Complete(d.name)
		md.h.prog.typeDecls[identity] = d
	}
	md.entries = append(md.entries, d)
}

func (md *moduleDecls) loadImports() {
	for _, d := range md.entries {
		if d.use == nil {
			continue
		}
		md.run(d.stmt, func() {
			m, err := md.h.deps.Module(d.use.Module.Name)
			if err != nil {
				d.state = resolutionFailed
				md.h.declareBad(d.stmt)
				md.h.scope.Complete(d.name)
				panic(err)
			}
			d.imported = m
			md.h.module.AddUse(m.Interface())
			if d.use.FromAll {
				md.h.scope.Use(m.Interface(), d.public)
				d.action = func(*Hlc) {}
			}
		})
	}
}

func (md *moduleDecls) resolve() {
	if md.state != resolutionPending {
		return
	}
	md.state = resolutionActive
	for _, d := range md.entries {
		if other := md.h.prog.declarations[d.imported]; other != nil {
			other.resolve()
		}
		md.run(d.stmt, d.resolve)
	}
	// Preserve module-wide BSS defaults, independent of declaration resolution order.
	if bss, ok := md.h.module.Options.Get("bss"); ok {
		for _, def := range md.h.module.Defs {
			if def.Kind == ir.DefBss && def.Segment == "" {
				def.Segment = bss.Str
			}
		}
	}
	md.state = resolutionComplete
}

// completeType requests layout only where it is actually required. A pointer
// breaks the size dependency; arrays require their element's complete layout.
func (h *Hlc) completeType(t *types.Type) {
	if t == nil {
		return
	}
	if d := h.prog.typeDecls[t]; d != nil {
		d.resolve()
	}
	if t.Kind == types.Array && !t.IsSoa {
		h.completeType(t.Base)
	}
	if t.Kind == types.Bad {
		panic(&diag.Error{Suppressed: true})
	}
}

// soaElement resolves just the element identity, without asking for its layout
// or the container length. This permits Node { next:*Nodes; } / soa Nodes:[N]Node.
func (h *Hlc) soaElement(t *types.Type) *types.Type {
	if t.Base != nil {
		return t.Base
	}
	d := h.prog.typeDecls[t]
	if d == nil || d.state == resolutionFailed {
		panic(&diag.Error{Suppressed: true})
	}
	s := d.stmt.(*syntax.SoaDecl)
	owner := &Hlc{prog: h.prog, deps: d.owner.h.deps, module: d.owner.h.module, scope: d.owner.h.scope}
	owner.updatePos(s)
	outer := h.prog.curModule
	h.prog.curModule = owner.module.Id
	defer func() { h.prog.curModule = outer }()
	at, ok := s.Type.(*syntax.ArrayType)
	if ok && at.Len != nil {
		if nt, ok := at.Elem.(*syntax.NamedType); ok {
			elem := owner.namedType(nt)
			if elem.Kind == types.Struct {
				t.Base = elem
				return elem
			}
		}
	}
	panic(&diag.Error{Msg: fmt.Sprintf("soa %s: type must be [N]Struct", s.Name.Name), Pos: owner.curPos})
}
