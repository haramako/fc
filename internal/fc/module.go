package fc

// lib/fc/base.rb の Module / Lambda の移植。

// Def は Module#defs / Lambda#defs の1エントリ ( [symbol, kind, type, val] )。
type Def struct {
	Sym  any  // シンボル名 (Sym)
	Kind Sym  // :equ, :bss, :block, :code
	Type *Type
	Val  any // Sym / int / string / *OMap{segment:} / []any(Value列) / *Lambda / nil
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

type Lambda struct {
	Id        any // Sym
	Args      []any // 最初は []any{id, *Type} のペア、compile_lambda 後は *Value
	Type      *Type
	Opt       *OMap
	Ast       any // 関数本体のAST ([]any) または nil (extern)
	Ops       [][]any
	Vars      []*Value
	Bank      int
	Result    *Value
	Defs      []*Def
	Asm       []string
	FrameSize int
}

func NewLambda(id any, args []any, baseType *Type, opt *OMap, ast any) *Lambda {
	if opt == nil {
		opt = NewOMap()
	}
	argTypes := make([]any, len(args))
	for i, a := range args {
		argTypes[i] = a.([]any)[1]
	}
	typ := TypeOf([]any{Sym("lambda"), argTypes, baseType, opt.GetOr(Sym("fastcall"))})
	return &Lambda{Id: id, Args: args, Type: typ, Opt: opt, Ast: ast, Bank: 0}
}

// String は Ruby の "<Lambda:#{@id} #{@type}>" 相当 (エラーメッセージで使用)。
func (l *Lambda) String() string {
	return "<Lambda:" + ToS(l.Id) + " " + l.Type.String() + ">"
}
