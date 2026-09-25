package sema

// fc 3 の読み取り専用ポインタ `*const T` (doc/language_feature_candidates.md §3)。const の配列・文字列リテラルの値
// (ir.Value.ReadOnly) と、そこから添字・アドレス・ポインタ経由のフィールドで作るポインタは *const。*T → *const T は
// 暗黙に変換でき、*const T を通した書き込みと `as` で const を外すことはエラー、外すのは @bitcast だけ。読み取り専用の
// データを *T として渡すのは、fc 3 の最初の版では警告 (fclib と利用側を *const に直してからエラーにする)。

import (
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// storageType は変数 (引数・ローカル・グローバル・戻り値) の IR の型。`*const T` は `*T` にして、読み取り専用は値の印
// (ir.Value.ReadOnly) で持つ (IR の型が変わると最適化が変わり、*const を書いただけで生成コードが変わるので)。関数の型
// (シグネチャ) には *const を残す (渡すときの検査に使う)。
func (h *Hlc) storageType(t *types.Type) (*types.Type, bool) {
	if t != nil && t.Kind == types.Pointer && t.ReadOnly {
		return h.prog.Types.PointerTo(t.Base), true
	}
	return t, false
}

// markReadOnly は一時変数 tmp が読み取り専用のデータを指すことを覚える (ro なら)。IR の型は *T のまま
// (型が変わると写しの統合などの最適化が変わり、fc 2 の生成コードまで変わるので、読み取り専用は意味解析だけの情報にする)。
func (h *Hlc) markReadOnly(tmp *ir.Value, ro bool) {
	if ro {
		tmp.ReadOnly = true
	}
}

// readOnly は op が書き換えられないデータか: 読み取り専用のポインタ (*const) か、const の配列・文字列リテラルの値、
// またはそこから作ったポインタの一時変数。
func (h *Hlc) readOnly(op ir.Operand) bool {
	if t := ir.ValType(op); t != nil && t.Kind == types.Pointer && t.ReadOnly {
		return true
	}
	switch v := op.(type) {
	case *ir.Value:
		return v.ReadOnly
	case *ir.PointeredArray:
		return h.readOnly(v.From)
	case *ir.CastedValue:
		return !h.prog.unconst[v] && h.readOnly(v.From)
	}
	return false
}

// warnDropConst は読み取り専用のデータを書き換えられるポインタ (*T) として渡すときに警告する (fc 3 の最初の版は警告。
// doc/language_feature_candidates.md §3)。
func (h *Hlc) warnDropConst(what string, to *types.Type, from ir.Operand) {
	if to == nil || to.Kind != types.Pointer || to.ReadOnly || !h.readOnly(from) || h.version() < syntax.Version3 {
		return
	}
	h.warn("%s: passes read-only data as %s (use *const %s, or @bitcast(%s, x) to drop const)", what, to, to.Base, to)
}
