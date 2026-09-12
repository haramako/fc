package syntax

// goyacc 生成パーサ (parser.go) と Lexer の接続、および構文木構築のヘルパ。

//go:generate goyacc -o parser.go -p yy parser.y

import (
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
	return &File{
		Filename: filename,
		Stmts:    lx.result,
		Comments: lx.lex.Comments(),
		EOFPos:   lx.last.Pos,
	}, nil
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
	KwUse: kUSE, KwAs: kAS, KwFrom: kFROM, KwPublic: kPUBLIC, KwPrivate: kPRIVATE,
	Leq: LEQ, Geq: GEQ, EqEq: EQEQ, AddEq: ADDEQ, SubEq: SUBEQ, Neq: NEQ, Arrow: ARROW,
	Shl: LSHIFT, Shr: RSHIFT, AndAnd: ANDAND, OrOr: OROR,
	LParen: '(', RParen: ')', LBrace: '{', RBrace: '}', Semicolon: ';', Colon: ':',
	Lt: '<', Gt: '>', LBrack: '[', RBrack: ']', Plus: '+', Minus: '-', Star: '*',
	Slash: '/', Percent: '%', Amp: '&', Pipe: '|', Caret: '^', Assign: '=',
	Comma: ',', Dot: '.', Not: '!',
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
