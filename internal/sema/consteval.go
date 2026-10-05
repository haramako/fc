package sema

// 定数式の評価 (cexpr の部分評価) と、型付き整数の畳み込みの規則。

import (
	"fmt"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// constEval は const_eval 相当。未評価の cexpr を受け、評価済みの cexpr を返す
// (値が確定すれば cValue、そうでなければ子が評価済みの演算ノード)。入力は変異しない。
// 評価済みの木に再適用しても結果は変わらない。
func (h *Hlc) constEval(c *cexpr) *cexpr {
	if c.kind == cValue || c.kind == cOperand {
		return c
	}
	if h.cmemo == nil {
		h.cmemo = map[*cexpr]*cexpr{}
	}
	if r, ok := h.cmemo[c]; ok {
		return r
	}
	defer h.enterExpr(c.pos)()
	r := h.constEval0(c)
	r.pos, r.end = c.pos, c.end // 位置 (エラー報告と fc 4 への書き換え: rewrite.go) は評価前の式のもの
	if c.compound != nil {
		r.compound = c.compound
	}
	h.cmemo[c] = r
	return r
}

// constEvalOperand は評価結果を IR のオペランド (*ir.Value) として取り出す。定数でなければ CompileError。
func (h *Hlc) constEvalOperand(c *cexpr) ir.Operand {
	r := h.constEval(c)
	if r.kind == cName {
		panic(r.sym.notValue())
	}
	if r.kind != cValue {
		panic(&diag.Error{Msg: "constant value required (got an expression that is evaluated at runtime)"})
	}
	if h.prog.storageAliases[r.val] != nil {
		panic(&diag.Error{Msg: "cannot read a storage alias at compile time"})
	}
	return r.val
}

func (h *Hlc) constEval0(c *cexpr) *cexpr {
	switch c.kind {

	case cInt:
		if c.s == "bool" {
			return cv(ir.NewIntLiteral("", h.prog.Types.Bool(), c.n))
		}
		if c.s == "char" && c.n > 0x7f && !strings.HasPrefix(c.name, "'\\") {
			panic(&diag.Error{Msg: fmt.Sprintf("%s is not an ASCII character; convert it with a textmap converter (_T(%s)) or write the code ('\\xNN')", c.name, c.name)})
		}
		return cv(h.IntValue(c.n))

	case cNull:
		return &cexpr{kind: cNull} // 型が決まるまで保留 (withExpected)

	case cNullFn:
		return c // @null_fn (型が決まるまで保留)

	case cName:
		return c // 値でない名前 (評価済み)

	case cIdent:
		if h.caseDecls[c.name] && h.scope.Find(c.name, true) == nil {
			panic(&diag.Error{Msg: fmt.Sprintf("%s not found (in fc 3 a variable declared in a switch case is visible only in that case; declare it before the switch)", c.name)})
		}
		return h.symExpr(h.scope.FindMust(c.name, true))

	case cStr:
		// fc 3 までは String#unpack('c*') と同じ符号付きバイト (128 以上のバイトがあると [N]i8 になる)。fc 4 は符号なしのバイトで、
		// 文字列はいつも [N]u8 (`"a\xff"` が [2]i8 になって []const u8 にできず、for-each で負の値になっていた)
		elems := make([]*cexpr, 0, len(c.s)+1)
		for i := 0; i < len(c.s); i++ {
			if h.v4() {
				elems = append(elems, cint(int(c.s[i])))
			} else {
				elems = append(elems, cint(int(int8(c.s[i]))))
			}
		}
		term := !h.v4() // fc 3 までは終端の 0 を足す (長さには含めない)。fc 4 の文字列は 0 終端にしない
		if term {
			elems = append(elems, cint(0))
		}
		var rv *ir.Value
		if len(elems) == 0 {
			rv = ir.NewArrayLiteral(h.tmpName("$"), h.prog.Types.ArrayOf(h.prog.Types.IntType(1, false), 0), nil) // fc 4 の ""
		} else {
			rv = h.constEval(carray(elems)).val
		}
		rv.IsString, rv.StrTerm = true, term
		rv.Str = c.s
		h.noteStrLit(rv, c)
		return cv(rv)

	case cArray:
		if c.ty == nil && h.v4() && stringRows(c.args) {
			// fc 4: 型を書かない文字列の配列は slice の表 (`[?][]const u8`)。長さがそろうかどうかで型が変わらない (2026-09-30)
			st := h.prog.Types.Slice(h.prog.Types.IntType(1, false), true, false)
			return h.constEval(h.withExpected(&cexpr{kind: cArray, args: c.args, pos: c.pos, end: c.end}, h.prog.Types.ArrayOf(st, -1)))
		}
		vals := make([]ir.Operand, len(c.args))
		var typ *types.Type
		for i, e := range c.args {
			if h.needsExpected(e) {
				// 型名を省いた struct リテラルを含む配列: 宣言の型が与えられるまで評価を保留する
				return &cexpr{kind: cArray, args: c.args, ty: c.ty}
			}
			x := h.constEval(h.constSlice(e)) // slice の要素 (`[?][]const u8 = ["ab", "cde"]`) は定数の slice に
			// `[ORIGIN, {1, 2}]`: 名前付きの const の struct・配列は中身の値 (型が無ければ要素そのものの型で)
			if c.ty != nil && c.ty.Kind == types.Array {
				x = h.namedConstValue(x, c.ty.Base)
				if c.ty.Base.Kind == types.Pointer {
					x = h.constRefAddress(x, c.ty.Base) // `[&gp, &g[1]]`: グローバル変数のアドレス
				}
			} else if c.ty == nil && x.kind == cValue {
				x = h.namedConstValue(x, x.val.Type)
			} else if c.ty == nil {
				x = h.constRefAddress(x, nil)
			}
			if !isConstElem(x) || h.prog.storageAliases[x.val] != nil {
				// 実行時の値 (変数・式) を要素に持つ: 実行時に一時変数へ組み立てる (lval)。変数の名前がそのアドレスの定数に
				// なっていた (`var a:[2]u8 = [n, m]` が n と m のアドレスの表。const のポインタの表の規則が効いていた)
				return &cexpr{kind: cArray, args: c.args, ty: c.ty, rt: true, pos: c.pos}
			}
			v := x.val
			if t := c.ty; t != nil && t.Kind == types.Array && t.Base.Kind == types.Array && t.Base.Length > len(v.Elems) && v.IsString {
				// `const T:[2][3]u8 = ["ab", "cd"]`: 文字列の行は行の長さまで 0 で詰める (fc 4 の文字列には終端の 0 が無い)
				v = h.padArrayLiteral(v.Name, v, t.Base)
			}
			vals[i] = v
			if i == 0 {
				typ = ir.ValType(v)
			} else {
				typ = h.compatible(typ, ir.ValType(v))
			}
		}
		if typ == nil {
			panic(&diag.Error{Msg: "cannot infer the element type of an empty array literal"})
		}
		if c.ty == nil || c.ty.Kind != types.Array {
			typ = h.arrayElemType(typ, vals, c.args, true) // F4 (fc 4): 定数の要素の値を変えない型に (宣言の型があればそちらで作り直す)
		}
		return cv(ir.NewArrayLiteral(h.tmpName("$"), h.prog.Types.ArrayOf(typ, len(vals)), vals))

	case cIncbin:
		data := h.readFile(c.s)
		// unpack('C*') は符号なしバイト
		elems := make([]*cexpr, len(data))
		for i, b := range data {
			elems[i] = cint(int(b))
		}
		return h.constEval(carray(elems))

	case cLambda:
		lam := c.lam
		params := make([]ir.Param, len(lam.params))
		for i, p := range lam.params {
			params[i] = ir.Param{Name: p.name, Type: h.typeOf(p.typ)}
		}
		baseType := h.typeOf(lam.result)
		var id string
		if sym, ok := symbolOption(lam.options); ok {
			id = sym
		} else if lam.name == "main" {
			id = "_main"
		} else if lam.name != "" {
			id = fmt.Sprintf("_%s_%s", h.module.Id, lam.name)
		} else {
			// 無名関数。連番がモジュール単位になったので、リンク時の衝突を避けるためモジュール名で修飾する
			id = fmt.Sprintf("_%s_%s", h.module.Id, h.tmpName("$"))
		}
		lmd := h.newLambda(id, lam.name, params, baseType, lam.options, lam.body)
		lmd.Pos = syntax.At(h.module.Path, c.pos)
		h.module.Lambdas = append(h.module.Lambdas, lmd)
		h.addDefModule(&ir.Def{Sym: id, Kind: ir.DefCode, Type: lmd.Type, Lambda: lmd})
		h.prog.lambdas[id] = lmd
		h.registerDefaults(lmd, lam.params)
		return cv(ir.NewSymbolLiteral("", lmd.Type, id))

	case cEnumShort:
		return c // 型は文脈から (withExpected)。決まらないまま値として使ったら rval がエラーにする

	case cDot:
		left := h.constEval(c.args[0])
		if left.kind == cValue && left.val.Type.Kind == types.Bad {
			panic(&diag.Error{Suppressed: true})
		}
		if ref := left.typeOf(); ref != nil && ref.Enum != nil {
			return cv(h.enumMember(ref, c.name)) // enum のメンバー (Type.Name)
		}
		if left.moduleOf() != "" {
			return h.symExpr(h.prog.boundModule(left.sym).LookupMust(c.name))
		}
		// モジュールでなければ struct のフィールド参照 (実行時に評価する)
		return h.constField(left, c.name)

	case cStructLit:
		return h.constEvalStructLit(c)

	case cSizeof:
		return cv(h.IntValue(h.sizeofType(c.typ)))

	case cCast:
		x := h.constEval(c.args[0])
		ty := c.ty // 評価済みノードの再評価では型を再計算しない
		if ty == nil {
			ty = h.typeEval(c.typ)
		}
		if x.kind == cNull {
			return cv(h.nullOf(ty).(*ir.Value)) // `null as *T`
		}
		if x.isLiteralInt() {
			if ty.IsFarFunc() && !x.val.Type.IsFarFunc() {
				panic(&diag.Error{Msg: "cannot construct farfn from an integer; assign a function symbol or null"})
			}
			if x.val.Type.IsFarFunc() {
				h.checkCast(c.ck, x.val.Type, ty)
			}
			// 整数リテラルはサイズを持たないので、bitcast のサイズ検査はしない (`bitcast<*int>(0x2000)`, `bitcast<fn():int>(0)`)
			if c.ck == syntax.CastAs {
				h.checkCast(c.ck, x.val.Type, ty)
			}
			n := x.val.Int
			if (ty.Kind == types.Int || ty.Kind == types.Bool) && ty.Size > 0 && ty.Size < 8 {
				// 数値変換・ビットの読み替え: 型の幅に切り詰めて、その符号で読む (`(300 as int) as int16` は 44、
				// `@bitcast(i8, 254 as u8)` は -2。畳まない変数の cast と同じ。
				// 以前は値をそのまま型だけ貼り替えていて、広げ直すと 300 のままだった)
				bits := 8 * ty.Size
				n = ir.FloorMod(n, 1<<bits)
				if ty.Signed && n >= 1<<(bits-1) {
					n -= 1 << bits
				}
			}
			return cv(ir.NewIntLiteral("", ty, n))
		}
		return &cexpr{kind: cCast, args: []*cexpr{x}, typ: c.typ, ty: ty, ck: c.ck, pos: c.pos}

	case cOp:
		switch c.op {
		case opAddWrap, opSubWrap, opMulWrap:
			if a, b := h.constEval(c.args[0]), h.constEval(c.args[1]); a.kind != cValue || b.kind != cValue {
				return c // 実行時の値がある: lval が wrapExpr にする
			}
			return h.constEval(h.wrapExpr(c, c.args[0], c.args[1]))
		case opAdd, opSub, opMul, opDiv, opMod,
			opEq, opNe, opLt, opGt, opLe, opGe,
			opAnd, opOr, opXor, opLand, opLor, opNot, opUminus, opBitNot,
			opShiftLeft, opShiftRight:
			args := make([]*cexpr, len(c.args))
			src := c.args
			if len(src) == 2 {
				a, b := h.resolveEnumShortPair(src[0], src[1]) // `x == .A`
				src = []*cexpr{a, b}
			}
			args[0] = h.constEval(src[0])
			if len(c.args) > 1 {
				args[1] = h.constEval(src[1])
			}
			if args[0].kind == cValue && (len(args) == 1 || args[1].kind == cValue) {
				var bt *types.Type
				if len(args) > 1 {
					bt = args[1].val.Type
				}
				checkEnumOp(c.op, args[0].val.Type, bt)
			}
			for _, arg := range args {
				if arg.kind != cValue || !arg.val.Type.IsFarFunc() {
					continue
				}
				switch c.op {
				case opEq, opNe:
					if args[0].isLiteralInt() && args[1].isLiteralInt() {
						h.compatible(args[0].val.Type, args[1].val.Type)
					}
				case opLand, opLor, opNot:
				case opLt, opGt, opLe, opGe:
					panic(&diag.Error{Msg: "ordered comparison is not supported on farfn"})
				default:
					panic(&diag.Error{Msg: "arithmetic is not supported on farfn"})
				}
			}
			if args[0].isLiteralInt() && (len(args) == 1 || args[1].isLiteralInt()) {
				v1 := args[0].val.Int
				v2 := 0
				if len(args) > 1 {
					v2 = args[1].val.Int
				}
				// 型付きの定数は、実行時と同じく両方を演算の型 (互換型) の値にしてから計算し、結果も型の幅で折り返す
				// (`(0 as u8) + 242` と `(-5 as i8)` の剰余は i8 の -14 % -5。畳み込みが 242 のまま計算して実行時と違っていた。
				// TestRandomConstFold で発覚)。シフトの量はそのまま
				t := h.foldType(args, false)
				if c.op == opShiftLeft || c.op == opShiftRight {
					// F1: fc 4 のシフトの結果は左辺の型 (型のない左辺なら型のない値)。fc 3 は両辺で決めるので、違えば
					// migrate に左辺を今の型の `as` にする書き換えを報告する (`32 << (6 as u8)` は fc 3 では u8 の 0)
					tl := h.foldType(args[:1], false)
					switch {
					case h.v4():
						t = tl
					case h.rewriting() && t != nil && t != tl && len(c.args) == 2:
						h.rewriteAs("shift-type", c.args[0], t.String())
					}
				}
				// F6 の符号の混ざった大小の比較は、A1 で広げる前の型で見る (実行時も広げる前に検査する。`i16 >= (255 as u8) << 3` の
				// 右は広げると u16 の 2040 になるが、元は u8 で i16 に収まる。TestRandomConstFoldV4 の種 700002)
				mixed := len(args) > 1 && mixedSign(args[0].val, args[1].val)
				if t != nil && c.op != opLand && c.op != opLor && c.op != opNot {
					// A1 (widen.go): 広い型と出会う、印の付いた (折り返した) オペランドは、fc 4 では元の式をその幅で畳み込み直す
					// (fc 3 のモジュールでは migrate に `as` を報告する)
					if h.foldWidenArgs(c, args, t) {
						v1 = args[0].val.Int
						if len(args) > 1 {
							v2 = args[1].val.Int
						}
					}
				}
				switch {
				case t == nil || c.op == opLand || c.op == opLor || c.op == opNot:
				case c.op == opEq || c.op == opNe || c.op == opLt || c.op == opGt || c.op == opLe || c.op == opGe:
					// 比較: 型付きの定数だけを互換型の値にする。型のない定数は実行時も相手に合わせる / 広げる / 16 ビットを
					// 超えれば数学の値で比べる (foldBeyond16) ので、そのまま
					// 片方が型のない定数でも、16 ビットに収まるなら両方を互換型の値にする (実行時は型のない定数を相手に合わせるか
					// 広げた型で比べる: `65533 > ((-81 as i8) >> 9)` は i16 で -3 > -1。数学の値で比べて 1 になっていた。
					// TestRandomConstFold)。16 ビットを超える型のない定数は実行時も数学の値で比べる (foldBeyond16)
					if len(args) > 1 {
						u0, u1 := args[0].val.Untyped, args[1].val.Untyped
						switch {
						case !u0 && !u1:
							if c.op != opEq && c.op != opNe && mixed {
								// F6: 型付きの定数どうしの大小の比較も実行時と同じ
								switch {
								case h.v4():
									panic(h.mixedSignError(args[0].val.Type, args[1].val.Type))
								case h.rewriting():
									h.rewriteMixedSign(args[0].val, args[1].val, [2]*cexpr{c.args[0], c.args[1]})
								}
							}
							v1, v2 = wrapInt(v1, t), wrapInt(v2, t)
						case u0 != u1:
							// 型付きの側が符号付きなら、実行時はそれを符号拡張して、定数が i16 に収まれば符号付き (数学の値) で、
							// 収まらなければ u16 で比べる (`(-5 as i8) < 271` は真、`65533 > (-1 as i8)` は 65533 > 65535 で偽)。
							// 符号なしなら互換型 (広いほう) の値で比べる
							typed, uv := args[0], &v2
							tv := &v1
							if u0 {
								typed, uv, tv = args[1], &v1, &v2
							}
							if *uv >= -32768 && *uv <= 65535 { // 16 ビットを超える型のない定数は数学の値のまま (foldBeyond16)
								if tt := typed.val.Type; tt.Kind == types.Int && tt.Signed {
									*tv = wrapInt(*tv, tt)
									if *uv >= 32768 {
										*tv, *uv = wrapInt(*tv, t), wrapInt(*uv, t)
									}
								} else {
									*tv, *uv = wrapInt(*tv, t), wrapInt(*uv, t)
								}
							}
						}
					}
				default:
					v1 = wrapInt(v1, t)
					if len(args) > 1 && c.op != opShiftLeft && c.op != opShiftRight {
						v2 = wrapInt(v2, t)
					}
				}
				n := foldIntOp(c.op, v1, v2)
				switch c.op {
				case opEq, opNe, opLt, opGt, opLe, opGe, opLand, opLor, opNot:
					return cv(ir.NewIntLiteral("", h.prog.Types.Bool(), n)) // 比較・論理演算の定数畳み込みも bool
				}
				if t != nil {
					w := wrapInt(n, t) // 型付きの定数は、変数と同じく型の幅で折り返す
					if h.foldOverflows(c, args, t, n != w) {
						lit := ir.NewIntLiteral("", t, w)
						h.markTaint(lit, c)
						return cv(lit)
					}
					return cv(ir.NewIntLiteral("", t, w))
				}
				return cv(h.IntValue(n))
			}
			return &cexpr{kind: cOp, op: c.op, args: args}

		case opCall:
			args := make([]*cexpr, len(c.args))
			for i, a := range c.args {
				if i > 0 && a.kind == cInt && a.s == "char" && args[0].textmapOf() != nil {
					args[i] = a // _T('あ'): 文字のリテラルのまま変換器に渡す (ASCII 以外の文字は評価するとエラー)
					continue
				}
				args[i] = h.constEval(a)
			}
			// 定数式で評価する組み込み (textmap など) はここで展開する
			if m := args[0].macroOf(); m != nil && m.constFn != nil {
				return m.constFn(h, args[1:])
			}
			return &cexpr{kind: cOp, op: opCall, args: args, block: c.block}

		case opField:
			return h.constField(h.constEval(c.args[0]), c.name)

		case opCond:
			return h.constEvalCond(c)

		case opMin, opMax, opClamp:
			args := make([]*cexpr, len(c.args))
			allLit := true
			for i, a := range c.args {
				args[i] = h.constEval(a)
				allLit = allLit && args[i].isLiteralInt()
			}
			if allLit {
				// 型付きの定数があれば、実行時と同じく互換型の値にして比べる (数学の値で比べていた)。型は比較の規則
				t := h.foldType(args, true)
				val := func(i int) int {
					if t != nil {
						return wrapInt(args[i].val.Int, t)
					}
					return args[i].val.Int
				}
				v := val(0)
				switch c.op {
				case opMin:
					v = min(v, val(1))
				case opMax:
					v = max(v, val(1))
				case opClamp:
					v = min(max(v, val(1)), val(2))
				}
				if t != nil {
					return cv(ir.NewIntLiteral("", t, v))
				}
				return cv(h.IntValue(v))
			}
			return &cexpr{kind: cOp, op: c.op, args: args}

		case opLoad, opIndex, opRef, opDeref, opSlice, opToSlice, opLen:
			if c.op == opRef && h.constIndex {
				// `&C[1]` は要素の値ではなく場所 (constRefAddress)
				h.constIndex = false
				defer func() { h.constIndex = true }()
			}
			args := make([]*cexpr, len(c.args))
			for i, a := range c.args {
				if a != nil { // opSlice の省いた lo / hi
					args[i] = h.constEval(a)
				}
			}
			if c.op == opIndex && h.constIndex {
				if e := h.constElem(args[0], args[1]); e != nil {
					return e
				}
			}
			return &cexpr{kind: cOp, op: c.op, args: args, ty: c.ty, incl: c.incl}
		}
	}
	panic(fmt.Sprintf("invalid op %v", c.op))
}

// constElem は定数の配列 (配列リテラル・文字列・名前付きの配列定数) a の定数の添字 i の要素 (整数の要素でなければ nil)。
func (h *Hlc) constElem(a, i *cexpr) *cexpr {
	if a == nil || i == nil || !i.isLiteralInt() {
		return nil
	}
	arr := h.constAggregate(a)
	if arr == nil || arr.Type.Kind != types.Array {
		return nil
	}
	n := i.val.Int
	if n < 0 || n >= len(arr.Elems) {
		panic(&diag.Error{Msg: fmt.Sprintf("index %d is out of the constant array (length %d)", n, len(arr.Elems))})
	}
	if e := ir.ValLiteral(arr.Elems[n]); e != nil && e.Kind == ir.KindLiteral && e.IsInt {
		return cv(e)
	}
	return nil
}

// constField は評価済みの left のフィールド name の参照。const の宣言の中 (constIndex) で、定数の表の整数のフィールド
// (`MONS[1].hp`、`ORIGIN.x`) なら定数にする。
func (h *Hlc) constField(left *cexpr, name string) *cexpr {
	if h.constIndex {
		if v := h.constAggregate(left); v != nil && v.Type.Kind == types.Struct && !v.Type.IsSlice() {
			for i, f := range v.Type.Fields {
				if f.Name == name && i < len(v.Elems) {
					if e := ir.ValLiteral(v.Elems[i]); e != nil && e.Kind == ir.KindLiteral && e.IsInt {
						return cv(e)
					}
				}
			}
		}
	}
	return &cexpr{kind: cOp, op: opField, args: []*cexpr{left}, name: name}
}

// constAggregate は評価済みの式 c が定数の配列・struct (リテラル、名前付きの配列・struct の定数、その定数の添字の
// 要素・フィールド: `TBL[1]`、`MONS[1].pos`) ならその中身 (ir.KindArrayLiteral)。そうでなければ nil。
func (h *Hlc) constAggregate(c *cexpr) *ir.Value {
	switch {
	case c == nil:
	case c.kind == cValue:
		v := c.val
		if lit, ok := h.prog.constArrays[v]; ok {
			v = lit
		}
		if v.Kind == ir.KindArrayLiteral {
			return v
		}
	case c.kind == cOp && c.op == opIndex && len(c.args) == 2 && c.args[1] != nil && c.args[1].isLiteralInt():
		arr := h.constAggregate(c.args[0])
		if arr == nil || arr.Type.Kind != types.Array {
			return nil
		}
		if n := c.args[1].val.Int; n >= 0 && n < len(arr.Elems) {
			if e := ir.ValLiteral(arr.Elems[n]); e != nil && e.Kind == ir.KindArrayLiteral {
				return e
			}
		}
	case c.kind == cOp && c.op == opField:
		st := h.constAggregate(c.args[0])
		if st == nil || st.Type.Kind != types.Struct || st.Type.IsSlice() {
			return nil
		}
		for i, f := range st.Type.Fields {
			if f.Name == c.name && i < len(st.Elems) {
				if e := ir.ValLiteral(st.Elems[i]); e != nil && e.Kind == ir.KindArrayLiteral {
					return e
				}
			}
		}
	}
	return nil
}

// IntValue は値から型を推定した整数リテラル: 0〜255 → uint8、256 以上 → uint16、-128〜-1 → sint8、-129 以下 → sint16。
func (h *Hlc) IntValue(n int) *ir.Value {
	var t *types.Type
	switch {
	case n >= 256:
		t = h.prog.Types.IntType(2, false)
	case n < -128:
		t = h.prog.Types.IntType(2, true)
	case n < 0:
		t = h.prog.Types.IntType(1, true)
	default:
		t = h.prog.Types.IntType(1, false)
	}
	v := ir.NewIntLiteral("", t, n)
	v.Untyped = true
	return v
}

// newLambda は ir.Lambda を作る。型は params / baseType / options(fastcall) から決まる。
func (h *Hlc) newLambda(id, name string, params []ir.Param, baseType *types.Type, opts ir.Options, body *syntax.Block) *ir.Lambda {
	argTypes := make([]*types.Type, len(params))
	for i, p := range params {
		argTypes[i] = p.Type
	}
	h.checkABI(name, opts, body == nil)
	typ := h.prog.Types.Func(argTypes, baseType, opts.Flag("fastcall"))
	lmd := &ir.Lambda{Id: id, Name: name, Params: params, Type: typ, Options: opts, Module: h.module, Extern: body == nil}
	if body != nil {
		h.prog.bodies[lmd] = body
	}
	return lmd
}

// foldIntOp は整数リテラル同士の演算を畳み込む (除算・剰余は床除算、比較・論理演算の結果は 0/1。実行時の演算と同じ規則)。
func foldIntOp(op cop, v1, v2 int) int {
	b2i := func(b bool) int {
		if b {
			return 1
		}
		return 0
	}
	switch op {
	case opAdd:
		return v1 + v2
	case opSub:
		return v1 - v2
	case opMul:
		return v1 * v2
	case opDiv, opMod:
		if v2 == 0 {
			// 定数同士の 0 除算 (`1/0`) は畳み込みで Go の panic になっていた (fuzz で発覚)
			panic(&diag.Error{Msg: "div by 0"})
		}
		if op == opDiv {
			return ir.FloorDiv(v1, v2)
		}
		return ir.FloorMod(v1, v2)
	case opEq:
		return b2i(v1 == v2)
	case opNe:
		return b2i(v1 != v2)
	case opLt:
		return b2i(v1 < v2)
	case opGt:
		return b2i(v1 > v2)
	case opLe:
		return b2i(v1 <= v2)
	case opGe:
		return b2i(v1 >= v2)
	case opAnd:
		return v1 & v2
	case opOr:
		return v1 | v2
	case opXor:
		return v1 ^ v2
	case opLand:
		return b2i(v1 != 0 && v2 != 0)
	case opLor:
		return b2i(v1 != 0 || v2 != 0)
	case opNot:
		return b2i(v1 == 0)
	case opBitNot:
		return ^v1
	case opUminus:
		return -v1
	case opShiftLeft, opShiftRight:
		if v2 < 0 {
			panic(&diag.Error{Msg: fmt.Sprintf("shift count %d is negative", v2)})
		}
		if op == opShiftLeft {
			return ir.Shl(v1, v2)
		}
		return ir.Shr(v1, v2)
	}
	panic("unreachable")
}

// ---------------------------------------------------------------
// 型式の評価
// ---------------------------------------------------------------

// foldType は定数の演算の結果の型 (オペランドに型付きの整数定数があるとき。型のない定数は相手に合わせる)。
// 全部が型のない定数なら nil (値から型を決める IntValue)。`(200 as u8) + (100 as u8)` や `const A:u8 = 200; A + 100` は
// u8 で 44 (変数と同じ。畳み込みで 300 のまま広がっていた。survey 2026-09-27)。
func (h *Hlc) foldType(args []*cexpr, cmp bool) *types.Type {
	var ops []ir.Operand
	typed := false
	for _, a := range args {
		v := a.val
		if v.Type.Kind != types.Int || v.Type.Enum != nil {
			return nil // bool・enum・ポインタの定数は今までどおり
		}
		typed = typed || !v.Untyped
		ops = append(ops, v)
	}
	if !typed {
		return nil
	}
	if len(ops) == 1 {
		return ir.ValType(ops[0])
	}
	var t *types.Type
	for _, o := range ops[1:] {
		a, b := h.adaptLiteralNoErr(ops[0], o, cmp)
		ct := h.prog.Types.CommonType(ir.ValType(a), ir.ValType(b))
		if ct == nil {
			return nil
		}
		if t == nil {
			t = ct
		} else if t = h.prog.Types.CommonType(t, ct); t == nil {
			return nil
		}
	}
	return t
}

// wrapInt は n を整数型 t の値に折り返す (幅で切り詰めて、その符号で読む)。
func wrapInt(n int, t *types.Type) int {
	if t.Size <= 0 || t.Size >= 8 {
		return n
	}
	bits := 8 * t.Size
	n = ir.FloorMod(n, 1<<bits)
	if t.Signed && n >= 1<<(bits-1) {
		n -= 1 << bits
	}
	return n
}

// checkRaggedLiteral は、長さの揃わない入れ子の配列リテラル (`[[1, 2, 3], [4, 5]]`) が、ポインタの配列への変換
// (`const PS:[N]*T = [...]`) を通らずに残っていたらエラーにする (要素の互換型がポインタになり、要素が配列のまま残って
// codegen が panic していた。survey 2026-09-27)。
func checkRaggedLiteral(v *ir.Value) {
	if v.Kind != ir.KindArrayLiteral || v.Type.Kind != types.Array || v.Type.Base.Kind != types.Pointer {
		return
	}
	for _, e := range v.Elems {
		if ev := ir.ValLiteral(e); ev != nil && ev.Kind == ir.KindArrayLiteral {
			panic(&diag.Error{Msg: "elements of an array literal have different lengths; declare the type, e.g. `const M:[?][]const u8 = [...]` (rows as slices) or `const M:[?]*const u8 = [...]`"})
		}
	}
}

// symExpr は名前 sym の参照の節点 (値なら評価済みの値、値でなければ cName)。
func (h *Hlc) symExpr(sym *Symbol) *cexpr {
	v := sym.Val
	if v == nil {
		return symName(sym) // soa のコンテナは値 (型名も兼ねる)
	}
	if str, ok := h.prog.buildStrings[v]; ok {
		return h.constEval(cstr(str)) // 文字列の @(build) の const は使った場所で文字列リテラルに
	}
	if a := h.exprAliases[v]; a != nil {
		return a // 評価済みの式そのもの (代入の検査は同じ node かで見る)
	}
	return cv(v)
}
