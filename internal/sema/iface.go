package sema

// fc 4 の interface (Agent/wiki/design/methods-interface.md)。
//
//	soa interface Task:u8 @(far) {        // ID の型 (u8)。soa は soa に置く interface。@(far) はバンクをまたいで呼ぶ
//	    x:u8; y:u8;                       // 共通のフィールド
//	    function process(self:*Task):void;               // 既定の本体なし
//	    function hp(self:*Task):u8 { return 0; }         // 既定の本体
//	}
//	struct Slime: Task = 1 { dir:u8; }    // 実装 (ID は手動 = N か自動)
//	function Slime.process(self:*Slime):void { ... }
//	soa Tasks:[16]Task;                   // soa の interface の置き場所 (interface と同じモジュールに 1 つ)
//
// 型: interface も実装も struct 型。先頭に ID のフィールド `$id` (型は interface ごとの enum `Task.Id`: none = 0 と実装の名前)、
// 続けて共通のフィールド (ここまでが見出し)、実装はその後ろに自分のフィールドを持つ。普通の interface の struct は見出しと、
// 実装のフィールドを重ねて置く `$data:[K]u8` (K は実装の最大)。
//
// soa の interface: `*Task` は置き場所の soa (Tasks) のハンドル。`*Slime` は「Tasks を Slime として見た」soa (見方。名前は実装の
// struct の修飾名) のハンドルで、見出しのリーフは Tasks の配列、実装のフィールドのリーフは共有の列 Tasks_$v0, Tasks_$v1, ... を
// 指す (実装のフィールドは先頭の列から詰める)。フィールドの読み書きは soa の仕組みそのまま。
//
// 実装の一覧と ID は、全モジュールを読み込んで宣言の解決を始める前に決める (finalizeInterfaces)。メソッドの呼び出し `t.m(...)`
// は interface の struct のメソッド Task.m (振り分けの関数) の呼び出しで、その本体 `TBL[self.$id as u8](self, ...)` と ID で
// 引く関数の表は、関数の本体をコンパイルする前に作る (finishInterfaces)。表の空き (.none・実装の無い番号・実装に無いメソッド) は
// 既定の本体、無ければ何もしない関数 (値を返すメソッドは既定の本体が要る)。

