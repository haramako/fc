package syntax

// 型付き構文木 (doc/v2_plan.md R1-b)。
//
// 設計原則:
//   - 全ノードが Pos()/End() を持つ (C1: ロスレス。フォーマッタがコメントと共に元の構造を復元できる)
//   - ノードは「意味」で命名し、表層文法を知るのはパーサとプリンタだけ (C2: 文法バージョン中立)
//   - `+=` や `+x` の脱糖はここでは行わない (sema の仕事)。括弧もノードとして残す
//   - ノードは不変として扱う。sema は書き換えない
//
// 位置の規約: 各ノードは自身の範囲を決めるトークンの位置をフィールドに持つ。
// Pos() は先頭トークンの開始位置、End() は末尾トークンの終端 (その次の位置)。

// Node は全構文木ノードの共通インターフェース。
type Node interface {
	Pos() Pos
	End() Pos
}

// Stmt は文。
type Stmt interface {
	Node
	stmtNode()
}

// Expr は式。
type Expr interface {
	Node
	exprNode()
}

// TypeExpr は型式。
type TypeExpr interface {
	Node
	typeNode()
}

// File は 1 ソースファイル。
type File struct {
	Filename string
	Stmts    []Stmt
	Comments []Comment // 出現順
	EOFPos   Pos
}

func (f *File) Pos() Pos {
	if len(f.Stmts) > 0 {
		return f.Stmts[0].Pos()
	}
	return f.EOFPos
}
func (f *File) End() Pos { return f.EOFPos }

// ---------------------------------------------------------------
// 文
// ---------------------------------------------------------------

// VarDecl は `var` / `const` 宣言文。`public var ...` の形で可視性を持てる。
type VarDecl struct {
	PublicPos Pos // `public` の位置 (省略時は !IsValid())
	Keyword   Pos // `var` / `const`
	Const     bool
	Specs     []*VarSpec
	Semi      Pos
}

// VarSpec は 1 変数の宣言 `name: type = init options(...)`。
// Type と Init は片方が省略可 (両方省略は文法上不可)。
type VarSpec struct {
	Name    *Ident
	Type    TypeExpr // nil なら型推論
	Init    Expr     // nil なら初期値なし
	Options *Options // nil なら省略
}

// FuncDecl は関数宣言。Body が nil なら `;` による宣言のみ (extern)。
type FuncDecl struct {
	PublicPos Pos
	Keyword   Pos // `function`
	Name      *Ident
	Params    []*VarSpec
	Result    TypeExpr
	Options   *Options // nil なら省略
	Body      *Block   // nil なら `;`
	Semi      Pos      // Body == nil のときの `;`
}

// IfStmt は if 文。Else は nil / 任意の文 (`else stmt`) / *IfStmt (`elsif`: IsElsif が真)。
type IfStmt struct {
	If      Pos // `if` または `elsif`
	IsElsif bool
	Lparen  Pos
	Cond    Expr
	Rparen  Pos
	Then    Stmt
	Else    Stmt
}

// LoopStmt は `loop() stmt`。
type LoopStmt struct {
	Loop   Pos
	Rparen Pos
	Body   Stmt
}

// WhileStmt は `while(cond) stmt`。
type WhileStmt struct {
	While  Pos
	Cond   Expr
	Rparen Pos
	Body   Stmt
}

// ForStmt は `for(var, from, to) { ... }`。
type ForStmt struct {
	For    Pos
	Var    *Ident
	From   Expr
	To     Expr
	Rparen Pos
	Body   *Block
}

// BreakStmt は `break;`。
type BreakStmt struct {
	Keyword Pos
	Semi    Pos
}

// ContinueStmt は `continue;`。
type ContinueStmt struct {
	Keyword Pos
	Semi    Pos
}

// ReturnStmt は `return [value];`。
type ReturnStmt struct {
	Return Pos
	Value  Expr // nil なら値なし
	Semi   Pos
}

// SwitchStmt は switch 文。
type SwitchStmt struct {
	Switch  Pos
	Tag     Expr
	Lbrace  Pos
	Cases   []*CaseClause
	Default *DefaultClause // nil なら省略
	Rbrace  Pos
}

// CaseClause は `case v1, v2: stmts` (stmts は 1 つ以上)。
type CaseClause struct {
	Case   Pos
	Values []Expr
	Colon  Pos
	Body   []Stmt
}

// DefaultClause は `default: stmts` (stmts は 1 つ以上)。
type DefaultClause struct {
	Default Pos
	Colon   Pos
	Body    []Stmt
}

// ExprStmt は式文 `expr;`。
type ExprStmt struct {
	X    Expr
	Semi Pos
}

