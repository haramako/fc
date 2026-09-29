package ir

// 6502 のレジスタ (常駐の印 Op.Res の添字)。

// Reg は A / Y / X。Op.Res と codegen の状態の添字に使う (以前は A / Y / X ごとに別のフィールドで、同じ処理が 3 回
// 書かれていた)。
type Reg uint8

const (
	RegA Reg = iota
	RegY
	RegX
	NumRegs
)

func (r Reg) String() string { return [...]string{"a", "y", "x"}[r] }

// Loc はそのレジスタに置かれた値の Location。
func (r Reg) Loc() Location { return [...]Location{LocA, LocY, LocX}[r] }

// RegOfLoc は Location がレジスタなら (その Reg, true)。
func RegOfLoc(loc Location) (Reg, bool) {
	switch loc {
	case LocA:
		return RegA, true
	case LocY:
		return RegY, true
	case LocX:
		return RegX, true
	}
	return 0, false
}

// Residency は命令 1 つでのレジスタ 1 つの常駐の印 (regalloc.AllocateResident が付ける。Agent/wiki/design/regalloc.md)。
type Residency struct {
	V   *Value // この命令でそのレジスタに置いたままにしている変数 (Location はそのレジスタ、Home がメモリ側)。nil なら無し
	In  bool   // V が命令の入口で生きている (レジスタに値がある)
	Out bool   // V が命令の出口で生きている
}

// HasResident はどれかのレジスタに常駐の印があるか。
func (op *Op) HasResident() bool {
	for _, r := range op.Res {
		if r.V != nil {
			return true
		}
	}
	return false
}
