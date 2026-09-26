package syntax

// フォーマッタ (doc/archive/go_evolution_plan.md F-fmt)。構文木をコメント付きで正規形に印字する。
//
// 方針は gofmt と同じ:
//   - トークンの並びは変えない (括弧もそのまま)。変えるのは空白・改行・インデントだけ。
//     したがって parse(Format(x)) は parse(x) と (位置を除いて) 同じ構文木になる
//   - コメントは元の位置関係 (前の行・同じ行の末尾・ブロック末尾) を保って出力する
//   - 文の間の空行は 1 行までに詰める。ブロック先頭の空行は消す
//   - 配列リテラルと呼び出し引数は、元ソースで改行されていた要素の前で改行する (表データ用)
//   - 長い行は折り返さない
//
// スタイル (fc の既存コードの多数派に合わせた):
//   - インデントはタブ
//   - 関数本体の `{` は次の行、制御文の `{` は同じ行 (`if (cond) {`)
//   - `name:type`、`a + b`、`a.b`、`f(a, b)`、`options(bank: -1)`

import (
	"bytes"
	"fmt"
	"strings"
)

// Format は fc ソースを正規形に整形する。構文エラーは *Error で返す。
func Format(src []byte, filename string) ([]byte, error) {
	src = normalizeNewlines(src)
	f, err := Parse(src, filename)
	if err != nil {
		return nil, err
	}
	return Print(f), nil
}

// normalizeNewlines は改行の直前の CR をすべて落とす (CRLF → LF)。`\r\n` を 1 回置き換えるだけだと
// `\r\r\n` が `\r\n` として残り、整形が冪等でなくなる (fuzz で発覚)。
func normalizeNewlines(src []byte) []byte {
	if !bytes.Contains(src, []byte("\r\n")) {
		return src
	}
	out := make([]byte, 0, len(src))
	crs := 0 // 保留中の CR の数 (直後が LF なら捨てる)
	for _, c := range src {
		switch c {
		case '\r':
			crs++
			continue
		case '\n':
			crs = 0
		}
		for ; crs > 0; crs-- {
			out = append(out, '\r')
		}
		out = append(out, c)
	}
	for ; crs > 0; crs-- {
		out = append(out, '\r')
	}
	return out
}

// Print は構文木を整形して出力する。f.Comments の位置を使ってコメントを差し込む。
func Print(f *File) []byte {
	p := &printer{comments: f.Comments, version: f.Version}
	if f.Pragma != "" {
		// `#fc N` は正規化して 1 行目に (直後の空行を保つ)。無いソースには足さない
		p.write(fmt.Sprintf("#fc %d", f.Version))
		p.lastLine = 1
		p.blankOK = true
		p.newline()
	}
	p.stmtList(f.Stmts, true)
	p.flushComments(Pos{Offset: int(^uint(0) >> 1)})
	if p.buf.Len() > 0 {
		p.buf.WriteByte('\n')
	}
	return p.buf.Bytes()
}

type printer struct {
	version  int // ソースの文法バージョン (fc 3 は @sizeof / @incbin で出す)
	buf      bytes.Buffer
	comments []Comment
	ci       int // 次に出力するコメント

	indent    int
	pending   int  // 次のトークンの前に出す改行数 (0 なら同じ行)
	needSpace bool // 次のトークンの前に空白を出す
	lastLine  int  // 最後に出力したトークン/コメントのソース行 (0 なら未出力)
	lastEnd   Pos  // 最後に出力したトークン/コメントの終端
	blankOK   bool // 空行を許すか (文と文の間だけ真。ブロック先頭では偽)
	lastByte  byte

	afterComment bool // 直前に出力したのがコメント
}

// ---------------------------------------------------------------
// 低レベル出力
// ---------------------------------------------------------------

