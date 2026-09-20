package sema

// struct (doc/v2_types_struct.md §4) の意味解析: struct リテラル、フィールド参照、sizeof。
//
// フィールド参照の落とし方:
//   - 変数 (ローカル / グローバル / 一時) の struct: ir.CastedValue{From: 変数, Type: フィールド型, Offset} で
//     直接その場所を指す (実行時のポインタ計算なし。コード生成が sym+off / S+addr+off,x にする)
//   - ポインタ経由 (`p.x` の p が *Struct、`arr[i].x`、`(*p).x`): ポインタ + オフセットを add で計算し、
//     結果を *フィールド型 の左辺値 (pget / pset で読み書き) とする。オフセット 0 なら型ラベルの貼り替えだけ

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// needsExpected は、型名を省いた struct リテラル (`{1, 2}`) を含み、文脈の型がないと評価できない式か。
func (h *Hlc) needsExpected(c *cexpr) bool {
	switch c.kind {
	case cNull:
		return true
	case cStructLit:
		return c.typ == nil && c.ty == nil
	case cArray:
		for _, e := range c.args {
			if h.needsExpected(e) {
				return true
			}
		}
	}
	return false
}

// withExpected supplies the context for null/struct/array literals and constructs
// farfn values from function symbols. A runtime near pointer cannot supply a bank.
// 元の式は変更しない (constEval のメモは cexpr のポインタで引くため、新しいノードを作る)。t が nil ならそのまま。
func (h *Hlc) withExpected(c *cexpr, t *types.Type) *cexpr {
	if t != nil && t.IsFarFunc() && c.kind != cNull {
		x := h.constEval(c)
		if x.kind == cValue && x.val.Kind == ir.KindLiteral && !x.val.IsInt && x.val.Symbol != "" && types.SameFuncSignature(t, x.val.Type) {
			return cv(ir.NewSymbolLiteral("", t, x.val.Symbol))
		}
	}
	if t == nil || (!h.needsExpected(c) && c.kind != cArray) {
		return c
	}
	switch c.kind {
	case cNull:
		return cv(h.nullOf(t).(*ir.Value))
	case cStructLit:
		if t.Kind != types.Struct {
			panic(&diag.Error{Msg: fmt.Sprintf("struct literal cannot be used as %s", t)})
		}
		r := *c
		r.ty = t
		r.flds = make([]cfield, len(c.flds))
		for i, f := range c.flds {
			ft := h.litFieldType(t, f, i)
			r.flds[i] = cfield{key: f.key, val: h.withExpected(f.val, ft)}
		}
		return &r
	case cArray:
		if t.Kind != types.Array {
			return c
		}
		r := *c
		r.args = make([]*cexpr, len(c.args))
		for i, e := range c.args {
			r.args[i] = h.withExpected(e, t.Base)
		}
		return &r
	}
	return c
}

// litFieldType はリテラルの i 番目の項目 (キー付きなら名前で、なければ位置で) に対応するフィールドの型。
func (h *Hlc) litFieldType(st *types.Type, f cfield, i int) *types.Type {
	h.completeType(st)
	if f.key != "" {
		fd, ok := st.Field(f.key)
		if !ok {
			panic(&diag.Error{Msg: fmt.Sprintf("struct %s has no field %s", st.Name, f.key)})
		}
		return fd.Type
	}
	if i >= len(st.Fields) {
		panic(&diag.Error{Msg: fmt.Sprintf("too many values for struct %s (%d fields)", st.Name, len(st.Fields))})
	}
	return st.Fields[i].Type
}

