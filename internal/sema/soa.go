package sema

// SoA コンテナ `soa` (doc/v2_types_struct.md §4.5)。
//
//	soa Points:[4]Point;            // フィールドごとの配列 Points_x, Points_y, ... (2 バイト以上のフィールドはバイトごと)
//	var p:*Points;                  // 要素ハンドル (types.SoaRef): 実体は 1 バイトのインデックス
//	p.x = 1;                        // index Points_x, p → pset (最適化で index_pset: ldy p; sta Points_x,y)
//	Points[2].y = 5;                // 添字はハンドルを作るだけ
//	var pt:Point = *p; *p = pt;     // gather / scatter (リーフごとに index + pget/pset)
//	soa const T:[N]Point = [...];   // 初期値を転置して定数ブロックに
//
// ハンドルの「左辺値」(lval の leftValue が真) は要素そのもの (rval で gather、代入で scatter)、
// 「右辺値」はハンドルの値 (インデックス)。`&Points[i]` は左辺値からハンドルを取り出し、`*p` はその逆。

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// soaLeaf は SoA コンテナの 1 本の配列 (struct の平坦化した 1 バイト分)。
type soaLeaf struct {
	name   string    // Points_x, Points_v_0, Points_pos_x
	offset int       // 要素 (struct 値) の中のバイト位置
	arr    *ir.Value // [N]T のグローバル配列 (T は 1 バイトのフィールド型、分割したバイトなら uint8)
}

// soaInfo はコンテナごとの情報 (Program.soas)。
type soaInfo struct {
	leaves []soaLeaf
}

// soaSplit は 2 バイト以上のフィールド (バイトごとの配列に分かれている) への参照。
// 1 つのポインタでは表せないので、読み出しは soaGatherSplit、代入は soaStoreSplit で扱う。
type soaSplit struct {
	ref    ir.Operand // ハンドル (インデックス)
	leaves []soaLeaf  // バイト順
	typ    *types.Type
}

// soaLeaves は struct をリーフに平坦化する。配列フィールドは禁止 (S5)。
func (h *Hlc) soaLeaves(st *types.Type, prefix string, base int, leaves []soaLeaf) []soaLeaf {
	for _, f := range st.Fields {
		switch f.Type.Kind {
		case types.Struct:
			leaves = h.soaLeaves(f.Type, prefix+f.Name+"_", base+f.Offset, leaves)
		case types.Array:
			panic(&diag.Error{Msg: fmt.Sprintf("soa: array field %s is not allowed (struct %s)", f.Name, st.Name)})
		default:
			if f.Type.Size == 1 {
				leaves = append(leaves, soaLeaf{name: prefix + f.Name, offset: base + f.Offset})
			} else {
				for i := 0; i < f.Type.Size; i++ {
					leaves = append(leaves, soaLeaf{name: fmt.Sprintf("%s%s_%d", prefix, f.Name, i), offset: base + f.Offset + i})
				}
			}
		}
	}
	return leaves
}