// tokAt は位置 pos のトークン s を出力する。先に pos より前のコメントを吐く。
func (p *printer) tokAt(pos Pos, s string) {
	if pos.IsValid() {
		p.flushComments(pos)
		if p.afterComment && pos.Line > p.lastLine {
			// 独立行のブロックコメントの後、トークンが次の行にあるなら行を分ける
			p.newline()
		}
		if p.pending > 0 && p.blankOK && p.lastLine > 0 && pos.Line-p.lastLine > 1 {
			p.pending = 2
		}
	}
	p.afterComment = false
	p.write(s)
	if pos.IsValid() {
		p.lastLine = pos.Line + strings.Count(s, "\n")
		p.lastEnd = after(pos, len(s))
	}
	p.blankOK = false
}

// tok は位置を持たない (または位置が重要でない) トークンを出力する。
func (p *printer) tok(s string) {
	p.write(s)
}

func (p *printer) write(s string) {
	if s == "" {
		return
	}
	if p.pending > 0 {
		if p.buf.Len() > 0 {
			for i := 0; i < p.pending; i++ {
				p.buf.WriteByte('\n')
			}
			for i := 0; i < p.indent; i++ {
				p.buf.WriteByte('\t')
			}
		}
		p.pending = 0
		p.needSpace = false
	} else if p.needSpace || mergesWithPrev(p.lastByte, s[0]) {
		p.buf.WriteByte(' ')
		p.needSpace = false
	}
	p.buf.WriteString(s)
	p.lastByte = s[len(s)-1]
}

// mergesWithPrev は直前の文字と次のトークン先頭を続けて書くと別のトークンに
// 読まれてしまう組み合わせ (`<` `<` → `<<` など) かを返す。
func mergesWithPrev(prev, next byte) bool {
	switch string([]byte{prev, next}) {
	case "<<", ">>", "&&", "||", "==", "<=", ">=", "!=", "+=", "-=", "->", "//", "/*", "*/", "++", "--":
		return true
	}
	return false
}

func (p *printer) space() { p.needSpace = true }

// newline は次のトークンを新しい行に出す。
func (p *printer) newline() {
	if p.pending < 1 {
		p.pending = 1
	}
}

// flushComments は before より前にあるコメントをすべて出力する。
func (p *printer) flushComments(before Pos) {
	for p.ci < len(p.comments) && p.comments[p.ci].Pos.Offset < before.Offset {
		c := p.comments[p.ci]
		p.ci++
		if c.IsLine() {
			// 行コメント末尾の単独の CR は、直後に出す改行と合わせて CRLF になり次の整形で消える (冪等でなくなる。fuzz で発覚)
			c.Text = strings.TrimRight(c.Text, "\r")
		}
		if p.lastLine > 0 && c.Pos.Line == p.lastLine && p.buf.Len() > 0 {
			// 直前のトークンと同じ行 → 行末コメント。保留中の改行より前に出す
			saved := p.pending
			p.pending = 0
			p.needSpace = true
			p.write(c.Text)
			p.pending = saved
		} else {
			p.newline()
			if p.blankOK && p.lastLine > 0 && c.Pos.Line-p.lastLine > 1 {
				p.pending = 2
			}
			p.write(c.Text)
			p.blankOK = true // コメントの後の文の前には空行を置ける
		}
		p.lastLine = c.End.Line
		p.lastEnd = c.End
		p.afterComment = true
		if c.IsLine() {
			p.newline()
		} else {
			p.needSpace = true
		}
	}
}

// ---------------------------------------------------------------
// 文
// ---------------------------------------------------------------

// stmtList は文の並びを 1 文 1 行で出力する。top なら先頭にも空行を許す。
func (p *printer) stmtList(stmts []Stmt, top bool) {
	for i, s := range stmts {
		if i > 0 || top {
			p.newline()
			if i > 0 {
				p.blankOK = true // 先頭の文の前は呼び出し側の設定に従う (ブロック先頭は偽、プラグマの後は真)
			}
		}
		p.stmt(s)
	}
}

