package syntax

// goyacc 生成パーサ (parser.go) と Lexer の接続、および構文木構築のヘルパ。

//go:generate goyacc -o parser.go -p yy parser.y

import (
	"fmt"
	"strings"
)

// Parse は fc ソースを構文解析して構文木を返す。
// 字句・構文エラーは *Error (位置付き) で返す。構文エラーのメッセージは
// 旧実装 (racc) と同じく "parse error" で始まる。
func Parse(src []byte, filename string) (*File, error) {
	lx := &yyLexAdapter{lex: NewLexer(src, filename)}
	rc := yyParse(lx)
	if lx.lexErr != nil {
		return nil, lx.lexErr
	}
	if lx.parseErr != nil {
		return nil, lx.parseErr
	}
	if rc != 0 {
		return nil, &Error{Filename: filename, Pos: lx.last.Pos, Msg: "parse error"}
	}
	f := &File{
		Filename: filename,
		Version:  lx.lex.Version(),
		Pragma:   lx.lex.Pragma(),
		Stmts:    lx.result,
		Comments: lx.lex.Comments(),
		EOFPos:   lx.last.Pos,
	}
	if err := checkVersion(f); err != nil {
		return nil, err
	}
	return f, nil
}

// checkVersion は文法バージョンごとの受理範囲を検査する (doc/v2_grammar.md §4.1)。
// 文法ファイルは v1 ∪ v2 のスーパーセットなので、「そのバージョンに無い構文」をここで落とす。
func checkVersion(f *File) error {
	var err *Error
	fail := func(pos Pos, msg string) {
		if err == nil {
			err = &Error{Filename: f.Filename, Pos: pos, Msg: msg}
		}
	}
	checkTypeForm := func(prefix bool, pos Pos) {
		if f.Version >= Version2 && !prefix {
			fail(pos, "postfix types (int*, int[4], void(int)) are written prefix in fc 2 (*int, [4]int, fn(int):void)")
		}
		if f.Version < Version2 && prefix {
			fail(pos, "prefix types (*int, [4]int, fn(int):void) require fc 2")
		}
	}
	Inspect(f, func(n Node) bool {
		if err != nil {
			return false
		}
		switch n := n.(type) {
		case *ScopeLabel:
			if f.Version >= Version2 {
				fail(n.Keyword, "public:/private: labels are not allowed in fc 2 (declarations are private by default; mark exports with `public`)")
			}
		case *UseDecl:
			if f.Version < Version2 && n.PublicPos.IsValid() {
				fail(n.PublicPos, "`public use` requires fc 2 (add `#fc 2` to the first line)")
			}
			if f.Version < Version2 && len(n.Names) > 0 {
				fail(n.Names[0].NamePos, "`use a, b from mod;` requires fc 2 (add `#fc 2` to the first line)")
			}
		case *LoopStmt:
			if f.Version >= Version2 && n.Rparen.IsValid() {
				fail(n.Loop, "`loop()` is written `loop` in fc 2")
			}
			if f.Version < Version2 && !n.Rparen.IsValid() {
				fail(n.Loop, "`loop { ... }` requires fc 2 (write `loop() { ... }` in fc 1)")
			}
		case *ForStmt:
			if f.Version >= Version2 && n.IsV1() {
				fail(n.For, "`for (i, from, to)` is written `for (i = from; i < to; i++)` in fc 2")
			}
			if f.Version < Version2 && !n.IsV1() {
				fail(n.For, "C-style `for (init; cond; step)` requires fc 2")
			}
		case *ArrayType:
			checkTypeForm(n.IsPrefix(), n.Lbrack)
		case *PointerType:
			checkTypeForm(n.IsPrefix(), n.Star)
		case *FuncType:
			checkTypeForm(n.IsPrefix(), n.Lparen)
		case *CastExpr:
			if f.Version >= Version2 && n.Kind == CastLegacy {
				fail(n.Lt, "`<T>x` is written `x as T` (numeric conversion) or `bitcast<T>(x)` (bit reinterpretation) in fc 2")
			}
			if f.Version < Version2 && n.Kind != CastLegacy {
				fail(n.Pos(), "`as` / `bitcast` require fc 2")
			}
		case *AssignExpr:
			if f.Version < Version2 && n.Op.IsCompoundAssign() && n.Op != AddEq && n.Op != SubEq {
				fail(n.OpPos, fmt.Sprintf("`%s` requires fc 2", n.Op))
			}
		case *UnaryExpr:
			if f.Version < Version2 && n.Op == Tilde {
				fail(n.OpPos, "`~` requires fc 2")
			}
		case *StructDecl:
			if f.Version < Version2 {
				fail(n.Keyword, "`struct` requires fc 2")
			}
		case *SoaDecl:
			if f.Version < Version2 {
				fail(n.Keyword, "`soa` requires fc 2")
			}
		case *StructLit:
			if f.Version < Version2 {
				fail(n.Lbrace, "struct literals require fc 2")
			}
		case *SizeofExpr:
			if f.Version < Version2 {
				fail(n.Sizeof, "`sizeof` requires fc 2")
			}
		case *NamedType:
			if f.Version < Version2 && n.Module != nil {
				fail(n.Module.NamePos, "qualified type names (mod.T) require fc 2")
			}
		case *ArrayLit:
			if f.Version < Version2 && n.Comma.IsValid() {
				fail(n.Comma, "trailing comma requires fc 2")
			}
		case *CallExpr:
			if f.Version < Version2 && n.Comma.IsValid() {
				fail(n.Comma, "trailing comma requires fc 2")
			}
		case *IncDecStmt:
			if f.Version < Version2 {
				fail(n.OpPos, "`++` / `--` require fc 2")
			}
			// `y = x++` は文法上 `(y = x)++` に読めてしまう。++/-- は文なので代入の中では使えない
			if _, ok := n.X.(*AssignExpr); ok {
				fail(n.OpPos, "`++` / `--` is a statement and cannot be used inside an expression")
			}
		case *LabeledStmt:
			if f.Version < Version2 {
				fail(n.Label.NamePos, "statement labels require fc 2")
			}
			switch n.Stmt.(type) {
			case *LoopStmt, *WhileStmt, *ForStmt, *SwitchStmt:
			default:
				fail(n.Label.NamePos, "a label must be placed on loop / while / for / switch")
			}
		case *BreakStmt:
			if f.Version < Version2 && n.Label != nil {
				fail(n.Label.NamePos, "`break label;` requires fc 2")
			}
		case *ContinueStmt:
			if f.Version < Version2 && n.Label != nil {
				fail(n.Label.NamePos, "`continue label;` requires fc 2")
			}
		case *IncludeDecl:
			if f.Version >= Version2 && n.Kind != nil {
				fail(n.Kind.NamePos, fmt.Sprintf("include %s(...) is not allowed in fc 2 (the kind is decided by the file extension)", n.Kind.Name))
			}
		}
		return true
	})
	if err != nil {
		return err
	}
	return nil
}