import (
	"fmt"
	"sort"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// idField は interface と実装の先頭の ID のフィールドの名前 (ソースからは書けない)。
const idField = "$id"

// ifaceInfo は interface 1 つ。
type ifaceInfo struct {
	decl    *declaration
	stmt    *syntax.InterfaceDecl
	module  *ir.Module
	t       *types.Type // interface の struct 型 (mod.Task)
	soa     bool
	far     bool
	idType  *types.Type   // ID の enum (mod.Task.Id)
	header  []types.Field // 見出し ($id と共通のフィールド。解決したら)
	hsize   int
	methods []*ifaceMethod
	impls   []*implInfo // ID の順 (finalize したら)
	final   bool

	container *types.Type // soa の interface の置き場所の soa (Tasks)
	columns   []*ir.Value // 共有の列 Tasks_$v0, ... (使う所まで作る)
	h         *Hlc        // interface のモジュールの文脈
	ready     bool        // 振り分けの表と本体を作った (finishInterfaces)
	bss       string      // 置き場所の soa のセグメント
	bankTbl   *ir.Value   // ID → 実装のバンクの番号の表 (@bank_of_id。使わなければ出さない)
}

// ifaceMethod は interface のメソッド 1 つ。
type ifaceMethod struct {
	fd     *syntax.FuncDecl
	params []lambdaParam // 引数 (振り分けの関数・既定の本体・何もしない関数で共通)
	m      *methodDecl   // Task.m (振り分けの関数)
	lmd    *ir.Lambda
	def    *ir.Value // 既定の本体 (無ければ nil)
	none   *ir.Value // 何もしない関数 (作ったら)
}

// implInfo は interface の実装 1 つ。
type implInfo struct {
	t      *types.Type
	decl   *declaration
	stmt   *syntax.StructDecl
	module *ir.Module
	iface  *ifaceInfo
	id     int
	view   *types.Type // soa の interface: 見方の soa
}

// collectInterface は interface の宣言を集める (型の名前は struct と同じく先に束縛する: collectOne。メソッドは interface の型の
// メソッド)。
func (md *moduleDecls) collectInterface(d *declaration, s *syntax.InterfaceDecl) *ifaceInfo {
	p := md.h.prog
	info := &ifaceInfo{decl: d, stmt: s, module: md.h.module, soa: s.Soa.IsValid()}
	d.action = func(h *Hlc) { h.compileInterfaceDecl(info) }
	if p.ifaces == nil {
		p.ifaces = map[*types.Type]*ifaceInfo{}
		p.impls = map[*types.Type]*implInfo{}
	}
	for _, mem := range s.Members {
		fd, ok := mem.(*syntax.FuncDecl)
		if !ok {
			continue
		}
		one := *fd // interface の型 (Recv) のメソッドとして登録する
		one.Recv = s.Name
		one.PublicPos = s.PublicPos
		m := md.addMethod(d, &one)
		info.methods = append(info.methods, &ifaceMethod{fd: fd, m: m})
	}
	return info
}

// compileInterfaceDecl は interface の見出し (ID の型・共通のフィールド) と振り分けの関数の型を決める (宣言の解決)。
func (h *Hlc) compileInterfaceDecl(info *ifaceInfo) {
	h.mustInModule()
	s := info.stmt
	name := s.Name.Name
	info.h = &Hlc{prog: h.prog, deps: h.deps, module: h.module, scope: h.scope}
	u := h.prog.Types
	base := u.IntType(1, false)
	if s.Base != nil {
		base = h.typeEval(s.Base)
		if base.Kind != types.Int || base.Size != 1 || base.Signed || base.Enum != nil {
			panic(&diag.Error{Msg: fmt.Sprintf("interface %s: the id type must be u8 (got %s)", name, base)})
		}
	}
	if s.Options != nil {
		for _, e := range s.Options.Entries {
			if e.Key.Name != "far" {
				h.updatePos(e.Key)
				panic(&diag.Error{Msg: fmt.Sprintf("interface %s: unknown attribute %s (far)", name, e.Key.Name)})
			}
		}
	}
	info.far = parseOptions(s.Options).Flag("far")
	info.idType = u.NewEnum(h.module.Id+"."+name+".Id", base)
	info.idType.Enum.Members = []types.EnumMember{{Name: "none", Value: 0}}
	fields := []types.Field{{Name: idField, Type: info.idType}}
	for _, mem := range s.Members {
		f, ok := mem.(*syntax.FieldDecl)
		if !ok {
			continue
		}
		h.updatePos(f.Name)
		fname := f.Name.Name
		for _, prev := range fields {
			if prev.Name == fname {
				panic(&diag.Error{Msg: fmt.Sprintf("field %s already defined in interface %s", fname, name)})
			}
		}
		if fname == "Id" {
			panic(&diag.Error{Msg: fmt.Sprintf("interface %s: Id is the name of the id type (%s.Id); use another name for the field", name, name)})
		}
		ft := h.fieldType(f.Type, fname)
		fields = append(fields, types.Field{Name: fname, Type: ft})
	}
	off := 0
	for i := range fields {
		fields[i].Offset = off
		off += fields[i].Type.Size
	}
	info.header, info.hsize = fields, off
	if info.soa {
		u.SetFields(info.t, append([]types.Field(nil), fields...))
	}
	// 振り分けの関数 (本体は finishInterfaces が作る) と既定の本体
	for _, im := range info.methods {
		fd := im.fd
		h.updatePos(fd.Name)
		if fd.Name.Name == "Id" {
			panic(&diag.Error{Msg: fmt.Sprintf("interface %s: a method cannot be named Id (%s.Id is the id type)", name, name)})
		}
		for _, f := range fields {
			if f.Name == fd.Name.Name {
				panic(&diag.Error{Msg: fmt.Sprintf("interface %s: method %s has the name of a field", name, fd.Name.Name)})
			}
		}
		if len(fd.Params) == 0 {
			panic(&diag.Error{Msg: fmt.Sprintf("method %s.%s needs the receiver as the first parameter (self:*%s)", name, fd.Name.Name, name)})
		}
		for _, p := range fd.Params {
			if p.Init != nil {
				h.updatePos(p.Init)
				panic(&diag.Error{Msg: fmt.Sprintf("method %s.%s: an interface method cannot have default arguments", name, fd.Name.Name)})
			}
		}
		params := funcParams(fd.Params)
		im.params = params
		if fd.Options != nil {
			panic(&diag.Error{Msg: fmt.Sprintf("method %s.%s: an interface method cannot have attributes", name, fd.Name.Name)})
		}
		mname := name + "." + fd.Name.Name
		sym := methodSym(h.module.Id, name, fd.Name.Name)
		disp := h.constEval(&cexpr{kind: cLambda, pos: fd.Pos(), lam: &lambdaLit{
			name: mname, sym: sym, params: params, result: fd.Result, body: &syntax.Block{Lbrace: fd.Name.NamePos, Rbrace: fd.Name.NamePos},
		}}).val
		if want := h.ifaceSelf(info); disp.Type.Params[0] != want {
			h.updatePos(fd.Params[0].Name)
			panic(&diag.Error{Msg: fmt.Sprintf("method %s: the first parameter is the receiver (self:*%s), not %s", mname, name, disp.Type.Params[0])})
		}
		im.m.val = disp
		im.lmd = h.prog.lambdas[sym]
		if fd.Body != nil {
			im.def = h.constEval(&cexpr{kind: cLambda, pos: fd.Pos(), lam: &lambdaLit{
				name: mname, sym: sym + "__default", params: params, result: fd.Result, body: fd.Body,
			}}).val
		}
	}
}

// ifaceIdType は interface の ID の enum (Task.Id)。
func (h *Hlc) ifaceIdType(info *ifaceInfo) *types.Type {
	if info.idType == nil {
		info.decl.resolve()
	}
	return info.idType
}

// ifaceSelf は interface の受け取り手の型 (soa なら置き場所のハンドル `*Tasks`、普通ならポインタ `*Task`)。
func (h *Hlc) ifaceSelf(info *ifaceInfo) *types.Type {
	if info.soa {
		c := h.ifaceContainer(info)
		return h.prog.Types.SoaRef(c, info.t, "")
	}
	return h.prog.Types.PointerTo(info.t)
}

// ifaceContainer は soa の interface の置き場所の soa (interface のモジュールの `soa S:[N]Task;`)。
func (h *Hlc) ifaceContainer(info *ifaceInfo) *types.Type {
	if info.container != nil {
		return info.container
	}
	if md := h.prog.declarations[info.module]; md != nil {
		for _, d := range md.entries {
			s, ok := d.stmt.(*syntax.SoaDecl)
			if !ok || s.Const {
				continue
			}
			if at, ok := s.Type.(*syntax.ArrayType); ok {
				if nt, ok := at.Elem.(*syntax.NamedType); ok && nt.Module == nil && nt.Name.Name == info.stmt.Name.Name {
					d.resolve()
					if info.container != nil {
						return info.container
					}
				}
			}
		}
	}
	n := info.stmt.Name.Name
	panic(&diag.Error{Msg: fmt.Sprintf("soa interface %s has no soa (declare `soa %ss:[N]%s;` in module %s)", n, n, n, info.module.Id)})
}

// ifaceOfNamed は `struct T: I` の I (interface)。
func (h *Hlc) ifaceOfNamed(nt *syntax.NamedType) *ifaceInfo {
	t := h.namedType(nt)
	if t.Kind == types.Bad {
		panic(&diag.Error{Suppressed: true})
	}
	info := h.prog.ifaces[t]
	if info == nil {
		panic(&diag.Error{Msg: fmt.Sprintf("%s is not an interface", t)})
	}
	if info.header == nil {
		info.decl.resolve()
	}
	return info
}

// compileImplDecl は interface の実装 `struct T: I [= ID] { ... }` の型 (見出し + 自分のフィールド)。
func (h *Hlc) compileImplDecl(s *syntax.StructDecl, st *types.Type) {
	name := s.Name.Name
	impl := &implInfo{t: st, stmt: s, module: h.module, decl: h.prog.typeDecls[st]}
	h.prog.impls[st] = impl // 自分を指すフィールド (`next:*T`) のため先に
	h.updatePos(s.Iface)
	info := h.ifaceOfNamed(s.Iface)
	impl.iface = info
	fields := append([]types.Field(nil), info.header...)
	for _, f := range s.Fields {
		h.updatePos(f.Name)
		fname := f.Name.Name
		for _, prev := range fields {
			if prev.Name == fname {
				if prev.Offset < info.hsize {
					panic(&diag.Error{Msg: fmt.Sprintf("field %s of %s is a field of interface %s", fname, name, info.stmt.Name.Name)})
				}
				panic(&diag.Error{Msg: fmt.Sprintf("field %s already defined in struct %s", fname, name)})
			}
		}
		ft := h.fieldType(f.Type, fname)
		if info.soa && ft.Kind == types.Array {
			panic(&diag.Error{Msg: fmt.Sprintf("field %s: an implementation of soa interface %s cannot have an array field", fname, info.stmt.Name.Name)})
		}
		fields = append(fields, types.Field{Name: fname, Type: ft})
	}
	h.prog.Types.SetFields(st, fields)
}

// implOf は t が interface の実装ならその情報 (宣言を解決する)。
func (h *Hlc) implOf(t *types.Type) *implInfo {
	if t == nil || t.Kind != types.Struct || h.prog.impls == nil {
		return nil
	}
	if impl := h.prog.impls[t]; impl != nil {
		return impl
	}
	if d := h.prog.typeDecls[t]; d != nil {
		if s, ok := d.stmt.(*syntax.StructDecl); ok && s.Iface != nil && d.state != resolutionActive {
			d.resolve()
			return h.prog.impls[t]
		}
	}
	return nil
}

// ifaceHandle は `*T` (T が soa の interface か、その実装) のハンドルの型 (そうでなければ nil)。
func (h *Hlc) ifaceHandle(t *types.Type) *types.Type {
	if h.prog.ifaces == nil || t.Kind != types.Struct {
		return nil
	}
	if info := h.prog.ifaces[t]; info != nil {
		if !info.soa {
			return nil
		}
		return h.prog.Types.SoaRef(h.ifaceContainer(info), t, "")
	}
	if impl := h.implOf(t); impl != nil && impl.iface.soa {
		return h.prog.Types.SoaRef(h.implView(impl), t, "")
	}
	return nil
}

// implView は soa の interface の実装 impl の見方の soa (リーフは soaOf が要るときに作る: buildView)。
func (h *Hlc) implView(impl *implInfo) *types.Type {
	if impl.view != nil {
		return impl.view
	}
	c := h.ifaceContainer(impl.iface)
	v := h.prog.Types.SoaArray(impl.t.Name, impl.t, c.Length, false)
	if v.Base == nil || v.Length < 0 {
		v.Base, v.Length = impl.t, c.Length
	}
	impl.view = v
	if h.prog.soaViews == nil {
		h.prog.soaViews = map[*types.Type]*implInfo{}
	}
	h.prog.soaViews[v] = impl
	return v
}

// buildView は見方の soa のリーフ (見出しは置き場所の配列、実装のフィールドは共有の列)。
func (h *Hlc) buildView(v *types.Type, impl *implInfo) *soaInfo {
	info := impl.iface
	if impl.t.Size < 0 {
		impl.decl.resolve()
	}
	ch := h.soaOf(info.container)
	leaves := h.soaLeaves(impl.t, shortName(v.Name)+"_", 0, nil)
	for i := range leaves {
		lf := &leaves[i]
		if lf.offset < info.hsize {
			for _, cl := range ch.leaves {
				if cl.offset == lf.offset {
					lf.arr = cl.arr
				}
			}
			continue
		}
		lf.arr = h.ifaceColumn(info, lf.offset-info.hsize)
	}
	si := &soaInfo{leaves: leaves}
	h.prog.soas[v] = si
	return si
}

// ifaceColumn は soa の interface の共有の列 k (Tasks_$v<k>。無ければ置き場所のモジュールに作る)。
func (h *Hlc) ifaceColumn(info *ifaceInfo, k int) *ir.Value {
	for len(info.columns) <= k {
		n := len(info.columns)
		c := info.container
		name := fmt.Sprintf("%s_$v%d", shortName(c.Name), n)
		at := h.prog.Types.ArrayOf(h.prog.Types.IntType(1, false), c.Length)
		sym := info.h.addDef(name, &ir.Def{Kind: ir.DefBss, Type: at, Segment: info.bss})
		info.columns = append(info.columns, ir.NewGlobal(name, at, sym))
	}
	return info.columns[k]
}

// finalizeInterfaces は全モジュールを読み込んだ後 (宣言の解決の前) に、interface ごとに実装の一覧と ID を決める。
func (p *Program) finalizeInterfaces() {
	if len(p.ifaces) == 0 {
		return
	}
	var infos []*ifaceInfo
	var implDecls []*declaration
	for _, m := range p.Modules.List() {
		md := p.declarations[m]
		if md == nil {
			continue
		}
		for _, d := range md.entries {
			switch s := d.stmt.(type) {
			case *syntax.InterfaceDecl:
				if d.identity != nil {
					infos = append(infos, p.ifaces[d.identity.Type])
				}
			case *syntax.StructDecl:
				if s.Iface != nil {
					implDecls = append(implDecls, d)
				}
			}
		}
	}
	for _, info := range infos {
		info.decl.owner.run(info.stmt, info.decl.resolve)
	}
	for _, d := range implDecls {
		d.owner.run(d.stmt, d.resolve)
	}
	for _, info := range infos {
		if info.header != nil {
			info.decl.owner.run(info.stmt, func() { info.h.finalize(info) })
		}
	}
}

// finalize は interface info の実装の一覧と ID、ID の enum のメンバー、普通の interface の大きさを決める。
func (h *Hlc) finalize(info *ifaceInfo) {
	info.final = true
	var impls []*implInfo
	for _, m := range h.prog.modulesByPath() {
		for _, impl := range h.prog.implsByModule(m) {
			if impl.iface == info {
				impls = append(impls, impl)
			}
		}
	}
	name := info.stmt.Name.Name
	used := map[int]*implInfo{}
	names := map[string]*implInfo{}
	for _, impl := range impls {
		n := impl.stmt.Name.Name
		if n == "none" {
			panic(&diag.Error{Msg: fmt.Sprintf("an implementation of %s cannot be named none (%s.Id.none is the id 0)", name, name), Pos: syntax.At(impl.module.Path, impl.stmt.Name.NamePos)})
		}
		if prev := names[n]; prev != nil {
			panic(&diag.Error{Msg: fmt.Sprintf("%s and %s.%s both implement %s with the name %s (the names are the members of %s.Id)", prev.module.Id+"."+n, impl.module.Id, n, name, n, name), Pos: syntax.At(impl.module.Path, impl.stmt.Name.NamePos)})
		}
		names[n] = impl
		if impl.stmt.ID == nil {
			continue
		}
		ih := &Hlc{prog: h.prog, deps: impl.decl.owner.h.deps, module: impl.module, scope: impl.decl.owner.h.scope}
		ih.updatePos(impl.stmt.ID)
		e := ih.constEval(toC(impl.stmt.ID))
		if !e.isLiteralInt() {
			panic(&diag.Error{Msg: fmt.Sprintf("the id of %s must be a constant integer", n), Pos: ih.curPos})
		}
		id := e.val.Int
		switch {
		case id == 0:
			panic(&diag.Error{Msg: fmt.Sprintf("the id of %s cannot be 0 (%s.Id.none)", n, name), Pos: ih.curPos})
		case id < 0 || id > 255:
			panic(&diag.Error{Msg: fmt.Sprintf("the id of %s must be 1..255 (got %d)", n, id), Pos: ih.curPos})
		case used[id] != nil:
			panic(&diag.Error{Msg: fmt.Sprintf("%s and %s have the same id %d", used[id].stmt.Name.Name, n, id), Pos: ih.curPos})
		}
		impl.id = id
		used[id] = impl
	}
	next := 1
	for _, impl := range impls { // 自動: モジュールの順 (読み込んだ順でなくパス) と宣言の順
		if impl.stmt.ID != nil {
			continue
		}
		for used[next] != nil {
			next++
		}
		if next > 255 {
			panic(&diag.Error{Msg: fmt.Sprintf("interface %s has more than 255 implementations", name), Pos: syntax.At(impl.module.Path, impl.stmt.Name.NamePos)})
		}
		impl.id = next
		used[next] = impl
	}
	sort.Slice(impls, func(i, j int) bool { return impls[i].id < impls[j].id })
	info.impls = impls
	members := []types.EnumMember{{Name: "none", Value: 0}}
	for _, impl := range impls {
		members = append(members, types.EnumMember{Name: impl.stmt.Name.Name, Value: impl.id})
	}
	info.idType.Enum.Members = members
	if !info.soa {
		k := 0
		for _, impl := range impls {
			k = max(k, impl.t.Size-info.hsize)
		}
		fields := append([]types.Field(nil), info.header...)
		if k > 0 {
			fields = append(fields, types.Field{Name: "$data", Type: h.prog.Types.ArrayOf(h.prog.Types.IntType(1, false), k)})
		}
		h.prog.Types.SetFields(info.t, fields)
	}
}

// implsByModule はモジュール m の実装 (宣言の順。自動の ID はモジュールのパスの順に振るので、呼ぶ側がモジュールを並べる)。
func (p *Program) implsByModule(m *ir.Module) []*implInfo {
	md := p.declarations[m]
	if md == nil {
		return nil
	}
	var r []*implInfo
	for _, d := range md.entries {
		if s, ok := d.stmt.(*syntax.StructDecl); ok && s.Iface != nil && d.identity != nil {
			if impl := p.impls[d.identity.Type]; impl != nil && impl.iface != nil {
				r = append(r, impl)
			}
		}
	}
	return r
}

// modulesByPath はモジュールをパスの順に並べる (自動の ID の順)。
func (p *Program) modulesByPath() []*ir.Module {
	ms := append([]*ir.Module(nil), p.Modules.List()...)
	sort.SliceStable(ms, func(i, j int) bool { return ms[i].Path < ms[j].Path })
	return ms
}

// finishInterfaces は振り分けの関数の本体と ID で引く関数の表を作る (関数の本体をコンパイルする前)。
func (p *Program) finishInterfaces() {
	for _, m := range p.Modules.List() {
		md := p.declarations[m]
		if md == nil {
			continue
		}
		for _, d := range md.entries {
			s, ok := d.stmt.(*syntax.InterfaceDecl)
			if !ok || d.identity == nil || d.state != resolutionComplete {
				continue
			}
			info := p.ifaces[d.identity.Type]
			if info.ready || !info.final {
				continue
			}
			info.ready = true
			md.run(s, func() { info.h.buildDispatch(info) })
		}
	}
}

// buildDispatch は interface の各メソッドの表と振り分けの関数の本体を作る。
func (h *Hlc) buildDispatch(info *ifaceInfo) {
	for _, im := range info.methods {
		// 作れなかったメソッドの振り分けの関数は本体なし (巻き添えのエラーを出さない)。作れたら本体を入れる
		delete(h.prog.bodies, im.lmd)
		im.lmd.Extern = true
	}
	name := info.stmt.Name.Name
	n := 1
	for _, impl := range info.impls {
		n = max(n, impl.id+1)
		h.module.AddUse(impl.module.Id) // 表から実装の関数を参照する (.inc の .import。use はしていない)
	}
	// ID → 実装のバンク (@bank_of_id): 実装が書いた最初のメソッドの関数のバンク。実装の無い ID と、メソッドを 1 つも書いていない
	// 実装は最初のメソッドの表の要素 (既定の本体か何もしない関数) のバンク。表の要素で決めると、最初のメソッドを書かずに既定の
	// 本体を使う実装が固定の所のバンクになっていた (fuzz の TestRandomFarIfaceNES)
	banks := make([]ir.Operand, n)
	var first []ir.Operand
	var firstT *types.Type
	u8 := h.prog.Types.IntType(1, false)
	bankOf := func(e ir.Operand, dispT *types.Type) ir.Operand {
		farT := h.prog.Types.FarFunc(dispT.Params, dispT.Base)
		return ir.NewCastedValue(ir.NewSymbolLiteral("", farT, ir.ValLiteral(e).Symbol), u8, 2)
	}
	for _, im := range info.methods {
		fd := im.fd
		mname := name + "." + fd.Name.Name
		dispT := im.m.val.Type
		elemT := dispT
		if info.far {
			elemT = h.prog.Types.FarFunc(dispT.Params, dispT.Base)
		}
		hole := func(what string) ir.Operand {
			if im.def != nil {
				return ir.NewSymbolLiteral("", elemT, im.def.Symbol)
			}
			if im.none == nil {
				// 既定の本体が無ければ、何もしない (値を返すメソッドは 0・null・すべて 0 の struct・長さ 0 の slice を返す) 関数
				body := &syntax.Block{Lbrace: fd.Name.NamePos, Rbrace: fd.Name.NamePos}
				z := zeroExpr(dispT.Base, fd.Result, fd.Name.NamePos)
				if z == nil && (dispT.Base.Kind == types.Struct || dispT.Base.Kind == types.Array) {
					// slice (長さ 0) と配列: 0 の値を隠した名前で置いて返す (構文の木では書けない)。slice は起動のときに 0 の RAM の
					// 変数 (書き換えられる slice に ROM の定数を渡すと警告になる)、配列は定数
					zname := "$zero_" + im.lmd.Id
					if dispT.Base.IsSlice() {
						h.addVar(ir.NewGlobal(zname, dispT.Base, h.addDef(zname, &ir.Def{Kind: ir.DefBss, Type: dispT.Base})))
					} else {
						zv := h.zeroLiteral(dispT.Base)
						zv.Name = zname
						h.scope.Declare(zv)
					}
					z = &syntax.Ident{NamePos: fd.Name.NamePos, Name: zname}
				}
				if z != nil {
					body.Stmts = []syntax.Stmt{&syntax.ReturnStmt{Return: fd.Name.NamePos, Value: z, Semi: fd.Name.NamePos}}
				} else if dispT.Base.Kind != types.Void {
					panic(&diag.Error{Msg: fmt.Sprintf("method %s returns %s: write its default body in interface %s (it runs for %s)", mname, dispT.Base, name, what), Pos: syntax.At(info.module.Path, fd.Name.NamePos)})
				}
				im.none = h.constEval(&cexpr{kind: cLambda, pos: fd.Pos(), lam: &lambdaLit{
					name: mname, sym: im.lmd.Id + "__none", params: im.params, result: fd.Result, body: body,
				}}).val
			}
			return ir.NewSymbolLiteral("", elemT, im.none.Symbol)
		}
		elems := make([]ir.Operand, n)
		for _, impl := range info.impls {
			m := h.prog.methods[impl.t.Name][fd.Name.Name]
			if m == nil {
				elems[impl.id] = hole(impl.stmt.Name.Name + ", which does not have " + fd.Name.Name)
				continue
			}
			m.decl.resolve()
			mt := m.val.Type
			ok := len(mt.Params) == len(dispT.Params) && mt.Base == dispT.Base && mt.Fastcall() == dispT.Fastcall()
			for i := 1; ok && i < len(mt.Params); i++ {
				ok = mt.Params[i] == dispT.Params[i]
			}
			self := mt.Params[0]
			if ok {
				ok = self.Kind == types.SoaRef && self.Base == impl.t || self.Kind == types.Pointer && self.Base == impl.t && !self.ReadOnly
			}
			if !ok {
				want := h.prog.Types.Func(append([]*types.Type{h.implSelf(impl)}, dispT.Params[1:]...), dispT.Base, false)
				panic(&diag.Error{Msg: fmt.Sprintf("method %s does not match %s: want %s, got %s", m.name(), mname, want, mt), Pos: syntax.At(m.module.Path, m.fd.Name.NamePos)})
			}
			elems[impl.id] = ir.NewSymbolLiteral("", elemT, m.val.Symbol)
			if banks[impl.id] == nil {
				banks[impl.id] = bankOf(elems[impl.id], dispT)
			}
		}
		for i := range elems {
			if elems[i] == nil {
				what := fmt.Sprintf("id %d, which has no implementation", i)
				if i == 0 {
					what = name + ".Id.none"
				}
				elems[i] = hole(what)
			}
		}
		if first == nil {
			first, firstT = elems, dispT
		}
		at := h.prog.Types.ArrayOf(elemT, n)
		tblName := fmt.Sprintf("$%s_%s", name, fd.Name.Name)
		d := &ir.Def{Kind: ir.DefBlock, Type: at, Elems: elems, Droppable: true}
		sym := h.addDef(tblName, d)
		g := ir.NewGlobal(tblName, at, sym)
		g.ReadOnly = true
		h.prog.constArrays[g] = ir.NewArrayLiteral("", at, elems)
		h.scope.Declare(g)
		h.prog.bodies[im.lmd] = dispatchBody(fd, tblName)
		im.lmd.Extern = false
	}
	if first != nil {
		for i, b := range banks {
			if b == nil {
				banks[i] = bankOf(first[i], firstT)
			}
		}
		at := h.prog.Types.ArrayOf(u8, n)
		tblName := "$" + name + "_bank"
		sym := h.addDef(tblName, &ir.Def{Kind: ir.DefBlock, Type: at, Elems: banks, Droppable: true})
		info.bankTbl = ir.NewGlobal(tblName, at, sym)
		info.bankTbl.ReadOnly = true
	}
}

// implSelf は実装 impl のメソッドの受け取り手の型 (soa なら見方のハンドル、普通ならポインタ)。
func (h *Hlc) implSelf(impl *implInfo) *types.Type {
	if impl.iface.soa {
		return h.prog.Types.SoaRef(h.implView(impl), impl.t, "")
	}
	return h.prog.Types.PointerTo(impl.t)
}

// dispatchBody は振り分けの関数の本体 `TBL[self.$id as u8](self, a, ...)` (値を返すなら return)。
func dispatchBody(fd *syntax.FuncDecl, tbl string) *syntax.Block {
	pos := fd.Name.NamePos
	id := func(name string) *syntax.Ident { return &syntax.Ident{NamePos: pos, Name: name} }
	self := fd.Params[0].Name.Name
	idx := &syntax.CastExpr{Kind: syntax.CastAs, As: pos,
		X:    &syntax.BinaryExpr{X: id(self), OpPos: pos, Op: syntax.Dot, Y: id(idField)},
		Type: &syntax.NamedType{Name: id("u8")}}
	args := make([]syntax.Expr, len(fd.Params))
	for i, p := range fd.Params {
		args[i] = id(p.Name.Name)
	}
	call := &syntax.CallExpr{Fun: &syntax.IndexExpr{X: id(tbl), Lbrack: pos, Index: idx, Rbrack: pos}, Lparen: pos, Args: args, Rparen: pos}
	var st syntax.Stmt = &syntax.ReturnStmt{Return: pos, Value: call, Semi: pos}
	if nt, ok := fd.Result.(*syntax.NamedType); ok && nt.Module == nil && nt.Name.Name == "void" {
		st = &syntax.ExprStmt{X: call, Semi: pos}
	}
	return &syntax.Block{Lbrace: pos, Stmts: []syntax.Stmt{st}, Rbrace: pos}
}

// ifaceOfType は t (struct の値・ポインタ・ハンドル) が指す interface か実装の interface (無ければ nil)。
func (h *Hlc) ifaceOfType(t *types.Type) *ifaceInfo {
	switch t.Kind {
	case types.Pointer, types.SoaRef:
		t = t.Base
	}
	if t == nil || t.Kind != types.Struct || h.prog.ifaces == nil {
		return nil
	}
	if info := h.prog.ifaces[t]; info != nil {
		return info
	}
	if impl := h.implOf(t); impl != nil {
		return impl.iface
	}
	return nil
}

// ifaceIdArg は @id_of / @set_id の要素の引数 e を検査し、その interface を返す。
func (h *Hlc) ifaceIdArg(e *cexpr, what string) *ifaceInfo {
	info, ok := h.exprType(e)
	if ok && !info.untyped {
		if i := h.ifaceOfType(info.t); i != nil {
			return i
		}
	}
	panic(&diag.Error{Msg: fmt.Sprintf("%s takes an element of an interface (*Task, &Tasks[i], *Slime, ...)", what)})
}

func registerIfaceBuiltins(h *Hlc) {
	// @id_of(e): 要素の ID (interface の Id の enum)
	h.defmacroTyped("@id_of", pureMacro, func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		if len(args) != 1 {
			panic(&diag.Error{Msg: "@id_of takes 1 argument (@id_of(&Tasks[i]))"})
		}
		h.ifaceIdArg(args[0], "@id_of")
		return macroResult{expr: &cexpr{kind: cOp, op: opField, args: []*cexpr{args[0]}, name: idField, pos: args[0].pos}}
	})
	// @bank_of_id(id): ID の実装のメソッドを置いたバンクの番号 (near の interface でバンクを自分で切り替えるとき)
	h.defmacroTyped("@bank_of_id", macroTyping{typ: func(h *Hlc, args []*cexpr) (exprInfo, bool) {
		return exprInfo{t: h.prog.Types.IntType(1, false)}, true
	}}, func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		if len(args) != 1 {
			panic(&diag.Error{Msg: "@bank_of_id takes 1 argument (an id: @bank_of_id(@id_of(e)))"})
		}
		a, ok := h.exprType(args[0])
		var info *ifaceInfo
		if ok && a.t.Enum != nil {
			for _, i := range h.prog.ifaces {
				if i.idType == a.t {
					info = i
				}
			}
		}
		if info == nil {
			panic(&diag.Error{Msg: "@bank_of_id takes the id of an interface (Task.Id)"})
		}
		if info.bankTbl == nil {
			panic(&diag.Error{Msg: fmt.Sprintf("@bank_of_id: interface %s has no method", info.stmt.Name.Name)})
		}
		idx := &cexpr{kind: cCast, args: []*cexpr{args[0]}, ty: h.prog.Types.IntType(1, false), ck: syntax.CastAs, pos: args[0].pos}
		return macroResult{expr: &cexpr{kind: cOperand, opnd: h.rval(cop2(opIndex, cv(info.bankTbl), idx))}}
	})
	// @set_id(e, id): 要素の ID を書く (ほかのフィールドには触らない)
	h.defmacroTyped("@set_id", voidMacro, func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		if len(args) != 2 {
			panic(&diag.Error{Msg: "@set_id takes 2 arguments (@set_id(&Tasks[i], .Slime))"})
		}
		info := h.ifaceIdArg(args[0], "@set_id")
		dst := &cexpr{kind: cOp, op: opField, args: []*cexpr{args[0]}, name: idField, pos: args[0].pos}
		return macroResult{stmts: []*cexpr{cop2(opLoad, dst, h.withExpected(args[1], info.idType))}}
	})
}