func (p *printer) block(b *Block) {
	if p.isEmptyBlock(b) {
		p.tokAt(b.Lbrace, "{")
		p.tokAt(b.Rbrace, "}")
		return
	}
	p.tokAt(b.Lbrace, "{")
	p.indent++
	p.blankOK = false
	p.stmtList(b.Stmts, true)
	p.flushComments(b.Rbrace) // ブロック末尾のコメントは中のインデントで出す
	p.indent--
	p.newline()
	p.blankOK = false
	p.tokAt(b.Rbrace, "}")
}

// isEmptyBlock は文もコメントも含まないブロックか (`{}` と 1 行で出す)。
func (p *printer) isEmptyBlock(b *Block) bool {
	if len(b.Stmts) > 0 {
		return false
	}
	for i := p.ci; i < len(p.comments); i++ {
		c := p.comments[i]
		if c.Pos.Offset > b.Rbrace.Offset {
			break
		}
		if c.Pos.Offset > b.Lbrace.Offset {
			return false
		}
	}
	return true
}

// body は制御文の本体。ブロックなら同じ行に `{`、単文なら次の行にインデントして出す。
func (p *printer) body(s Stmt) {
	if b, ok := s.(*Block); ok {
		p.space()
		p.block(b)
		return
	}
	p.indent++
	p.newline()
	p.blankOK = false
	p.stmt(s)
	p.indent--
}