// yyLexAdapter は goyacc の yyLexer インターフェースを Lexer で実装する。
type yyLexAdapter struct {
	lex      *Lexer
	last     Token  // 最後に返したトークン (EOF を含む)
	result   []Stmt // program の reduce で設定される
	lexErr   *Error
	parseErr *Error
}

// Kind → goyacc トークン番号。1 文字記号はその文字コード。
var kindToYacc = map[Kind]int{
	Number: NUMBER, Identifier: IDENT, String: STRING,
	KwInclude: kINCLUDE, KwFunction: kFUNCTION, KwConst: kCONST, KwVar: kVAR,
	KwOptions: kOPTIONS, KwIf: kIF, KwElse: kELSE, KwElsif: kELSIF,
	KwLoop: kLOOP, KwWhile: kWHILE, KwFor: kFOR, KwReturn: kRETURN,
	KwBreak: kBREAK, KwContinue: kCONTINUE, KwIncbin: kINCBIN,
	KwSwitch: kSWITCH, KwCase: kCASE, KwDefault: kDEFAULT,
	KwUse: kUSE, KwAs: kAS, KwFrom: kFROM, KwPublic: kPUBLIC, KwPrivate: kPRIVATE, KwFn: kFN, KwBitcast: kBITCAST, KwStruct: kSTRUCT, KwSizeof: kSIZEOF, KwSoa: kSOA,
	Leq: LEQ, Geq: GEQ, EqEq: EQEQ, AddEq: ADDEQ, SubEq: SUBEQ, Neq: NEQ, Arrow: ARROW,
	Shl: LSHIFT, Shr: RSHIFT, AndAnd: ANDAND, OrOr: OROR, Inc: INCR, Dec: DECR,
	MulEq: MULEQ, DivEq: DIVEQ, ModEq: MODEQ, AndEq: ANDEQ, OrEq: OREQ, XorEq: XOREQ, ShlEq: SHLEQ, ShrEq: SHREQ,
	LParen: '(', RParen: ')', LBrace: '{', RBrace: '}', Semicolon: ';', Colon: ':',
	Lt: '<', Gt: '>', LBrack: '[', RBrack: ']', Plus: '+', Minus: '-', Star: '*',
	Slash: '/', Percent: '%', Amp: '&', Pipe: '|', Caret: '^', Assign: '=',
	Comma: ',', Dot: '.', Not: '!', Tilde: '~',
}