// compileSoaDecl は `soa Name:[N]Struct [options];` / `soa const Name:[N]Struct = [...] [options];`。
// 名前はコンテナの値 (添字でハンドルを作る) であり、同時に型名 (`*Name` の Name) でもある。
func (h *Hlc) compileSoaDecl(s *syntax.SoaDecl) {
	h.mustInModule()
	name := s.Name.Name
	at, ok := s.Type.(*syntax.ArrayType)
	if !ok || at.Len == nil {
		panic(&diag.Error{Msg: fmt.Sprintf("soa %s: type must be [N]Struct", name)})
	}
	arrType := h.typeEval(at)
	elem := arrType.Base
	if elem.Kind != types.Struct || arrType.Length < 0 {
		panic(&diag.Error{Msg: fmt.Sprintf("soa %s: type must be [N]Struct", name)})
	}
	h.checkComplete(elem, "soa "+name)
	n := arrType.Length
	if n < 1 || n > 256 {
		panic(&diag.Error{Msg: fmt.Sprintf("soa %s: length must be 1..256 (indexed by the Y register)", name)})
	}
	opt := parseOptions(s.Options)
	seg := h.groupBss
	if sv, ok := opt.Get("segment"); ok {
		seg = sv.Text()
		if seg == "" {
			seg = "BSS"
		} // explicit legacy default overrides inherited bss
	}

	soa := h.prog.Types.SoaArray(h.module.Id+"."+name, elem, n, s.Const)
	if _, dup := h.prog.soas[soa]; dup {
		panic(&diag.Error{Msg: fmt.Sprintf("soa %s already defined", name)})
	}
	leaves := h.soaLeaves(elem, name+"_", 0, nil)

	// const なら初期値 (struct の配列) を転置する
	var init *ir.Value
	if s.Const {
		c := h.constEval(h.withExpected(toC(s.Init), arrType))
		if c.kind != cValue || c.val.Kind != ir.KindArrayLiteral {
			panic(&diag.Error{Msg: fmt.Sprintf("soa const %s: initializer must be a constant array of %s", name, elem)})
		}
		if len(c.val.Elems) != n {
			panic(&diag.Error{Msg: fmt.Sprintf("soa const %s has %d elements but %d given", name, n, len(c.val.Elems))})
		}
		init = c.val
	}

	uint8T := h.prog.Types.IntType(1, false)
	for i := range leaves {
		lf := &leaves[i]
		lt := h.soaLeafType(elem, lf.offset)
		if lt == nil {
			lt = uint8T
		}
		at := h.prog.Types.ArrayOf(lt, n)
		var sym string
		if init != nil {
			elems := make([]ir.Operand, n)
			for j, e := range init.Elems {
				elems[j] = h.soaByteOf(elem, e, lf.offset)
			}
			sym = h.addDef(lf.name, &ir.Def{Kind: ir.DefBlock, Type: at, Elems: elems})
		} else {
			sym = h.addDef(lf.name, &ir.Def{Kind: ir.DefBss, Type: at, Segment: seg})
		}
		lf.arr = ir.NewGlobal(lf.name, at, sym)
	}
	h.prog.soas[soa] = &soaInfo{leaves: leaves}

	// コンテナの値 (型名も兼ねる)。シンボルは持たない (リーフの配列だけがメモリ上にある)
	var v *ir.Value
	if h.prog.typeDecls[soa] != nil {
		v = h.prog.typeDecls[soa].identity
		h.module.Vars = append(h.module.Vars, v)
	} else {
		v = h.addVar(ir.NewTypeValue(name, soa, soa))
	}
	if h.scopeIsPublic(s.PublicPos) {
		v.Public = true
	}
}

// soaLeafType は要素 struct のバイト位置 off にある 1 バイトのフィールドの型 (2 バイト以上のフィールドの一部なら nil)。
func (h *Hlc) soaLeafType(st *types.Type, off int) *types.Type {
	for _, f := range st.Fields {
		if off < f.Offset || off >= f.Offset+f.Type.Size {
			continue
		}
		if f.Type.Kind == types.Struct {
			return h.soaLeafType(f.Type, off-f.Offset)
		}
		if f.Type.Size == 1 {
			return f.Type
		}
		return nil
	}
	return nil
}

// soaByteOf は定数 struct 値 v (KindArrayLiteral) のバイト位置 off の 1 バイトを定数として返す。
func (h *Hlc) soaByteOf(st *types.Type, v ir.Operand, off int) ir.Operand {
	lv := ir.ValLiteral(v)
	if lv == nil || lv.Kind != ir.KindArrayLiteral {
		panic(&diag.Error{Msg: fmt.Sprintf("soa const: invalid constant for struct %s", st.Name)})
	}
	uint8T := h.prog.Types.IntType(1, false)
	for i, f := range st.Fields {
		if off < f.Offset || off >= f.Offset+f.Type.Size {
			continue
		}
		e := lv.Elems[i]
		if f.Type.Kind == types.Struct {
			return h.soaByteOf(f.Type, e, off-f.Offset)
		}
		el := ir.ValLiteral(e)
		byteNo := off - f.Offset
		// Retain the function symbol for Entry analysis and the bank range assertion.
		if f.Type.IsFarFunc() {
			return ir.NewCastedValue(e, uint8T, byteNo)
		}
		switch {
		case el != nil && el.Kind == ir.KindLiteral && el.IsInt:
			return ir.NewIntLiteral("", uint8T, ir.FloorMod(ir.Shr(el.Int, byteNo*8), 256))
		case el != nil && el.Kind == ir.KindLiteral && byteNo <= 1:
			// 関数などのシンボル: .LOBYTE / .HIBYTE で 1 バイトずつ
			if byteNo == 0 {
				return ir.NewSymbolLiteral("", uint8T, ".LOBYTE("+el.Symbol+")")
			}
			return ir.NewSymbolLiteral("", uint8T, ".HIBYTE("+el.Symbol+")")
		}
		panic(&diag.Error{Msg: fmt.Sprintf("soa const: field %s must be an integer or symbol constant", f.Name)})
	}
	panic("soaByteOf: offset out of range")
}