func (p *printer) stmt(s Stmt) {
	switch s := s.(type) {
	case *VarDecl:
		if s.PublicPos.IsValid() {
			p.tokAt(s.PublicPos, "public")
			p.space()
		}
		if s.Alias {
			p.tokAt(s.Keyword, "alias")
		} else if s.Const {
			p.tokAt(s.Keyword, "const")
		} else {
			p.tokAt(s.Keyword, "var")
		}
		p.space()
		for i, sp := range s.Specs {
			if i > 0 {
				p.tok(",")
				p.space()
			}
			p.varSpec(sp)
		}
		p.tokAt(s.Semi, ";")

	case *FuncDecl:
		if s.PublicPos.IsValid() {
			p.tokAt(s.PublicPos, "public")
			p.space()
		}
		p.tokAt(s.Keyword, "function")
		p.space()
		p.ident(s.Name)
		p.tok("(")
		for i, sp := range s.Params {
			if i > 0 {
				p.tok(",")
				p.space()
			}
			p.varSpec(sp)
		}
		p.tok(")")
		p.tok(":")
		p.typeExpr(s.Result)
		if s.Options != nil {
			p.space()
			p.options(s.Options)
		}
		switch {
		case s.Body == nil:
			p.tokAt(s.Semi, ";")
		case p.isEmptyBlock(s.Body):
			// 空の本体は同じ行に `{}`
			p.space()
			p.block(s.Body)
		default:
			p.newline()
			p.block(s.Body)
		}

	case *IfStmt:
		p.ifStmt(s)

	case *StaticIfStmt:
		p.staticIf(s)

	case *LoopStmt:
		p.tokAt(s.Loop, "loop")
		if s.Rparen.IsValid() {
			// v1: loop()
			p.tok("(")
			p.tokAt(s.Rparen, ")")
		}
		p.body(s.Body)

	case *LabeledStmt:
		p.ident(s.Label)
		p.tokAt(s.Colon, ":")
		p.space()
		p.stmt(s.Stmt)

	case *WhileStmt:
		p.tokAt(s.While, "while")
		p.space()
		p.tok("(")
		p.expr(s.Cond)
		p.tokAt(s.Rparen, ")")
		p.body(s.Body)

	case *ForStmt:
		p.tokAt(s.For, "for")
		p.space()
		p.tok("(")
		if s.IsV1() {
			p.ident(s.Var)
			p.tok(",")
			p.space()
			p.expr(s.From)
			p.tok(",")
			p.space()
			p.expr(s.To)
		} else {
			// for (init; cond; step)。省略部は空 (`for (;;)`)
			if s.Init != nil {
				p.simpleStmt(s.Init)
			}
			p.tok(";")
			if s.Cond != nil {
				p.space()
				p.expr(s.Cond)
			}
			p.tok(";")
			if s.Step != nil {
				p.space()
				p.simpleStmt(s.Step)
			}
		}
		p.tokAt(s.Rparen, ")")
		p.space()
		p.block(s.Body)

	case *IncDecStmt:
		p.incDec(s)
		p.tokAt(s.Semi, ";")

	case *BreakStmt:
		p.tokAt(s.Keyword, "break")
		if s.Label != nil {
			p.space()
			p.ident(s.Label)
		}
		p.tokAt(s.Semi, ";")

	case *FallthroughStmt:
		p.tokAt(s.Keyword, "fallthrough")
		p.tokAt(s.Semi, ";")

	case *ContinueStmt:
		p.tokAt(s.Keyword, "continue")
		if s.Label != nil {
			p.space()
			p.ident(s.Label)
		}
		p.tokAt(s.Semi, ";")

	case *ReturnStmt:
		p.tokAt(s.Return, "return")
		if s.Value != nil {
			p.space()
			p.expr(s.Value)
		}
		p.tokAt(s.Semi, ";")

	case *SwitchStmt:
		p.tokAt(s.Switch, "switch")
		p.space()
		p.tok("(")
		p.expr(s.Tag)
		p.tok(")")
		p.space()
		p.tokAt(s.Lbrace, "{")
		// case は switch と同じインデント (Go と同じ。fc の既存コードも多数派はこれ)
		for _, c := range s.Cases {
			p.newline()
			p.blankOK = true
			p.tokAt(c.Case, "case")
			p.space()
			for i, v := range c.Values {
				if i > 0 {
					p.tok(",")
					p.space()
				}
				p.expr(v)
			}
			p.tokAt(c.Colon, ":")
			p.indent++
			p.stmtList(c.Body, true)
			p.indent--
		}
		if s.Default != nil {
			p.newline()
			p.blankOK = true
			p.tokAt(s.Default.Default, "default")
			p.tokAt(s.Default.Colon, ":")
			p.indent++
			p.stmtList(s.Default.Body, true)
			p.indent--
		}
		p.indent++
		p.flushComments(s.Rbrace)
		p.indent--
		p.newline()
		p.blankOK = false
		p.tokAt(s.Rbrace, "}")

	case *ExprStmt:
		p.expr(s.X)
		p.tokAt(s.Semi, ";")

	case *OptionsStmt:
		p.options(s.Options)
		p.tokAt(s.Semi, ";")

	case *UseDecl:
		if s.PublicPos.IsValid() {
			p.tokAt(s.PublicPos, "public")
			p.space()
		}
		p.tokAt(s.Use, "use")
		p.space()
		if s.FromAll {
			p.tokAt(s.Star, "*")
			p.space()
			p.tok("from")
			p.space()
		} else if len(s.Names) > 0 {
			for i, id := range s.Names {
				if i > 0 {
					p.tok(",")
					p.space()
				}
				p.ident(id)
			}
			p.space()
			p.tok("from")
			p.space()
		}
		p.ident(s.Module)
		if s.As != nil {
			p.space()
			p.tok("as")
			p.space()
			p.ident(s.As)
		}
		p.tokAt(s.Semi, ";")

	case *StructDecl:
		if s.PublicPos.IsValid() {
			p.tokAt(s.PublicPos, "public")
			p.space()
		}
		p.tokAt(s.Keyword, "struct")
		p.space()
		p.ident(s.Name)
		p.space()
		p.tokAt(s.Lbrace, "{")
		p.indent++
		for _, f := range s.Fields {
			p.newline()
			p.blankOK = true
			p.ident(f.Name)
			p.tok(":")
			p.typeExpr(f.Type)
			p.tokAt(f.Semi, ";")
		}
		p.flushComments(s.Rbrace)
		p.indent--
		p.newline()
		p.blankOK = false
		p.tokAt(s.Rbrace, "}")

	case *EnumDecl:
		// enum Name:u8 {  (メンバーは 1 行に 1 つ、末尾のカンマ付き)
		if s.PublicPos.IsValid() {
			p.tokAt(s.PublicPos, "public")
			p.space()
		}
		p.tokAt(s.Keyword, "enum")
		p.space()
		p.ident(s.Name)
		if s.Base != nil {
			p.tok(":")
			p.typeExpr(s.Base)
		}
		p.space()
		p.tokAt(s.Lbrace, "{")
		p.indent++
		for _, m := range s.Members {
			p.newline()
			p.blankOK = true
			p.ident(m.Name)
			if m.Value != nil {
				p.space()
				p.tok("=")
				p.space()
				p.expr(m.Value)
			}
			p.tok(",")
		}
		p.flushComments(s.Rbrace)
		p.indent--
		p.newline()
		p.blankOK = false
		p.tokAt(s.Rbrace, "}")

	case *SoaDecl:
		if s.PublicPos.IsValid() {
			p.tokAt(s.PublicPos, "public")
			p.space()
		}
		p.tokAt(s.Keyword, "soa")
		p.space()
		if s.Const {
			p.tok("const")
			p.space()
		}
		p.ident(s.Name)
		p.tok(":")
		p.typeExpr(s.Type)
		if s.Init != nil {
			p.space()
			p.tok("=")
			p.space()
			p.expr(s.Init)
		}
		if s.Options != nil {
			p.space()
			p.options(s.Options)
		}
		p.tokAt(s.Semi, ";")

	case *IncludeDecl:
		if s.At {
			// fc 3: @include("path", key: value, ...)
			p.tokAt(s.Include, "@include")
			p.tok("(")
			p.tokAt(s.Path.ValuePos, s.Path.Text)
			if s.Options != nil {
				p.optionEntries(s.Options, true)
			}
			p.tokAt(s.Rparen, ")")
			p.tokAt(s.Semi, ";")
			break
		}
		p.tokAt(s.Include, "include")
		if s.Kind != nil {
			p.space()
			p.ident(s.Kind)
		}
		p.tok("(")
		p.tokAt(s.Path.ValuePos, s.Path.Text)
		p.tokAt(s.Rparen, ")")
		if s.Options != nil {
			p.space()
			p.options(s.Options)
		}
		p.tokAt(s.Semi, ";")

	case *ScopeLabel:
		// ラベルはインデントを戻して出す (トップレベル専用の構文)
		saved := p.indent
		p.indent = 0
		if s.Public {
			p.tokAt(s.Keyword, "public")
		} else {
			p.tokAt(s.Keyword, "private")
		}
		p.tokAt(s.Colon, ":")
		p.indent = saved

	case *PlacementBlock:
		if s.Keyword == nil {
			// fc 3: @(...) { ... }
			p.options(s.Options)
			p.space()
			p.block(s.Body)
			break
		}
		p.ident(s.Keyword)
		p.space()
		p.block(s.Body)
		p.space()
		p.options(s.Options)
		p.tokAt(s.Semi, ";")
	case *Block:
		p.block(s)

	case *EmptyStmt:
		p.tokAt(s.Semi, ";")

	default:
		panic("unknown statement")
	}
}

