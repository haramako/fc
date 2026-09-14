/*
 * parser.y — fc 文法の goyacc 定義。型付き構文木 (ast.go) を生成する。
 *
 * v1 と v2 (doc/v2_grammar.md) のスーパーセットを 1 つの文法で受理し、
 * バージョンごとの受理範囲は parse.go の checkVersion で絞る。
 * v1 部分の規則と優先順位は Ruby racc 版の parser.y と 1:1 で対応させている
 * (到達不能だった `id_list: tID ...` 規則だけ削除)。v2 で足した規則には「v2」と注記する。
 *
 * 生成: go generate ./internal/syntax  ( goyacc -o parser.go -p yy parser.y )
 */

%{
package syntax
%}

%union {
	tok     Token
	stmt    Stmt
	stmts   []Stmt
	expr    Expr
	exprs   []Expr
	alist   argList
	typ     TypeExpr
	spec    *VarSpec
	specs   []*VarSpec
	opts    *Options
	optents []*OptionEntry
	params  []*Param
	param   *Param
	cases   []*CaseClause
	kase    *CaseClause
	deflt   *DefaultClause
	block   *Block
	fbody   funcBody
	ident   *Ident
	idents  []*Ident
	use     *UseDecl
	fields  []*FieldDecl
	field   *FieldDecl
	inits   []*FieldInit
	init    *FieldInit
	slit    *StructLit
	optTok  *Token
}

%token <tok> NUMBER IDENT STRING
%token <tok> kINCLUDE kFUNCTION kCONST kVAR kOPTIONS kIF kELSE kELSIF kLOOP kWHILE kFOR kRETURN kBREAK kCONTINUE kINCBIN kSWITCH kCASE kDEFAULT kUSE kAS kFROM kPUBLIC kPRIVATE kFN kBITCAST kSTRUCT kSIZEOF kSOA
%token <tok> LEQ GEQ EQEQ ADDEQ SUBEQ NEQ ARROW LSHIFT RSHIFT ANDAND OROR INCR DECR
%token <tok> '(' ')' '{' '}' ';' ':' '<' '>' '[' ']' '+' '-' '*' '/' '%' '&' '|' '^' '=' ',' '.' '!'

%type <stmts>   program opt_statement_list statement_list
%type <stmt>    statement_i statement else_block opt_for_init opt_for_step simple_stmt incdec
%type <optTok>  opt_scope
%type <use>     use_target
%type <fields>  field_decl_list
%type <field>   field_decl
%type <inits>   field_init_list field_inits
%type <init>    field_init
%type <slit>    struct_lit anon_struct_lit
%type <expr>    lit_elem
%type <exprs>   lit_elem_list
%type <ident>   opt_as opt_ident
%type <idents>  ident_list
%type <deflt>   opt_default_block
%type <cases>   switch_block
%type <kase>    case_block
%type <block>   block opt_block
%type <fbody>   function_block
%type <expr>    opt_exp exp
%type <exprs>   exp_list
%type <alist>   arg_list
%type <opts>    opt_options options
%type <optents> option_list option_list_sub option
%type <specs>   opt_var_decl_list var_decl_list
%type <spec>    var_decl
%type <expr>    opt_var_init
%type <typ>     type_decl type_post type_v2 type_v2_prefix
%type <params>  arg_decl_list
%type <param>   arg_decl

%right '=' ADDEQ SUBEQ
%left OROR
%left ANDAND
%left '|'
%left '^'
%left '&'
%left EQEQ NEQ
%left '<' '>' LEQ GEQ
%left LSHIFT RSHIFT
%left '+' '-'
%left '*' '/' '%'
%left kAS
%nonassoc UMINUS
%left '.' '(' '['
%nonassoc NO_ELSE
%nonassoc kELSE kELSIF

%%

/****************************************************/
/* program */
program: opt_statement_list { yylex.(*yyLexAdapter).result = $1 }