// soaInterfaceDecl は `soa S:[N]Task;` の Task が interface なら検査して置き場所として登録する (compileSoaDecl)。
func (h *Hlc) soaInterfaceDecl(s *syntax.SoaDecl, soa, elem *types.Type, seg string) {
	if h.prog.ifaces == nil {
		return
	}
	if impl := h.prog.impls[elem]; impl != nil {
		panic(&diag.Error{Msg: fmt.Sprintf("soa %s: %s is an implementation of %s; the soa holds the interface (soa %s:[N]%s)", s.Name.Name, shortName(elem.Name), impl.iface.stmt.Name.Name, s.Name.Name, impl.iface.stmt.Name.Name)})
	}
	info := h.prog.ifaces[elem]
	if info == nil {
		return
	}
	n := info.stmt.Name.Name
	switch {
	case !info.soa:
		panic(&diag.Error{Msg: fmt.Sprintf("soa %s: interface %s is not a soa interface (declare it `soa interface %s`)", s.Name.Name, n, n)})
	case s.Const:
		panic(&diag.Error{Msg: fmt.Sprintf("soa const %s: a soa interface cannot be const", s.Name.Name)})
	case h.module != info.module:
		panic(&diag.Error{Msg: fmt.Sprintf("soa %s: the soa of interface %s must be declared in module %s", s.Name.Name, n, info.module.Id)})
	case info.container != nil && info.container != soa:
		panic(&diag.Error{Msg: fmt.Sprintf("soa %s: interface %s already has the soa %s (a soa interface has one soa)", s.Name.Name, n, shortName(info.container.Name))})
	}
	info.container, info.bss = soa, seg
}

