package fc

// lib/fc/base.rb の Module / Lambda の移植。

import "github.com/haramako/fc/internal/syntax"

// Def は Module#defs / Lambda#defs の1エントリ ( [symbol, kind, type, val] )。
type Def struct {
	Sym  any // シンボル名 (Sym)
	Kind DefKind
	Type *Type
	Val  any // Sym / int / string / *OMap{segment:} / []Operand(配列要素) / *Lambda / nil
}

type Module struct {
	Vars           []*Value
	Lambdas        []*Lambda
	Options        *OMap
	IncludeChrs    []string
	IncludeAsms    []string
	IncludeHeaders []string
	Modules        *OMap // key: Sym, val: *Module
	Scope          *Scope
	CurrentScope   Sym // :public / :private
	Id             Sym
	Path           string
	FromFcm        bool
	Depends        []string
	Defs           []*Def
}

func NewModule(globalScope *Scope) *Module {
	return &Module{
		Options:      NewOMap(),
		Modules:      NewOMap(),
		Scope:        NewScope(globalScope),
		CurrentScope: "public",
	}
}

// Param は関数の仮引数 (名前と型)。
type Param struct {
	Name Sym
	Type *Type
}

type Lambda struct {
	Id        any     // Sym
	Params    []Param // 仮引数の宣言
	Args      []*Value // 仮引数の変数 (compileLambda で Params から作られる)
	Type      *Type
	Opt       *OMap
	Body      *syntax.Block // 関数本体。nil なら extern
	Ops       []*Op // nil 要素は最適化で削除された命令
	Vars      []*Value
	Bank      int
	Result    *Value
	Defs      []*Def
	Asm       []string
	FrameSize int
}

func NewLambda(id any, params []Param, baseType *Type, opt *OMap, body *syntax.Block) *Lambda {
	if opt == nil {
		opt = NewOMap()
	}
	argTypes := make([]any, len(params))
	for i, p := range params {
		argTypes[i] = p.Type
	}
	typ := TypeOf([]any{Sym("lambda"), argTypes, baseType, opt.GetOr(Sym("fastcall"))})
	return &Lambda{Id: id, Params: params, Type: typ, Opt: opt, Body: body, Bank: 0}
}

// String は Ruby の "<Lambda:#{@id} #{@type}>" 相当 (エラーメッセージで使用)。
func (l *Lambda) String() string {
	return "<Lambda:" + ToS(l.Id) + " " + l.Type.String() + ">"
}