/****************************************************/
/* statement */

opt_statement_list: statement_list
                  | /* empty */ { $$ = []Stmt{} }

statement_list: statement_list statement_i { $$ = append($1, $2) }
              | statement_i { $$ = []Stmt{$1} }

statement_i: statement

statement: opt_scope kVAR var_decl_list ';'     { $$ = &VarDecl{PublicPos: optPos($1), Keyword: $2.Pos, Specs: $3, Semi: $4.Pos} }
         | opt_scope kCONST var_decl_list ';'   { $$ = &VarDecl{PublicPos: optPos($1), Keyword: $2.Pos, Const: true, Specs: $3, Semi: $4.Pos} }
         | kIF '(' exp ')' statement else_block { $$ = &IfStmt{If: $1.Pos, Lparen: $2.Pos, Cond: $3, Rparen: $4.Pos, Then: $5, Else: $6} }
         | kLOOP '(' ')' statement              { $$ = &LoopStmt{Loop: $1.Pos, Rparen: $3.Pos, Body: $4} }
         | kLOOP block                          { $$ = &LoopStmt{Loop: $1.Pos, Body: $2} } /* v2: 括弧なし */
         | IDENT ':' statement                  { $$ = &LabeledStmt{Label: ident($1), Colon: $2.Pos, Stmt: $3} } /* v2: 文ラベル */
         | kWHILE '(' exp ')' statement         { $$ = &WhileStmt{While: $1.Pos, Cond: $3, Rparen: $4.Pos, Body: $5} }
         | kFOR '(' IDENT ',' exp ',' exp ')' block { $$ = &ForStmt{For: $1.Pos, Var: ident($3), From: $5, To: $7, Rparen: $8.Pos, Body: $9} } /* v1 */
         | kFOR '(' opt_for_init ';' opt_exp ';' opt_for_step ')' block
                                                { $$ = &ForStmt{For: $1.Pos, Init: $3, Cond: $5, Step: $7, Rparen: $8.Pos, Body: $9} } /* v2: C 型 */
         | incdec ';'                           { s := $1.(*IncDecStmt); s.Semi = $2.Pos; $$ = s } /* v2 */
         | kBREAK opt_ident ';'                 { $$ = &BreakStmt{Keyword: $1.Pos, Label: $2, Semi: $3.Pos} }
         | kCONTINUE opt_ident ';'              { $$ = &ContinueStmt{Keyword: $1.Pos, Label: $2, Semi: $3.Pos} }
         | kRETURN opt_exp ';'                  { $$ = &ReturnStmt{Return: $1.Pos, Value: $2, Semi: $3.Pos} }
         | kSWITCH '(' exp ')' '{' switch_block opt_default_block '}' { $$ = &SwitchStmt{Switch: $1.Pos, Tag: $3, Lbrace: $5.Pos, Cases: $6, Default: $7, Rbrace: $8.Pos} }
         | exp ';'                              { $$ = &ExprStmt{X: $1, Semi: $2.Pos} }
         | opt_scope kFUNCTION IDENT '(' opt_var_decl_list ')' ':' type_decl opt_options function_block
                                                { $$ = funcDecl($1, $2, $3, $5, $8, $9, $10) }
         | options ';'                          { $$ = &OptionsStmt{Options: $1, Semi: $2.Pos} }
         | opt_scope kUSE use_target ';'        { u := $3; u.PublicPos = optPos($1); u.Use = $2.Pos; u.Semi = $4.Pos; $$ = u }
         | opt_scope kSTRUCT IDENT '{' field_decl_list '}' { $$ = &StructDecl{PublicPos: optPos($1), Keyword: $2.Pos, Name: ident($3), Lbrace: $4.Pos, Fields: $5, Rbrace: $6.Pos} } /* v2 */
         | opt_scope kSOA IDENT ':' type_decl opt_options ';' { $$ = &SoaDecl{PublicPos: optPos($1), Keyword: $2.Pos, Name: ident($3), Type: $5, Options: $6, Semi: $7.Pos} } /* v2 */
         | opt_scope kSOA kCONST IDENT ':' type_decl '=' exp opt_options ';' { $$ = &SoaDecl{PublicPos: optPos($1), Keyword: $2.Pos, Const: true, Name: ident($4), Type: $6, Init: $8, Options: $9, Semi: $10.Pos} } /* v2 */
         | kINCLUDE opt_ident '(' STRING ')' opt_options ';' { $$ = &IncludeDecl{Include: $1.Pos, Kind: $2, Path: strLit($4), Rparen: $5.Pos, Options: $6, Semi: $7.Pos} }
         | kPUBLIC ':'                          { $$ = &ScopeLabel{Keyword: $1.Pos, Public: true, Colon: $2.Pos} }
         | kPRIVATE ':'                         { $$ = &ScopeLabel{Keyword: $1.Pos, Public: false, Colon: $2.Pos} }
         | block                                { $$ = $1 }
         | ';'                                  { $$ = &EmptyStmt{Semi: $1.Pos} }