func (a *yyLexAdapter) Lex(lval *yySymType) int {
	if a.lexErr != nil {
		return 0
	}
	t, err := a.lex.Next()
	if err != nil {
		// 字句エラーは記録して EOF を返し、Parse が優先して報告する
		a.lexErr = err.(*Error)
		return 0
	}
	a.last = t
	lval.tok = t
	if t.Kind == EOF {
		return 0
	}
	return kindToYacc[t.Kind]
}

func (a *yyLexAdapter) Error(s string) {
	if a.parseErr != nil {
		return
	}
	// racc は "parse error on value ..." 形式 (errors.fc が /parse error/ で照合する)
	msg := strings.Replace(s, "syntax error", "parse error", 1)
	a.parseErr = &Error{Filename: a.lex.Filename(), Pos: a.last.Pos, Msg: msg}
}

// argList は arg_list (引数・配列要素) の値。comma は末尾のカンマの位置 (無ければ無効)。
type argList struct {
	exprs []Expr
	comma Pos
}

// funcBody は function_block (ブロック or `;`) の値。
type funcBody struct {
	Block *Block
	Semi  Pos
}

func optPos(t *Token) Pos {
	if t == nil {
		return Pos{}
	}
	return t.Pos
}

func ident(t Token) *Ident {
	return &Ident{NamePos: t.Pos, Name: t.Text}
}

func strLit(t Token) *StringLit {
	return &StringLit{ValuePos: t.Pos, EndPos: t.End, Value: t.Str, Text: t.Text}
}

func binary(x Expr, op Token, y Expr) Expr {
	return &BinaryExpr{X: x, OpPos: op.Pos, Op: op.Kind, Y: y}
}

func unary(op Token, x Expr) Expr {
	return &UnaryExpr{OpPos: op.Pos, Op: op.Kind, X: x}
}

func funcDecl(scope *Token, kw, name Token, params []*VarSpec, result TypeExpr, opts *Options, body funcBody) Stmt {
	return &FuncDecl{
		PublicPos: optPos(scope),
		Keyword:   kw.Pos,
		Name:      ident(name),
		Params:    params,
		Result:    result,
		Options:   opts,
		Body:      body.Block,
		Semi:      body.Semi,
	}
}