// ifaceElemCopy は soa の interface の要素を値として写す (gather。scatter なら store) のを弾く。実装として見た要素へ実装の値を
// 書くのはよい (`Tasks[i] = Slime{...}`: ID も書く)。
func (h *Hlc) ifaceElemCopy(t *types.Type, store bool) {
	i := h.ifaceOfType(t)
	if i == nil || !i.soa || store && h.prog.ifaces[t.Base] == nil {
		return
	}
	panic(&diag.Error{Msg: fmt.Sprintf("an element of soa interface %s cannot be copied as a value (read and write its fields)", i.stmt.Name.Name)})
}

// zeroExpr は型 t (書いた型 te) の 0 の式 (整数・enum・bool は `0 as T`、ポインタ・関数は null、struct は `{}`。void や
// それ以外は nil)。
func zeroExpr(t *types.Type, te syntax.TypeExpr, pos syntax.Pos) syntax.Expr {
	switch {
	case t.Kind == types.Int || t.Kind == types.Bool:
		return &syntax.CastExpr{Kind: syntax.CastAs, X: &syntax.IntLit{ValuePos: pos, Value: 0, Text: "0"}, As: pos, Type: te}
	case t.Kind == types.Pointer || t.Kind == types.Func:
		return &syntax.NullLit{ValuePos: pos}
	case t.Kind == types.Struct && !t.IsSlice():
		return &syntax.StructLit{Lbrace: pos, Rbrace: pos}
	}
	return nil
}