func (p *printer) ifStmt(s *IfStmt) {
	if s.IsElsif {
		p.tokAt(s.If, "elsif")
	} else {
		p.tokAt(s.If, "if")
	}
	p.space()
	p.tokAt(s.Lparen, "(")
	p.expr(s.Cond)
	p.tokAt(s.Rparen, ")")
	p.body(s.Then)
	if s.Else == nil {
		return
	}
	// `} else` / `} elsif` は同じ行。Then が単文なら次の行
	if _, ok := s.Then.(*Block); ok {
		p.space()
	} else {
		p.newline()
	}
	if e, ok := s.Else.(*IfStmt); ok && e.IsElsif {
		p.ifStmt(e)
		return
	}
	p.tok("else")
	p.body(s.Else)
}

// staticIf は fc 3 の `@if (...) { ... } else @if (...) { ... } else { ... }` (`} else` は同じ行)。
func (p *printer) staticIf(s *StaticIfStmt) {
	p.tokAt(s.At, "@if")
	p.space()
	p.tokAt(s.Lparen, "(")
	p.expr(s.Cond)
	p.tokAt(s.Rparen, ")")
	p.space()
	p.block(s.Then)
	if s.Else == nil {
		return
	}
	p.space()
	p.tokAt(s.ElsePos, "else")
	p.space()
	if e, ok := s.Else.(*StaticIfStmt); ok {
		p.staticIf(e)
		return
	}
	p.block(s.Else.(*Block))
}

