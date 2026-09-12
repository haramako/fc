/*
 * parser.y — fc 文法 (v1) の goyacc 定義。型付き構文木 (ast.go) を生成する。
 *
 * 文法規則と優先順位は internal/fc/parser.y (Ruby racc 版の移植) と 1:1 で対応させている。
 * 唯一の差: 到達不能だった `id_list: tID ...` 規則 (レキサが決して返さないトークン) を削除した。
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
	optTok  *Token
}

%token <tok> NUMBER IDENT STRING
%token <tok> kINCLUDE kFUNCTION kCONST kVAR kOPTIONS kIF kELSE kELSIF kLOOP kWHILE kFOR kRETURN kBREAK kCONTINUE kINCBIN kSWITCH kCASE kDEFAULT kUSE kAS kFROM kPUBLIC kPRIVATE
%token <tok> LEQ GEQ EQEQ ADDEQ SUBEQ NEQ ARROW LSHIFT RSHIFT ANDAND OROR
%token <tok> '(' ')' '{' '}' ';' ':' '<' '>' '[' ']' '+' '-' '*' '/' '%' '&' '|' '^' '=' ',' '.' '!'

%type <stmts>   program opt_statement_list statement_list
%type <stmt>    statement_i statement else_block
%type <optTok>  opt_scope opt_from
%type <ident>   opt_as opt_ident
%type <deflt>   opt_default_block
%type <cases>   switch_block
%type <kase>    case_block
%type <block>   block opt_block
%type <fbody>   function_block
%type <expr>    opt_exp exp
%type <exprs>   exp_list
%type <opts>    opt_options options
%type <optents> option_list option_list_sub option
%type <specs>   opt_var_decl_list var_decl_list
%type <spec>    var_decl
%type <expr>    opt_var_init
%type <typ>     type_decl
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
         | kWHILE '(' exp ')' statement         { $$ = &WhileStmt{While: $1.Pos, Cond: $3, Rparen: $4.Pos, Body: $5} }
         | kFOR '(' IDENT ',' exp ',' exp ')' block { $$ = &ForStmt{For: $1.Pos, Var: ident($3), From: $5, To: $7, Rparen: $8.Pos, Body: $9} }
         | kBREAK ';'                           { $$ = &BreakStmt{Keyword: $1.Pos, Semi: $2.Pos} }
         | kCONTINUE ';'                        { $$ = &ContinueStmt{Keyword: $1.Pos, Semi: $2.Pos} }
         | kRETURN opt_exp ';'                  { $$ = &ReturnStmt{Return: $1.Pos, Value: $2, Semi: $3.Pos} }
         | kSWITCH '(' exp ')' '{' switch_block opt_default_block '}' { $$ = &SwitchStmt{Switch: $1.Pos, Tag: $3, Lbrace: $5.Pos, Cases: $6, Default: $7, Rbrace: $8.Pos} }
         | exp ';'                              { $$ = &ExprStmt{X: $1, Semi: $2.Pos} }
         | opt_scope kFUNCTION IDENT '(' opt_var_decl_list ')' ':' type_decl opt_options function_block
                                                { $$ = funcDecl($1, $2, $3, $5, $8, $9, $10) }
         | options ';'                          { $$ = &OptionsStmt{Options: $1, Semi: $2.Pos} }
         | kUSE opt_from IDENT opt_as ';'       { $$ = &UseDecl{Use: $1.Pos, Star: optPos($2), FromAll: $2 != nil, Module: ident($3), As: $4, Semi: $5.Pos} }
         | kINCLUDE opt_ident '(' STRING ')' opt_options ';' { $$ = &IncludeDecl{Include: $1.Pos, Kind: $2, Path: strLit($4), Rparen: $5.Pos, Options: $6, Semi: $7.Pos} }
         | kPUBLIC ':'                          { $$ = &ScopeLabel{Keyword: $1.Pos, Public: true, Colon: $2.Pos} }
         | kPRIVATE ':'                         { $$ = &ScopeLabel{Keyword: $1.Pos, Public: false, Colon: $2.Pos} }
         | block                                { $$ = $1 }
         | ';'                                  { $$ = &EmptyStmt{Semi: $1.Pos} }

opt_scope: /* empty */ { $$ = nil }
         | kPUBLIC { t := $1; $$ = &t }

opt_from: /* empty */ { $$ = nil }
        | '*' kFROM { t := $1; $$ = &t }

opt_as: /* empty */ { $$ = nil }
      | kAS IDENT { $$ = ident($2) }

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
   | '<' type_decl '>' exp  { $$ = &CastExpr{Lt: $1.Pos, Type: $2, Gt: $3.Pos, X: $4} }
   | '!' exp %prec UMINUS   { $$ = unary($1, $2) }
   | '-' exp %prec UMINUS   { $$ = unary($1, $2) }
   | '+' exp %prec UMINUS   { $$ = unary($1, $2) }
   | '*' exp %prec UMINUS   { $$ = unary($1, $2) }
   | '&' exp %prec UMINUS   { $$ = unary($1, $2) }
   | exp '(' exp_list ')' opt_block { $$ = &CallExpr{Fun: $1, Lparen: $2.Pos, Args: $3, Rparen: $4.Pos, Block: $5} }
   | exp '[' exp ']'        { $$ = &IndexExpr{X: $1, Lbrack: $2.Pos, Index: $3, Rbrack: $4.Pos} }
   | '[' exp_list ']'       { $$ = &ArrayLit{Lbrack: $1.Pos, Elems: $2, Rbrack: $3.Pos} }
   | kINCBIN '(' STRING ')' { $$ = &IncbinExpr{Incbin: $1.Pos, Path: strLit($3), Rparen: $4.Pos} }
   | ARROW type_decl function_block { $$ = &LambdaExpr{Arrow: $1.Pos, Type: $2, Body: $3.Block, Semi: $3.Semi} }
   | NUMBER                 { $$ = &IntLit{ValuePos: $1.Pos, Value: $1.Int, Text: $1.Text} }
   | IDENT                  { $$ = ident($1) }
   | STRING                 { $$ = strLit($1) }

exp_list: exp_list ',' exp { $$ = append($1, $3) }
        | exp { $$ = []Expr{$1} }
        | /* empty */ { $$ = []Expr{} }

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
type_decl: type_decl '[' exp ']'           { $$ = &ArrayType{Elem: $1, Lbrack: $2.Pos, Len: $3, Rbrack: $4.Pos} }
         | type_decl '[' ']'               { $$ = &ArrayType{Elem: $1, Lbrack: $2.Pos, Rbrack: $3.Pos} }
         | type_decl '*'                   { $$ = &PointerType{Elem: $1, Star: $2.Pos} }
         | type_decl '(' arg_decl_list ')' { $$ = &FuncType{Result: $1, Lparen: $2.Pos, Params: $3, Rparen: $4.Pos} }
         | IDENT                           { $$ = &NamedType{Name: ident($1)} }

arg_decl_list: arg_decl_list ',' arg_decl { $$ = append($1, $3) }
             | arg_decl { $$ = []*Param{$1} }
             | /* empty */ { $$ = []*Param{} }

arg_decl: type_decl { $$ = &Param{Type: $1} }
        | IDENT ':' type_decl { $$ = &Param{Name: ident($1), Type: $3} }

%%