// OptionsStmt は `options(...);` 文 (モジュール属性)。
type OptionsStmt struct {
	Options *Options
	Semi    Pos
}

// UseDecl は `use mod;` / `use mod as x;` / `use * from mod;`。
type UseDecl struct {
	Use     Pos
	Star    Pos // `*` の位置。FromAll のとき有効
	FromAll bool
	Module  *Ident
	As      *Ident // nil なら省略
	Semi    Pos
}

// IncludeDecl は `include [kind]("path") [options(...)];`。Kind は `macro` 等の修飾子。
type IncludeDecl struct {
	Include Pos
	Kind    *Ident // nil なら省略
	Path    *StringLit
	Rparen  Pos
	Options *Options // nil なら省略
	Semi    Pos
}

// ScopeLabel は `public:` / `private:` ラベル。
type ScopeLabel struct {
	Keyword Pos
	Public  bool
	Colon   Pos
}

// Block は `{ stmts }`。文としても、関数・for・call のボディとしても使われる。
type Block struct {
	Lbrace Pos
	Stmts  []Stmt
	Rbrace Pos
}

// EmptyStmt は単独の `;`。
type EmptyStmt struct {
	Semi Pos
}

// ---------------------------------------------------------------
// 式
// ---------------------------------------------------------------

// Ident は識別子。
type Ident struct {
	NamePos Pos
	Name    string
}

// IntLit は整数リテラル。Text はソース上の綴り (`0x10` 等)。
type IntLit struct {
	ValuePos Pos
	Value    int
	Text     string
}

// StringLit は文字列リテラル。Value はエスケープ解釈後、Text はソース上の綴り (引用符含む)。
type StringLit struct {
	ValuePos Pos
	EndPos   Pos
	Value    string
	Text     string
}

// ParenExpr は `(x)`。
type ParenExpr struct {
	Lparen Pos
	X      Expr
	Rparen Pos
}

// BinaryExpr は二項演算。Op は演算子トークンの種別
// (Dot Plus Minus Star Slash Percent Amp Pipe Caret AndAnd OrOr EqEq Neq Lt Gt Leq Geq Shl Shr)。
type BinaryExpr struct {
	X     Expr
	OpPos Pos
	Op    Kind
	Y     Expr
}

// AssignExpr は代入 `lhs = rhs` / `lhs += rhs` / `lhs -= rhs`。Op は Assign / AddEq / SubEq。
type AssignExpr struct {
	Lhs   Expr
	OpPos Pos
	Op    Kind
	Rhs   Expr
}

// UnaryExpr は前置単項演算。Op は Not (`!`) / Minus / Plus / Star (デリファレンス) / Amp (参照)。
type UnaryExpr struct {
	OpPos Pos
	Op    Kind
	X     Expr
}

// CastExpr は `<type> x`。
type CastExpr struct {
	Lt   Pos
	Type TypeExpr
	Gt   Pos
	X    Expr
}

// CallExpr は `fun(args) [{ block }]`。Block はマクロ呼び出し用の後置ブロック (nil なら省略)。
type CallExpr struct {
	Fun    Expr
	Lparen Pos
	Args   []Expr
	Rparen Pos
	Block  *Block
}

// IndexExpr は `x[index]`。
type IndexExpr struct {
	X      Expr
	Lbrack Pos
	Index  Expr
	Rbrack Pos
}

// ArrayLit は `[e1, e2, ...]`。
type ArrayLit struct {
	Lbrack Pos
	Elems  []Expr
	Rbrack Pos
}

// IncbinExpr は `incbin("path")`。
type IncbinExpr struct {
	Incbin Pos
	Path   *StringLit
	Rparen Pos
}

// LambdaExpr は `-> type { body }` / `-> type ;`。
type LambdaExpr struct {
	Arrow Pos
	Type  TypeExpr
	Body  *Block // nil なら `;`
	Semi  Pos    // Body == nil のときの `;`
}

// ---------------------------------------------------------------
// 型式
// ---------------------------------------------------------------

// NamedType は型名 (`int`, `uint8`, `void` など)。
type NamedType struct {
	Name *Ident
}

// ArrayType は `elem[len]` / `elem[]`。
type ArrayType struct {
	Elem   TypeExpr
	Lbrack Pos
	Len    Expr // nil なら長さ省略
	Rbrack Pos
}

// PointerType は `elem*`。
type PointerType struct {
	Elem TypeExpr
	Star Pos
}

// FuncType は `result(params)`。
type FuncType struct {
	Result TypeExpr
	Lparen Pos
	Params []*Param
	Rparen Pos
}