// simpleStmt は for の init / step (`;` を持たない文)。
func (p *printer) simpleStmt(s Stmt) {
	switch s := s.(type) {
	case *VarDecl:
		p.tokAt(s.Keyword, "var")
		p.space()
		for i, sp := range s.Specs {
			if i > 0 {
				p.tok(",")
				p.space()
			}
			p.varSpec(sp)
		}
	case *ExprStmt:
		p.expr(s.X)
	case *IncDecStmt:
		p.incDec(s)
	default:
		panic("unknown simple statement")
	}
}

func (p *printer) incDec(s *IncDecStmt) {
	if s.Prefix {
		p.tokAt(s.OpPos, s.Op.String())
		p.expr(s.X)
	} else {
		p.expr(s.X)
		p.tokAt(s.OpPos, s.Op.String())
	}
}

func (p *printer) varSpec(sp *VarSpec) {
	p.ident(sp.Name)
	if sp.Type != nil {
		p.tok(":")
		p.typeExpr(sp.Type)
	}
	if sp.Init != nil {
		p.space()
		p.tok("=")
		p.space()
		p.expr(sp.Init)
	}
	if sp.Options != nil {
		p.space()
		p.options(sp.Options)
	}
}

func (p *printer) options(o *Options) {
	if o.At {
		p.tokAt(o.Keyword, "@")
	} else {
		p.tokAt(o.Keyword, "options")
	}
	p.tok("(")
	p.optionEntries(o, false)
	p.tokAt(o.Rparen, ")")
}

// optionEntries は `key: value, ...` を出す (lead なら最初の要素の前にも `, `)。
func (p *printer) optionEntries(o *Options, lead bool) {
	for i, e := range o.Entries {
		if i > 0 || lead {
			p.tok(",")
			p.space()
		}
		p.ident(e.Key)
		if e.Bare {
			continue // fc 3 の `@(inline)`
		}
		p.tok(":")
		p.space()
		p.expr(e.Value)
	}
}

// ---------------------------------------------------------------
// 式
// ---------------------------------------------------------------

// at は組み込みの綴り (fc 3 は `@` を付ける)。
func (p *printer) at(name string) string {
	if p.version >= Version3 {
		return "@" + name
	}
	return name
}

func (p *printer) ident(id *Ident) {
	p.tokAt(id.NamePos, id.Name)
}