// constEvalStructLit は struct リテラルを評価する。
// 全項目が定数なら定数 (ir.KindArrayLiteral、Elems はフィールド順で省略分は 0)、そうでなければ
// 項目を評価済みにした cStructLit (lval が実行時に組み立てる) を返す。型が決まらなければ未評価のまま返す。
func (h *Hlc) constEvalStructLit(c *cexpr) *cexpr {
	ty := c.ty
	if ty == nil && c.typ != nil {
		ty = h.typeOf(c.typ)
	}
	if ty == nil {
		r := *c
		return &r
	}
	if ty.Kind != types.Struct {
		panic(&diag.Error{Msg: fmt.Sprintf("%s is not a struct", ty)})
	}
	h.completeType(ty)
	if ty.Size < 0 {
		panic(&diag.Error{Msg: fmt.Sprintf("struct %s is not complete yet", ty.Name)})
	}
	// キー付きと位置指定は混ぜない。位置指定は全フィールド分が要る
	keyed := 0
	for _, f := range c.flds {
		if f.key != "" {
			keyed++
		}
	}
	if keyed != 0 && keyed != len(c.flds) {
		panic(&diag.Error{Msg: fmt.Sprintf("struct %s literal mixes named and positional values", ty.Name)})
	}
	if keyed == 0 && len(c.flds) != 0 && len(c.flds) != len(ty.Fields) {
		panic(&diag.Error{Msg: fmt.Sprintf("struct %s has %d fields but %d values given", ty.Name, len(ty.Fields), len(c.flds))})
	}
	// フィールド順に並べ直して評価する
	flds := make([]cfield, 0, len(c.flds))
	seen := map[string]bool{}
	for i, f := range c.flds {
		var fd types.Field
		if f.key != "" {
			var ok bool
			fd, ok = ty.Field(f.key)
			if !ok {
				panic(&diag.Error{Msg: fmt.Sprintf("struct %s has no field %s", ty.Name, f.key)})
			}
			if seen[f.key] {
				panic(&diag.Error{Msg: fmt.Sprintf("field %s given twice", f.key)})
			}
			seen[f.key] = true
		} else {
			fd = ty.Fields[i]
		}
		flds = append(flds, cfield{key: fd.Name, val: h.constEval(h.withExpected(f.val, fd.Type))})
	}
	r := &cexpr{kind: cStructLit, ty: ty, flds: flds}
	// 全部定数なら定数の struct にする
	for _, f := range flds {
		if !isConstLiteral(f.val) {
			return r
		}
	}
	elems := make([]ir.Operand, len(ty.Fields))
	for i, fd := range ty.Fields {
		if fv := structLitField(r, fd.Name); fv != nil {
			h.compatible(fd.Type, fv.val.Type)
			elems[i] = fv.val
		} else {
			elems[i] = h.zeroLiteral(fd.Type)
		}
	}
	return cv(ir.NewArrayLiteral(h.tmpName("$"), ty, elems))
}

// isConstLiteral は評価済みの式がデータとして置ける定数 (整数 / シンボル / 配列・struct リテラル) か。
func isConstLiteral(c *cexpr) bool {
	return c.kind == cValue && (c.val.Kind == ir.KindLiteral || c.val.Kind == ir.KindArrayLiteral)
}

// structLitField は評価済み struct リテラルから名前のフィールドの値を返す (無ければ nil)。
func structLitField(c *cexpr, name string) *cexpr {
	for _, f := range c.flds {
		if f.key == name {
			return f.val
		}
	}
	return nil
}

// zeroLiteral は型 t のゼロ値の定数 (省略したフィールド用)。
func (h *Hlc) zeroLiteral(t *types.Type) *ir.Value {
	switch t.Kind {
	case types.Struct:
		elems := make([]ir.Operand, len(t.Fields))
		for i, f := range t.Fields {
			elems[i] = h.zeroLiteral(f.Type)
		}
		return ir.NewArrayLiteral("", t, elems)
	case types.Array:
		elems := make([]ir.Operand, t.Length)
		for i := range elems {
			elems[i] = h.zeroLiteral(t.Base)
		}
		return ir.NewArrayLiteral("", t, elems)
	}
	return ir.NewIntLiteral("", t, 0)
}