// soaOf はハンドル型 / コンテナ型からコンテナ情報を返す。
func (h *Hlc) soaOf(t *types.Type) *soaInfo {
	if t.Kind == types.SoaRef {
		t = t.Soa
	}
	if d := h.prog.typeDecls[t]; d != nil && d.state == resolutionFailed {
		panic(&diag.Error{Suppressed: true})
	}
	info, ok := h.prog.soas[t]
	if !ok {
		panic(fmt.Sprintf("soa %s is not registered", t))
	}
	return info
}

// soaLeavesUnder はハンドル型 t (Path で入れ子を表す) が指す struct のリーフと、その struct の先頭バイト位置を返す。
func (h *Hlc) soaLeavesUnder(t *types.Type) ([]soaLeaf, int) {
	info := h.soaOf(t)
	base := t.Soa.Name[len(t.Soa.Name)-len(shortName(t.Soa.Name)):] + "_" + t.Path
	var r []soaLeaf
	first := -1
	for _, lf := range info.leaves {
		if len(lf.name) >= len(base) && lf.name[:len(base)] == base {
			if first < 0 || lf.offset < first {
				first = lf.offset
			}
			r = append(r, lf)
		}
	}
	return r, first
}

// shortName はモジュール修飾名 (mod.Name) の Name 部分。
func shortName(qual string) string {
	for i := len(qual) - 1; i >= 0; i-- {
		if qual[i] == '.' {
			return qual[i+1:]
		}
	}
	return qual
}

// soaRetype はハンドル値 v の型を t に貼り替える (リテラルは型付きリテラルに)。
func soaRetype(v ir.Operand, t *types.Type) ir.Operand {
	if n, ok := ir.ValIntLiteral(v); ok {
		return ir.NewIntLiteral("", t, n)
	}
	return ir.NewCastedValue(v, t, 0)
}

// soaIndex は `Points[i]`: 添字からハンドルを作る (コードは出ない。1 バイトの添字のみ)。
func (h *Hlc) soaIndex(soa *types.Type, idx ir.Operand) ir.Operand {
	if ir.ValType(idx).Kind != types.Int {
		panic(&diag.Error{Msg: fmt.Sprintf("index must be an integer (got %s)", ir.ValType(idx))})
	}
	if _, isLit := ir.ValIntLiteral(idx); !isLit && ir.ValType(idx).Size != 1 {
		panic(&diag.Error{Msg: fmt.Sprintf("soa %s: index must be 1 byte", shortName(soa.Name))})
	}
	return soaRetype(idx, h.prog.Types.SoaRef(soa, soa.Base, ""))
}

// soaLeafPtr はハンドル ref のリーフ lf へのポインタ (index)。
func (h *Hlc) soaLeafPtr(ref ir.Operand, lf soaLeaf, elemType *types.Type) ir.Operand {
	tmp := h.newTmp(h.prog.Types.PointerTo(elemType))
	h.emit(&ir.Op{Code: ir.OpIndex, Dst: tmp, Src: []ir.Operand{lf.arr, ref}})
	return tmp
}

// soaField は `p.name` (p はハンドル)。
//   - 入れ子の struct フィールド: Path を伸ばしたハンドルを要素 (左辺値) として返す (コードなし)
//   - 1 バイトのフィールド: リーフへのポインタ (左辺値)
//   - 2 バイト以上のフィールド: soaSplit (呼び出し側が gather / store する)
func (h *Hlc) soaField(ref ir.Operand, t *types.Type, name string) (v ir.Operand, lv bool, split *soaSplit) {
	f := h.fieldOf(t.Base, name)
	if f.Type.Kind == types.Struct {
		// 入れ子の struct は要素 (左辺値: 読めば gather、代入は scatter)。`&p.pos` でハンドルになる
		return soaRetype(ref, h.prog.Types.SoaRef(t.Soa, f.Type, t.Path+name+"_")), true, nil
	}
	leaves, base := h.soaLeavesUnder(t)
	var mine []soaLeaf
	for _, lf := range leaves {
		if lf.offset >= base+f.Offset && lf.offset < base+f.Offset+f.Type.Size {
			mine = append(mine, lf)
		}
	}
	if len(mine) != f.Type.Size {
		panic(fmt.Sprintf("soa: leaves of %s.%s not found", t, name))
	}
	if f.Type.Size == 1 {
		return h.soaLeafPtr(ref, mine[0], f.Type), true, nil
	}
	return nil, false, &soaSplit{ref: ref, leaves: mine, typ: f.Type}
}