func (p *printer) expr(e Expr) {
	switch e := e.(type) {
	case *Ident:
		p.ident(e)
	case *IntLit:
		p.tokAt(e.ValuePos, e.Text)
	case *BoolLit:
		if e.Value {
			p.tokAt(e.ValuePos, "true")
		} else {
			p.tokAt(e.ValuePos, "false")
		}
	case *NullLit:
		p.tokAt(e.ValuePos, "null")
	case *StringLit:
		p.tokAt(e.ValuePos, e.Text)
	case *ParenExpr:
		p.tokAt(e.Lparen, "(")
		p.expr(e.X)
		p.tokAt(e.Rparen, ")")
	case *BinaryExpr:
		p.expr(e.X)
		if e.Op == Dot {
			p.tokAt(e.OpPos, ".")
		} else {
			p.space()
			p.tokAt(e.OpPos, e.Op.String())
			p.space()
		}
		p.expr(e.Y)
	case *AssignExpr:
		p.expr(e.Lhs)
		p.space()
		p.tokAt(e.OpPos, e.Op.String())
		p.space()
		p.expr(e.Rhs)
	case *UnaryExpr:
		p.tokAt(e.OpPos, e.Op.String())
		p.expr(e.X)
	case *CastExpr:
		switch e.Kind {
		case CastAs:
			p.expr(e.X)
			p.space()
			p.tokAt(e.As, "as")
			p.space()
			p.typeExpr(e.Type)
		case CastBit:
			if e.Comma.IsValid() {
				// fc 3: @bitcast(T, x)
				p.tokAt(e.Bitcast, "@bitcast")
				p.tokAt(e.Lparen, "(")
				p.typeExpr(e.Type)
				p.tokAt(e.Comma, ",")
				p.space()
				p.expr(e.X)
				p.tokAt(e.Rparen, ")")
				break
			}
			p.tokAt(e.Bitcast, "bitcast")
			p.tokAt(e.Lt, "<")
			p.typeExpr(e.Type)
			p.tokAt(e.Gt, ">")
			p.tokAt(e.Lparen, "(")
			p.expr(e.X)
			p.tokAt(e.Rparen, ")")
		default:
			p.tokAt(e.Lt, "<")
			p.typeExpr(e.Type)
			p.tokAt(e.Gt, ">")
			p.expr(e.X)
		}
	case *CallExpr:
		p.expr(e.Fun)
		p.tokAt(e.Lparen, "(")
		p.exprList(e.Args, e.Comma, e.Rparen)
		p.tokAt(e.Rparen, ")")
		if e.Block != nil {
			p.space()
			p.block(e.Block)
		}
	case *IndexExpr:
		p.expr(e.X)
		p.tokAt(e.Lbrack, "[")
		p.expr(e.Index)
		p.tokAt(e.Rbrack, "]")
	case *ArrayLit:
		p.tokAt(e.Lbrack, "[")
		p.exprList(e.Elems, e.Comma, e.Rbrack)
		p.tokAt(e.Rbrack, "]")
	case *StructLit:
		if e.Type != nil {
			p.typeExpr(e.Type)
		}
		p.tokAt(e.Lbrace, "{")
		p.fieldInits(e.Fields, e.Rbrace)
		p.tokAt(e.Rbrace, "}")
	case *EnumShortExpr:
		p.tokAt(e.Dot, ".")
		p.ident(e.Name)
	case *SliceExpr:
		p.expr(e.X)
		p.tokAt(e.Lbrack, "[")
		if e.Lo != nil {
			p.expr(e.Lo)
		}
		p.tokAt(e.DotDot, "..")
		if e.Hi != nil {
			p.expr(e.Hi)
		}
		p.tokAt(e.Rbrack, "]")
	case *SizeofExpr:
		p.tokAt(e.Sizeof, p.at("sizeof"))
		p.tokAt(e.Lparen, "(")
		p.typeExpr(e.Type)
		p.tokAt(e.Rparen, ")")
	case *IncbinExpr:
		p.tokAt(e.Incbin, p.at("incbin"))
		p.tok("(")
		p.tokAt(e.Path.ValuePos, e.Path.Text)
		p.tokAt(e.Rparen, ")")
	case *LambdaExpr:
		p.tokAt(e.Arrow, "->")
		p.typeExpr(e.Type)
		if e.Body != nil {
			p.space()
			p.block(e.Body)
		} else {
			p.tokAt(e.Semi, ";")
		}
	default:
		panic("unknown expression")
	}
}

