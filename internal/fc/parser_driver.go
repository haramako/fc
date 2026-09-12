package fc

// goyacc 生成パーサ (parser.go) と Lexer をつなぐドライバ。
// Ruby版 Parser#parse ( [ast, pos_info] を返し、Racc::ParseError を CompileError に変換 ) 相当。

//go:generate goyacc -o parser.go -p fc parser.y

import (
	"fmt"
	"strings"

	"github.com/haramako/fc/internal/syntax"
)

type parserLexer struct {
	lex     *Lexer
	result  []any
	posInfo *OMap
	err     *CompileError
}

// lexPanic はレキサエラー (Ruby の raise "invalid token at N" 相当) を伝搬するための panic 値。
type lexPanic struct{ err error }

func (p *parserLexer) Lex(lval *fcSymType) int {
	tok, val, err := p.lex.NextToken()
	if err != nil {
		panic(lexPanic{err})
	}
	lval.n = val
	return tok
}

func (p *parserLexer) Error(s string) {
	if p.err == nil {
		// racc は "parse error on value ..." 形式 (errors.fc が /parse error/ で照合する)
		msg := strings.Replace(s, "syntax error", "parse error", 1)
		p.err = &CompileError{Msg: msg, Filename: p.lex.filename, LineNo: p.lex.lineNo}
	}
}

// info は statement_i の reduce 時に pos_info を記録する (Ruby の Parser#info 相当)。
// キーはASTノードの構造的等値 (Ruby の Hash と同じく、後勝ちで挿入位置は維持)。
func (p *parserLexer) info(node any) {
	p.posInfo.Set(node, []any{p.lex.filename, p.lex.lineNo})
}

// ParseSrc はソースをパースして (ast, pos_info) を返す。
// R1-b 以降: 新パーサ (internal/syntax) で構文木を作り、Lower で旧 AST に変換する。
// 旧 goyacc パーサ (parseSrcOld) は lower_test.go の差分テストのためだけに残している (R1-c で削除)。
func ParseSrc(src []byte, filename string) (ast []any, posInfo *OMap, err error) {
	f, perr := syntax.Parse(src, filename)
	if perr != nil {
		se := perr.(*syntax.Error)
		return nil, nil, &CompileError{Msg: se.Msg, Filename: se.Filename, LineNo: se.Pos.Line}
	}
	ast, posInfo = Lower(f)
	return ast, posInfo, nil
}

// parseSrcOld は旧 goyacc パーサによるパース (差分テスト用)。
func parseSrcOld(src []byte, filename string) (ast []any, posInfo *OMap, err error) {
	pl := &parserLexer{lex: NewLexer(src, filename), posInfo: NewOMap()}
	defer func() {
		if r := recover(); r != nil {
			if lp, ok := r.(lexPanic); ok {
				err = lp.err
				return
			}
			panic(r)
		}
	}()
	rc := fcParse(pl)
	if pl.err != nil {
		return nil, nil, pl.err
	}
	if rc != 0 {
		return nil, nil, &CompileError{Msg: fmt.Sprintf("parse failed (%d)", rc), Filename: filename}
	}
	return pl.result, pl.posInfo, nil
}