/* struct のフィールド宣言 (v2) */
field_decl_list: /* empty */ { $$ = []*FieldDecl{} }
               | field_decl_list field_decl { $$ = append($1, $2) }
field_decl: IDENT ':' type_decl ';' { $$ = &FieldDecl{Name: ident($1), Type: $3, Semi: $4.Pos} }

/* struct リテラル (v2): Point{x: 1, y: 2} / Point{1, 2} / 要素型が分かる文脈では {1, 2} */
struct_lit: IDENT '{' field_inits '}' { $$ = &StructLit{Type: &NamedType{Name: ident($1)}, Lbrace: $2.Pos, Fields: $3, Rbrace: $4.Pos} }
anon_struct_lit: '{' field_inits '}' { $$ = &StructLit{Lbrace: $1.Pos, Fields: $2, Rbrace: $3.Pos} }
field_inits: /* empty */ { $$ = []*FieldInit{} }
           | field_init_list { $$ = $1 }
           | field_init_list ',' { $$ = $1 }
field_init_list: field_init { $$ = []*FieldInit{$1} }
               | field_init_list ',' field_init { $$ = append($1, $3) }
field_init: IDENT ':' lit_elem { $$ = &FieldInit{Key: ident($1), Colon: $2.Pos, Value: $3} }
          | lit_elem { $$ = &FieldInit{Value: $1} }
/* 配列リテラルの要素と struct リテラルの値: 式か、型名を省いた struct リテラル */
lit_elem: exp
        | anon_struct_lit { $$ = $1 }
lit_elem_list: lit_elem { $$ = []Expr{$1} }
             | lit_elem_list ',' lit_elem { $$ = append($1, $3) }

/* C 型 for の各部 (v2) */
opt_for_init: /* empty */ { $$ = nil }
            | kVAR var_decl_list { $$ = &VarDecl{Keyword: $1.Pos, Specs: $2} }
            | simple_stmt

opt_for_step: /* empty */ { $$ = nil }
            | simple_stmt

simple_stmt: exp { $$ = &ExprStmt{X: $1} }
           | incdec

incdec: exp INCR { $$ = &IncDecStmt{X: $1, OpPos: $2.Pos, Op: Inc} }
      | exp DECR { $$ = &IncDecStmt{X: $1, OpPos: $2.Pos, Op: Dec} }
      | INCR exp { $$ = &IncDecStmt{X: $2, OpPos: $1.Pos, Op: Inc, Prefix: true} }
      | DECR exp { $$ = &IncDecStmt{X: $2, OpPos: $1.Pos, Op: Dec, Prefix: true} }

opt_scope: /* empty */ { $$ = nil }
         | kPUBLIC { t := $1; $$ = &t }

