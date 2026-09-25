package migrate

import (
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

func init() {
	Rules = append(Rules,
		Rule{Name: "int-types", Doc: "整数型名を fc 3 の名前にする (int / uint8 → u8、sint → i8、int16 → u16、sint16 → i16)", Apply: renameIntTypes},
	)
}

// renameIntTypes は型の位置の fc 2 だけの整数型名 (モジュール名の付かない NamedType) を fc 3 の名前にする。
// 変数名・コメント・文字列の中の同じ綴りは書き換えない (構文木の型名だけを見る)。doc/v3_plan.md §7。
func renameIntTypes(c *Ctx) {
	syntax.Inspect(c.File, func(n syntax.Node) bool {
		nt, ok := n.(*syntax.NamedType)
		if !ok || nt.Module != nil || nt.Name == nil {
			return true
		}
		if v3, old := types.V2IntTypeNames[nt.Name.Name]; old {
			off := nt.Name.NamePos.Offset
			c.Replace(off, off+len(nt.Name.Name), v3)
		}
		return true
	})
}