// litDefault は struct のリテラルで省いたフィールド fd の値 (interface の実装の ID はその実装の ID、ほかは 0)。
func (h *Hlc) litDefault(st *types.Type, fd types.Field) *ir.Value {
	if fd.Name == idField {
		if impl := h.implOf(st); impl != nil {
			return ir.NewIntLiteral("", fd.Type, impl.id)
		}
	}
	return h.zeroLiteral(fd.Type)
}

// implAssign は代入 args[0] = args[1] (評価済み) の右辺が interface の実装の struct の値 (リテラル `Slime{...}` など) で、左辺が
// その interface の要素なら、左辺を実装として見た代入 `*@bitcast(*Slime, &Tasks[i]) = Slime{...}` (ID も書く) にする (そうで
// なければ nil)。
func (h *Hlc) implAssign(args []*cexpr, pos syntax.Pos) *cexpr {
	if len(h.prog.impls) == 0 || len(args) != 2 {
		return nil
	}
	r, ok := h.exprType(args[1])
	if !ok || r.untyped {
		return nil
	}
	impl := h.implOf(r.t)
	if impl == nil {
		return nil
	}
	l, ok := h.exprType(args[0])
	if !ok || !(l.t.Kind == types.SoaRef && l.t.Base == impl.iface.t || l.t == impl.iface.t) {
		return nil
	}
	ref := &cexpr{kind: cCast, args: []*cexpr{cop2(opRef, args[0])}, ty: h.implSelf(impl), ck: syntax.CastBit, pos: pos}
	return &cexpr{kind: cOp, op: opLoad, args: []*cexpr{h.constEval(cop2(opDeref, ref)), args[1]}, pos: pos}
}

// InterfaceSummary は interface ごとの実装と ID の一覧 (fcc build -d)。
func (p *Program) InterfaceSummary() []string {
	var r []string
	for _, m := range p.Modules.List() {
		md := p.declarations[m]
		if md == nil {
			continue
		}
		for _, d := range md.entries {
			if _, ok := d.stmt.(*syntax.InterfaceDecl); !ok || d.identity == nil {
				continue
			}
			info := p.ifaces[d.identity.Type]
			if !info.final {
				continue
			}
			var kind []string
			if info.soa && info.container != nil {
				kind = append(kind, "soa "+shortName(info.container.Name))
			}
			if info.far {
				kind = append(kind, "far")
			}
			line := "  " + info.t.Name
			if len(kind) > 0 {
				line += " (" + strings.Join(kind, ", ") + ")"
			}
			line += ": none = 0"
			for _, impl := range info.impls {
				line += fmt.Sprintf(", %s = %d (%s)", impl.stmt.Name.Name, impl.id, impl.module.Id)
			}
			r = append(r, line)
		}
	}
	return r
}