/* use の対象: モジュール束縛 / glob / 選択的インポート (v2) */
use_target: IDENT opt_as                { $$ = &UseDecl{Module: ident($1), As: $2} }
          | '*' kFROM IDENT             { $$ = &UseDecl{Star: $1.Pos, FromAll: true, Module: ident($3)} }
          | ident_list kFROM IDENT      { $$ = &UseDecl{Names: $1, Module: ident($3)} } /* v2 */

opt_as: /* empty */ { $$ = nil }
      | kAS IDENT { $$ = ident($2) }

ident_list: IDENT { $$ = []*Ident{ident($1)} }
          | ident_list ',' IDENT { $$ = append($1, ident($3)) }

opt_ident: /* empty */ { $$ = nil }
         | IDENT { $$ = ident($1) }

opt_default_block: /* empty */ { $$ = nil }
                 | kDEFAULT ':' statement_list { $$ = &DefaultClause{Default: $1.Pos, Colon: $2.Pos, Body: $3} }

switch_block: switch_block case_block { $$ = append($1, $2) }
            | case_block { $$ = []*CaseClause{$1} }

case_block: kCASE exp_list ':' statement_list { $$ = &CaseClause{Case: $1.Pos, Values: $2, Colon: $3.Pos, Body: $4} }

function_block: block { $$ = funcBody{Block: $1} }
              | ';' { $$ = funcBody{Semi: $1.Pos} }

block: '{' opt_statement_list '}' { $$ = &Block{Lbrace: $1.Pos, Stmts: $2, Rbrace: $3.Pos} }

opt_block: /* empty */ { $$ = nil }
         | block

else_block: /* empty */ %prec NO_ELSE { $$ = nil }
          | kELSE statement { $$ = $2 }
          | kELSIF '(' exp ')' statement else_block { $$ = &IfStmt{If: $1.Pos, IsElsif: true, Lparen: $2.Pos, Cond: $3, Rparen: $4.Pos, Then: $5, Else: $6} }

/****************************************************/
/* expression */
opt_exp: /* empty */ { $$ = nil }
       | exp