// Param は関数型の引数。Name は省略可。
type Param struct {
	Name *Ident // nil なら型のみ
	Type TypeExpr
}

// ---------------------------------------------------------------
// options
// ---------------------------------------------------------------

// Options は `options(k1: v1, k2: v2)`。順序を保持する。
type Options struct {
	Keyword Pos
	Entries []*OptionEntry
	Rparen  Pos
}

// OptionEntry は `key: value`。
type OptionEntry struct {
	Key   *Ident
	Value Expr
}

// Get は key に一致する最後のエントリの値を返す (重複時は後勝ち)。
func (o *Options) Get(key string) Expr {
	if o == nil {
		return nil
	}
	var r Expr
	for _, e := range o.Entries {
		if e.Key.Name == key {
			r = e.Value
		}
	}
	return r
}

// ---------------------------------------------------------------
// Pos / End
// ---------------------------------------------------------------

// after は p から n バイト後の位置 (同一行内のトークン終端の計算用)。
func after(p Pos, n int) Pos {
	return Pos{Offset: p.Offset + n, Line: p.Line, Col: p.Col + n}
}

func firstValid(ps ...Pos) Pos {
	for _, p := range ps {
		if p.IsValid() {
			return p
		}
	}
	return Pos{}
}

func (s *VarDecl) Pos() Pos { return firstValid(s.PublicPos, s.Keyword) }
func (s *VarDecl) End() Pos { return after(s.Semi, 1) }

func (s *FuncDecl) Pos() Pos { return firstValid(s.PublicPos, s.Keyword) }
func (s *FuncDecl) End() Pos {
	if s.Body != nil {
		return s.Body.End()
	}
	return after(s.Semi, 1)
}

func (s *IfStmt) Pos() Pos { return s.If }
func (s *IfStmt) End() Pos {
	if s.Else != nil {
		return s.Else.End()
	}
	return s.Then.End()
}

func (s *LoopStmt) Pos() Pos { return s.Loop }
func (s *LoopStmt) End() Pos { return s.Body.End() }

func (s *WhileStmt) Pos() Pos { return s.While }
func (s *WhileStmt) End() Pos { return s.Body.End() }

func (s *ForStmt) Pos() Pos { return s.For }
func (s *ForStmt) End() Pos { return s.Body.End() }

func (s *BreakStmt) Pos() Pos { return s.Keyword }
func (s *BreakStmt) End() Pos { return after(s.Semi, 1) }

func (s *ContinueStmt) Pos() Pos { return s.Keyword }
func (s *ContinueStmt) End() Pos { return after(s.Semi, 1) }

func (s *ReturnStmt) Pos() Pos { return s.Return }
func (s *ReturnStmt) End() Pos { return after(s.Semi, 1) }

func (s *SwitchStmt) Pos() Pos { return s.Switch }
func (s *SwitchStmt) End() Pos { return after(s.Rbrace, 1) }

func (c *CaseClause) Pos() Pos { return c.Case }
func (c *CaseClause) End() Pos { return c.Body[len(c.Body)-1].End() }

func (c *DefaultClause) Pos() Pos { return c.Default }
func (c *DefaultClause) End() Pos { return c.Body[len(c.Body)-1].End() }

func (s *ExprStmt) Pos() Pos { return s.X.Pos() }
func (s *ExprStmt) End() Pos { return after(s.Semi, 1) }

func (s *OptionsStmt) Pos() Pos { return s.Options.Pos() }
func (s *OptionsStmt) End() Pos { return after(s.Semi, 1) }

func (s *UseDecl) Pos() Pos { return s.Use }
func (s *UseDecl) End() Pos { return after(s.Semi, 1) }

func (s *IncludeDecl) Pos() Pos { return s.Include }
func (s *IncludeDecl) End() Pos { return after(s.Semi, 1) }

func (s *ScopeLabel) Pos() Pos { return s.Keyword }
func (s *ScopeLabel) End() Pos { return after(s.Colon, 1) }

func (s *Block) Pos() Pos { return s.Lbrace }
func (s *Block) End() Pos { return after(s.Rbrace, 1) }

func (s *EmptyStmt) Pos() Pos { return s.Semi }
func (s *EmptyStmt) End() Pos { return after(s.Semi, 1) }

func (e *Ident) Pos() Pos { return e.NamePos }
func (e *Ident) End() Pos { return after(e.NamePos, len(e.Name)) }

func (e *IntLit) Pos() Pos { return e.ValuePos }
func (e *IntLit) End() Pos { return after(e.ValuePos, len(e.Text)) }

func (e *StringLit) Pos() Pos { return e.ValuePos }
func (e *StringLit) End() Pos { return e.EndPos }