// soaGatherSplit は 2 バイト以上のフィールドを一時変数に集める。
func (h *Hlc) soaGatherSplit(sp *soaSplit) ir.Operand {
	tmp := h.newTmp(sp.typ)
	uint8T := h.prog.Types.IntType(1, false)
	for i, lf := range sp.leaves {
		p := h.soaLeafPtr(sp.ref, lf, uint8T)
		h.emit(&ir.Op{Code: ir.OpPget, Dst: ir.NewCastedValue(tmp, uint8T, i), Src: []ir.Operand{p}})
	}
	return tmp
}

// soaStoreSplit は 2 バイト以上のフィールドへの代入 (バイトごとに pset)。
func (h *Hlc) soaStoreSplit(sp *soaSplit, v ir.Operand) {
	h.soaCheckWritable(sp.ref)
	h.compatible(sp.typ, ir.ValType(v))
	v = h.cast(v, sp.typ)
	uint8T := h.prog.Types.IntType(1, false)
	byteOf := func(i int) ir.Operand {
		if n, ok := ir.ValIntLiteral(v); ok {
			return ir.NewIntLiteral("", uint8T, ir.FloorMod(ir.Shr(n, i*8), 256))
		}
		return ir.NewCastedValue(v, uint8T, i)
	}
	if _, isLit := ir.ValIntLiteral(v); !isLit {
		if _, plain := v.(*ir.Value); !plain {
			v = h.operandValue(v) // 配列→ポインタ変換やサイズの違うキャストはバイト単位で切り出せないので一時変数に写す
		}
	}
	for i, lf := range sp.leaves {
		p := h.soaLeafPtr(sp.ref, lf, uint8T)
		h.emit(&ir.Op{Code: ir.OpPset, Src: []ir.Operand{p, byteOf(i)}})
	}
}

// soaGather は `*p` の読み出し: 要素全体を struct の一時変数に集める。
func (h *Hlc) soaGather(ref ir.Operand) ir.Operand {
	t := ir.ValType(ref)
	tmp := h.newTmp(t.Base)
	leaves, base := h.soaLeavesUnder(t)
	uint8T := h.prog.Types.IntType(1, false)
	for _, lf := range leaves {
		p := h.soaLeafPtr(ref, lf, uint8T)
		h.emit(&ir.Op{Code: ir.OpPget, Dst: ir.NewCastedValue(tmp, uint8T, lf.offset-base), Src: []ir.Operand{p}})
	}
	return tmp
}

// soaScatter は `*p = s` の代入: struct の値 s をリーフごとに書く。
func (h *Hlc) soaScatter(ref ir.Operand, src ir.Operand) {
	h.soaCheckWritable(ref)
	t := ir.ValType(ref)
	h.compatible(t.Base, ir.ValType(src))
	if _, plain := src.(*ir.Value); !plain {
		src = h.operandValue(src)
	}
	leaves, base := h.soaLeavesUnder(t)
	uint8T := h.prog.Types.IntType(1, false)
	for _, lf := range leaves {
		p := h.soaLeafPtr(ref, lf, uint8T)
		h.emit(&ir.Op{Code: ir.OpPset, Src: []ir.Operand{p, ir.NewCastedValue(src, uint8T, lf.offset-base)}})
	}
}

// soaCheckWritable は soa const への書き込みを弾く。
func (h *Hlc) soaCheckWritable(ref ir.Operand) {
	if t := ir.ValType(ref); t.Soa.IsConst {
		panic(&diag.Error{Msg: fmt.Sprintf("cannot assign to element of soa const %s", shortName(t.Soa.Name))})
	}
}