exp: '(' exp ')'            { $$ = &ParenExpr{Lparen: $1.Pos, X: $2, Rparen: $3.Pos} }
   | exp '.'  exp           { $$ = binary($1, $2, $3) }
   | exp '='  exp           { $$ = &AssignExpr{Lhs: $1, OpPos: $2.Pos, Op: Assign, Rhs: $3} }
   | exp '+'  exp           { $$ = binary($1, $2, $3) }
   | exp '-'  exp           { $$ = binary($1, $2, $3) }
   | exp '*'  exp           { $$ = binary($1, $2, $3) }
   | exp '/'  exp           { $$ = binary($1, $2, $3) }
   | exp '%'  exp           { $$ = binary($1, $2, $3) }
   | exp '&'  exp           { $$ = binary($1, $2, $3) }
   | exp '|'  exp           { $$ = binary($1, $2, $3) }
   | exp '^'  exp           { $$ = binary($1, $2, $3) }
   | exp ANDAND exp         { $$ = binary($1, $2, $3) }
   | exp OROR exp           { $$ = binary($1, $2, $3) }
   | exp ADDEQ exp          { $$ = &AssignExpr{Lhs: $1, OpPos: $2.Pos, Op: AddEq, Rhs: $3} }
   | exp SUBEQ exp          { $$ = &AssignExpr{Lhs: $1, OpPos: $2.Pos, Op: SubEq, Rhs: $3} }
   | exp EQEQ exp           { $$ = binary($1, $2, $3) }
   | exp NEQ exp            { $$ = binary($1, $2, $3) }
   | exp '<'  exp           { $$ = binary($1, $2, $3) }
   | exp '>'  exp           { $$ = binary($1, $2, $3) }
   | exp LEQ exp            { $$ = binary($1, $2, $3) }
   | exp GEQ exp            { $$ = binary($1, $2, $3) }
   | exp LSHIFT exp         { $$ = binary($1, $2, $3) }
   | exp RSHIFT exp         { $$ = binary($1, $2, $3) }
   | '<' type_decl '>' exp  { $$ = &CastExpr{Lt: $1.Pos, Type: $2, Gt: $3.Pos, X: $4} } /* v1 */
   | exp kAS type_v2         { $$ = &CastExpr{Kind: CastAs, X: $1, As: $2.Pos, Type: $3} } /* v2: 数値変換 */
   | kBITCAST '<' type_decl '>' '(' exp ')' { $$ = &CastExpr{Kind: CastBit, Bitcast: $1.Pos, Lt: $2.Pos, Type: $3, Gt: $4.Pos, Lparen: $5.Pos, X: $6, Rparen: $7.Pos} } /* v2: ビット読み替え */
   | '!' exp %prec UMINUS   { $$ = unary($1, $2) }
   | '-' exp %prec UMINUS   { $$ = unary($1, $2) }
   | '+' exp %prec UMINUS   { $$ = unary($1, $2) }
   | '*' exp %prec UMINUS   { $$ = unary($1, $2) }
   | '&' exp %prec UMINUS   { $$ = unary($1, $2) }
   | exp '(' arg_list ')' opt_block { $$ = &CallExpr{Fun: $1, Lparen: $2.Pos, Args: $3.exprs, Comma: $3.comma, Rparen: $4.Pos, Block: $5} }
   | exp '[' exp ']'        { $$ = &IndexExpr{X: $1, Lbrack: $2.Pos, Index: $3, Rbrack: $4.Pos} }
   | '[' ']'                { $$ = &ArrayLit{Lbrack: $1.Pos, Elems: []Expr{}, Rbrack: $2.Pos} }
   | '[' lit_elem_list ']'  { $$ = &ArrayLit{Lbrack: $1.Pos, Elems: $2, Rbrack: $3.Pos} }
   | '[' lit_elem_list ',' ']' { $$ = &ArrayLit{Lbrack: $1.Pos, Elems: $2, Comma: $3.Pos, Rbrack: $4.Pos} }
   | struct_lit             { $$ = $1 }
   | kSIZEOF '(' type_decl ')' { $$ = &SizeofExpr{Sizeof: $1.Pos, Lparen: $2.Pos, Type: $3, Rparen: $4.Pos} } /* v2 */
   | kINCBIN '(' STRING ')' { $$ = &IncbinExpr{Incbin: $1.Pos, Path: strLit($3), Rparen: $4.Pos} }
   | ARROW type_decl function_block { $$ = &LambdaExpr{Arrow: $1.Pos, Type: $2, Body: $3.Block, Semi: $3.Semi} }
   | NUMBER                 { $$ = &IntLit{ValuePos: $1.Pos, Value: $1.Int, Text: $1.Text} }
   | IDENT                  { $$ = ident($1) }
   | STRING                 { $$ = strLit($1) }

exp_list: exp_list ',' exp { $$ = append($1, $3) }
        | exp { $$ = []Expr{$1} }

/* 呼び出しの引数と配列リテラルの要素。空でもよく、末尾のカンマ (v2) を許す */
arg_list: /* empty */ { $$ = argList{exprs: []Expr{}} }
        | exp_list { $$ = argList{exprs: $1} }
        | exp_list ',' { $$ = argList{exprs: $1, comma: $2.Pos} }

/****************************************************/
/* option */
opt_options: /* empty */ { $$ = nil }
           | options
options: kOPTIONS '(' option_list ')' { $$ = &Options{Keyword: $1.Pos, Entries: $3, Rparen: $4.Pos} }

option_list: option_list_sub
option_list_sub: option_list_sub ',' option { $$ = append($1, $3...) }
               | option
option: IDENT ':' exp { $$ = []*OptionEntry{{Key: ident($1), Value: $3}} }