// fieldInits は struct リテラルの項目列 (exprList と同じ改行規則。末尾カンマは複数行のときだけ)。
func (p *printer) fieldInits(list []*FieldInit, close Pos) {
	if len(list) == 0 {
		return
	}
	multiline := false
	prevLine := p.lastLine
	p.indent++
	for i, f := range list {
		if i > 0 {
			p.tok(",")
		}
		if f.Pos().Line > prevLine {
			p.newline()
			multiline = true
		} else if i > 0 {
			p.space()
		}
		if f.Key != nil {
			p.ident(f.Key)
			p.tokAt(f.Colon, ":")
			p.space()
		}
		p.expr(f.Value)
		prevLine = f.End().Line
	}
	p.indent--
	if multiline && close.Line > prevLine {
		p.tok(",")
		p.newline()
	}
}

// exprList はカンマ区切りの式列。元ソースで改行されていた要素の前では改行し、
// 閉じ括弧 (close) が独立した行にあればその前でも改行する (表データの体裁を保つ)。
// 末尾のカンマ (comma が有効) は、閉じ括弧が独立した行にあるときだけ残す (gofmt と同じ)。
func (p *printer) exprList(list []Expr, comma, close Pos) {
	if len(list) == 0 {
		return
	}
	multiline := false
	prevLine := p.lastLine
	p.indent++
	for i, e := range list {
		if i > 0 {
			p.tok(",")
		}
		if e.Pos().Line > prevLine {
			p.newline()
			multiline = true
		} else if i > 0 {
			p.space()
		}
		p.expr(e)
		prevLine = e.End().Line
	}
	p.indent--
	if multiline && close.Line > prevLine {
		if comma.IsValid() {
			p.tokAt(comma, ",")
		}
		p.newline()
	}
}

// ---------------------------------------------------------------
// 型
// ---------------------------------------------------------------

// typeExpr は型を印字する (前置形: `[4]*int`, `fn(int):void`)。
func (p *printer) typeExpr(t TypeExpr) {
	p.typeExprV2(t)
}

func (p *printer) typeExprV2(t TypeExpr) {
	switch t := t.(type) {
	case *NamedType:
		if t.Module != nil {
			p.ident(t.Module)
			p.tok(".")
		}
		p.ident(t.Name)
	case *ArrayType:
		p.tokAt(t.Lbrack, "[")
		if t.Len != nil {
			p.expr(t.Len)
		}
		if t.Infer.IsValid() {
			p.tokAt(t.Infer, "?")
		}
		if t.LenType != nil {
			p.tokAt(t.Colon, ":")
			p.typeExprV2(t.LenType)
		}
		p.tokAt(t.Rbrack, "]")
		if t.Const.IsValid() {
			p.tokAt(t.Const, "const")
			p.space()
		}
		p.typeExprV2(t.Elem)
	case *PointerType:
		p.tokAt(t.Star, "*")
		if t.Const.IsValid() {
			p.tokAt(t.Const, "const")
			p.space()
		}
		p.typeExprV2(t.Elem)
	case *FuncType:
		if t.Far {
			p.tokAt(t.Fn, "farfn")
		} else {
			p.tokAt(t.Fn, "fn")
		}
		p.tokAt(t.Lparen, "(")
		p.params(t.Params)
		p.tokAt(t.Rparen, ")")
		p.tok(":")
		p.typeExprV2(t.Result)
	default:
		panic("unknown type")
	}
}

func (p *printer) params(params []*Param) {
	for i, prm := range params {
		if i > 0 {
			p.tok(",")
			p.space()
		}
		if prm.Name != nil {
			p.ident(prm.Name)
			p.tok(":")
		}
		p.typeExpr(prm.Type)
	}
}