// zeroValue は実行時に代入するゼロ値 (struct / 配列は定数ブロックを経由する)。
func (h *Hlc) zeroValue(t *types.Type) ir.Operand {
	if t.Kind == types.Struct || t.Kind == types.Array {
		return h.rval(cv(h.zeroLiteral(t)))
	}
	return ir.NewIntLiteral("", t, 0)
}

// sizeofType は sizeof(T) の値。T は型式だが、非修飾の名前が変数ならその変数の型のサイズ。
func (h *Hlc) sizeofType(t syntax.TypeExpr) int {
	var ty *types.Type
	if nt, ok := t.(*syntax.NamedType); ok && nt.Module == nil {
		if _, isBasic := h.prog.Types.Named(nt.Name.Name); !isBasic {
			if v := h.scope.Find(nt.Name.Name, true); v != nil && v.TypeRef == nil {
				ty = v.Type
			}
		}
	}
	if ty == nil {
		ty = h.typeEval(t)
	}
	h.completeType(ty)
	if ty.Size < 0 {
		panic(&diag.Error{Msg: fmt.Sprintf("sizeof(%s): size is not known", ty)})
	}
	return ty.Size
}

// fieldRef は `x.name` の評価結果。v / lv は lval と同じ (値, 左辺値か)。
// SoA の 2 バイト以上のフィールドは 1 つの場所で表せないので split で返す (v は nil)。
type fieldRef struct {
	v        ir.Operand
	lv       bool
	split    *soaSplit
	soaConst bool // soa const の要素 (代入不可)
}

// fieldRef は `x.name` (x は struct、struct へのポインタ、または SoA のハンドル)。
func (h *Hlc) fieldRef(arg *cexpr, name string) fieldRef {
	left, lv := h.lval(arg)
	t := ir.ValType(left)
	if lv && t.Kind != types.SoaRef {
		t = t.Base
	}
	switch {
	case t.Kind == types.Pointer && t.Base.Kind == types.Struct:
		// ポインタ経由 (自動で参照はがし)
		ptr := left
		if lv {
			ptr = h.newTmp(t)
			h.emit(&ir.Op{Code: ir.OpPget, Dst: ptr, Src: []ir.Operand{left}})
		}
		return fieldRef{v: h.fieldViaPointer(ptr, t.Base, name), lv: true}
	case t.Kind == types.Struct:
		if lv {
			return fieldRef{v: h.fieldViaPointer(left, t, name), lv: true}
		}
		f := h.fieldOf(t, name)
		return fieldRef{v: ir.NewCastedValue(left, f.Type, f.Offset)}
	case t.Kind == types.SoaRef:
		// ハンドルは左辺値 (要素) でも右辺値 (ハンドルの値) でも同じインデックス
		v, flv, split := h.soaField(left, t, name)
		return fieldRef{v: v, lv: flv, split: split, soaConst: t.Soa.IsConst}
	}
	panic(&diag.Error{Msg: fmt.Sprintf("cannot access field %s: %s is not a struct (type %s)", name, describe(left), t)})
}

func (h *Hlc) fieldOf(st *types.Type, name string) types.Field {
	f, ok := st.Field(name)
	if !ok {
		panic(&diag.Error{Msg: fmt.Sprintf("struct %s has no field %s", st.Name, name)})
	}
	return f
}

// fieldViaPointer は struct へのポインタ ptr からフィールドへのポインタ (左辺値) を作る。
func (h *Hlc) fieldViaPointer(ptr ir.Operand, st *types.Type, name string) ir.Operand {
	f := h.fieldOf(st, name)
	pt := h.prog.Types.PointerTo(f.Type)
	if f.Offset == 0 {
		return ir.NewCastedValue(ptr, pt, 0)
	}
	tmp := h.newTmp(pt)
	h.emit(&ir.Op{Code: ir.OpAdd, Dst: tmp, Src: []ir.Operand{ptr, h.IntValue(f.Offset)}})
	return tmp
}