/****************************************************/
/* var declaration */
opt_var_decl_list: /* empty */ { $$ = []*VarSpec{} }
                 | var_decl_list
var_decl_list: var_decl_list ',' var_decl { $$ = append($1, $3) }
             | var_decl { $$ = []*VarSpec{$1} }

var_decl: IDENT ':' type_decl opt_var_init opt_options { $$ = &VarSpec{Name: ident($1), Type: $3, Init: $4, Options: $5} }
        | IDENT '=' exp opt_options { $$ = &VarSpec{Name: ident($1), Init: $3, Options: $4} }

opt_var_init: /* empty */ { $$ = nil }
            | '=' exp { $$ = $2 }

/****************************************************/
/* type declaration */
/* 型。v1 は後置 (int*[4])、v2 は前置 ([4]*int)。1 つの文法で両方受理し、混在や版違いは parse.go のゲートで落とす */
type_decl: type_v2_prefix
         | type_post

type_v2_prefix: '*' type_decl                        { $$ = &PointerType{Star: $1.Pos, Elem: $2} }
              | '[' exp ']' type_decl                { $$ = &ArrayType{Lbrack: $1.Pos, Len: $2, Rbrack: $3.Pos, Elem: $4} }
              | '[' ']' type_decl                    { $$ = &ArrayType{Lbrack: $1.Pos, Rbrack: $2.Pos, Elem: $3} }
              | kFN '(' arg_decl_list ')' ':' type_decl { $$ = &FuncType{Fn: $1.Pos, Lparen: $2.Pos, Params: $3, Rparen: $4.Pos, Result: $6} }

/* v2 の前置形だけ (後置なし)。`x as T` の T に使う: 後ろに二項演算子が続いても曖昧にならない */
type_v2: '*' type_v2                        { $$ = &PointerType{Star: $1.Pos, Elem: $2} }
       | IDENT '.' IDENT                    { $$ = &NamedType{Module: ident($1), Name: ident($3)} }
       | '[' exp ']' type_v2                { $$ = &ArrayType{Lbrack: $1.Pos, Len: $2, Rbrack: $3.Pos, Elem: $4} }
       | '[' ']' type_v2                    { $$ = &ArrayType{Lbrack: $1.Pos, Rbrack: $2.Pos, Elem: $3} }
       | kFN '(' arg_decl_list ')' ':' type_v2 { $$ = &FuncType{Fn: $1.Pos, Lparen: $2.Pos, Params: $3, Rparen: $4.Pos, Result: $6} }
       | IDENT %prec kAS                    { $$ = &NamedType{Name: ident($1)} } /* `x as m.T` の '.' は型の修飾として読む (shift) */

type_post: IDENT '.' IDENT                 { $$ = &NamedType{Module: ident($1), Name: ident($3)} } /* v2: 他モジュールの型 */
         | type_post '[' exp ']'           { $$ = &ArrayType{Elem: $1, Lbrack: $2.Pos, Len: $3, Rbrack: $4.Pos} }
         | type_post '[' ']'               { $$ = &ArrayType{Elem: $1, Lbrack: $2.Pos, Rbrack: $3.Pos} }
         | type_post '*'                   { $$ = &PointerType{Elem: $1, Star: $2.Pos} }
         | type_post '(' arg_decl_list ')' { $$ = &FuncType{Result: $1, Lparen: $2.Pos, Params: $3, Rparen: $4.Pos} }
         | IDENT                           { $$ = &NamedType{Name: ident($1)} }

arg_decl_list: arg_decl_list ',' arg_decl { $$ = append($1, $3) }
             | arg_decl { $$ = []*Param{$1} }
             | /* empty */ { $$ = []*Param{} }

arg_decl: type_decl { $$ = &Param{Type: $1} }
        | IDENT ':' type_decl { $$ = &Param{Name: ident($1), Type: $3} }

%%