func (e *ParenExpr) Pos() Pos { return e.Lparen }
func (e *ParenExpr) End() Pos { return after(e.Rparen, 1) }

func (e *BinaryExpr) Pos() Pos { return e.X.Pos() }
func (e *BinaryExpr) End() Pos { return e.Y.End() }

func (e *AssignExpr) Pos() Pos { return e.Lhs.Pos() }
func (e *AssignExpr) End() Pos { return e.Rhs.End() }

func (e *UnaryExpr) Pos() Pos { return e.OpPos }
func (e *UnaryExpr) End() Pos { return e.X.End() }

func (e *CastExpr) Pos() Pos { return e.Lt }
func (e *CastExpr) End() Pos { return e.X.End() }

func (e *CallExpr) Pos() Pos { return e.Fun.Pos() }
func (e *CallExpr) End() Pos {
	if e.Block != nil {
		return e.Block.End()
	}
	return after(e.Rparen, 1)
}

func (e *IndexExpr) Pos() Pos { return e.X.Pos() }
func (e *IndexExpr) End() Pos { return after(e.Rbrack, 1) }

func (e *ArrayLit) Pos() Pos { return e.Lbrack }
func (e *ArrayLit) End() Pos { return after(e.Rbrack, 1) }

func (e *IncbinExpr) Pos() Pos { return e.Incbin }
func (e *IncbinExpr) End() Pos { return after(e.Rparen, 1) }

func (e *LambdaExpr) Pos() Pos { return e.Arrow }
func (e *LambdaExpr) End() Pos {
	if e.Body != nil {
		return e.Body.End()
	}
	return after(e.Semi, 1)
}

func (t *NamedType) Pos() Pos { return t.Name.Pos() }
func (t *NamedType) End() Pos { return t.Name.End() }

func (t *ArrayType) Pos() Pos { return t.Elem.Pos() }
func (t *ArrayType) End() Pos { return after(t.Rbrack, 1) }

func (t *PointerType) Pos() Pos { return t.Elem.Pos() }
func (t *PointerType) End() Pos { return after(t.Star, 1) }

func (t *FuncType) Pos() Pos { return t.Result.Pos() }
func (t *FuncType) End() Pos { return after(t.Rparen, 1) }

func (p *Param) Pos() Pos {
	if p.Name != nil {
		return p.Name.Pos()
	}
	return p.Type.Pos()
}
func (p *Param) End() Pos { return p.Type.End() }

func (o *Options) Pos() Pos { return o.Keyword }
func (o *Options) End() Pos { return after(o.Rparen, 1) }

func (e *OptionEntry) Pos() Pos { return e.Key.Pos() }
func (e *OptionEntry) End() Pos { return e.Value.End() }

func (s *VarSpec) Pos() Pos { return s.Name.Pos() }
func (s *VarSpec) End() Pos {
	switch {
	case s.Options != nil:
		return s.Options.End()
	case s.Init != nil:
		return s.Init.End()
	default:
		return s.Type.End()
	}
}

// マーカーメソッド
func (*VarDecl) stmtNode()      {}
func (*FuncDecl) stmtNode()     {}
func (*IfStmt) stmtNode()       {}
func (*LoopStmt) stmtNode()     {}
func (*WhileStmt) stmtNode()    {}
func (*ForStmt) stmtNode()      {}
func (*BreakStmt) stmtNode()    {}
func (*ContinueStmt) stmtNode() {}
func (*ReturnStmt) stmtNode()   {}
func (*SwitchStmt) stmtNode()   {}
func (*ExprStmt) stmtNode()     {}
func (*OptionsStmt) stmtNode()  {}
func (*UseDecl) stmtNode()      {}
func (*IncludeDecl) stmtNode()  {}
func (*ScopeLabel) stmtNode()   {}
func (*Block) stmtNode()        {}
func (*EmptyStmt) stmtNode()    {}

func (*Ident) exprNode()      {}
func (*IntLit) exprNode()     {}
func (*StringLit) exprNode()  {}
func (*ParenExpr) exprNode()  {}
func (*BinaryExpr) exprNode() {}
func (*AssignExpr) exprNode() {}
func (*UnaryExpr) exprNode()  {}
func (*CastExpr) exprNode()   {}
func (*CallExpr) exprNode()   {}
func (*IndexExpr) exprNode()  {}
func (*ArrayLit) exprNode()   {}
func (*IncbinExpr) exprNode() {}
func (*LambdaExpr) exprNode() {}

func (*NamedType) typeNode()   {}
func (*ArrayType) typeNode()   {}
func (*PointerType) typeNode() {}
func (*FuncType) typeNode()    {}
