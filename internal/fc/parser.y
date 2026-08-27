/*
 * parser.y (リポジトリルートの racc 版 parser.y の goyacc 移植)
 * 文法規則・アクション・優先順位を 1:1 で対応させている。
 * racc は prechigh→preclow (高い順)、yacc は低い順なので宣言順は逆になっている。
 *
 * 生成: go generate ./internal/fc  ( goyacc -o parser.go -p fc parser.y )
 */

%{
package fc
%}

%union {
	n any
}

%token <n> NUMBER IDENT STRING
%token <n> kINCLUDE kFUNCTION kCONST kVAR kOPTIONS kIF kELSE kELSIF kLOOP kWHILE kFOR kRETURN kBREAK kCONTINUE kINCBIN kSWITCH kCASE kDEFAULT kUSE kAS kFROM kPUBLIC kPRIVATE
%token <n> LEQ GEQ EQEQ ADDEQ SUBEQ NEQ ARROW LSHIFT RSHIFT ANDAND OROR
%token <n> tID /* racc版の未定義非終端子 'id' の再現。レキサは決して返さない */

%type <n> program opt_statement_list statement_list statement_i statement opt_scope opt_from id_list opt_as opt_ident opt_default_block switch_block case_block function_block block opt_block else_block opt_exp exp exp_list opt_options options option_list option_list_sub option opt_var_decl_list var_decl_list var_decl opt_var_init type_decl type_modifier arg_decl_list arg_decl

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
program: opt_statement_list { fclex.(*parserLexer).result = $1.([]any) }

/****************************************************/
/* statement */

opt_statement_list: statement_list
                  | /* empty */ { $$ = []any{} }

statement_list: statement_list statement_i { $$ = cons($1.([]any), $2) }
              | statement_i { $$ = []any{$1} }

statement_i: statement { fclex.(*parserLexer).info($1); $$ = $1 }

statement: opt_scope kVAR var_decl_list ';'     { $$ = []any{Sym("var"), $3, $1} }
         | opt_scope kCONST var_decl_list ';'   { $$ = []any{Sym("const"), $3, $1} }
         | kIF '(' exp ')' statement else_block { $$ = []any{Sym("if"), $3, $5, $6} }
         | kLOOP '(' ')' statement              { $$ = []any{Sym("loop"), $4} }
         | kWHILE '(' exp ')' statement         { $$ = []any{Sym("while"), $3, $5} }
         | kFOR '(' IDENT ',' exp ',' exp ')' block { $$ = []any{Sym("for"), $3, $5, $7, $9} }
         | kBREAK ';'                           { $$ = []any{Sym("break")} }
         | kCONTINUE ';'                        { $$ = []any{Sym("continue")} }
         | kRETURN opt_exp ';'                  { $$ = []any{Sym("return"), $2} }
         | kSWITCH '(' exp ')' '{' switch_block opt_default_block '}' { $$ = []any{Sym("switch"), $3, $6, $7} }
         | exp ';'                              { $$ = []any{Sym("exp"), $1} }
         | opt_scope kFUNCTION IDENT '(' opt_var_decl_list ')' ':' type_decl opt_options function_block
                                                { $$ = []any{Sym("function"), $1, $3, $5, $8, $9, $10} }
         | options ';'                          { $$ = []any{Sym("options"), $1} }
         | kUSE opt_from IDENT opt_as ';'       { $$ = []any{Sym("use"), $3, $4, $2} }
         | kINCLUDE opt_ident '(' STRING ')' opt_options ';' { $$ = []any{Sym("include"), $4, $2, $6} }
         | kPUBLIC ':'                          { $$ = []any{Sym("public")} }
         | kPRIVATE ':'                         { $$ = []any{Sym("private")} }
         | block                                { $$ = []any{Sym("block"), $1} }
         | ';'                                  { $$ = []any{Sym("blank")} }

opt_scope: /* empty */ { $$ = nil }
         | kPUBLIC { $$ = Sym("public") }

opt_from: /* empty */ { $$ = nil }
        | id_list kFROM { $$ = $1 }
        | '*' kFROM { $$ = $<n>1 }

id_list: tID { $$ = []any{$1} }
       | tID id_list { $$ = cons([]any{$1}, $2.([]any)...) }

opt_as: /* empty */ { $$ = nil }
      | kAS IDENT { $$ = $2 }

opt_ident: /* empty */ { $$ = nil }
         | IDENT

opt_default_block: /* empty */ { $$ = nil }
                 | kDEFAULT ':' statement_list { $$ = $3 }

switch_block: switch_block case_block { $$ = cons($1.([]any), $2) }
            | case_block { $$ = []any{$1} }

case_block: kCASE exp_list ':' statement_list { $$ = []any{$2, $4} }

function_block: block
              | ';' { $$ = nil }

block: '{' opt_statement_list '}' { $$ = $2 }

opt_block: /* empty */ { $$ = nil }
         | block

else_block: /* empty */ %prec NO_ELSE { $$ = nil }
          | kELSE statement { $$ = $2 }
          | kELSIF '(' exp ')' statement else_block { $$ = []any{Sym("if"), $3, $5, $6} }

/****************************************************/
/* expression */
opt_exp: /* empty */ { $$ = nil }
       | exp

exp: '(' exp ')'            { $$ = $2 }
   | exp '.'  exp           { $$ = []any{Sym("dot"), $1, $3} }
   | exp '='  exp           { $$ = []any{Sym("load"), $1, $3} }
   | exp '+'  exp           { $$ = []any{Sym("add"), $1, $3} }
   | exp '-'  exp           { $$ = []any{Sym("sub"), $1, $3} }
   | exp '*'  exp           { $$ = []any{Sym("mul"), $1, $3} }
   | exp '/'  exp           { $$ = []any{Sym("div"), $1, $3} }
   | exp '%'  exp           { $$ = []any{Sym("mod"), $1, $3} }
   | exp '&'  exp           { $$ = []any{Sym("and"), $1, $3} }
   | exp '|'  exp           { $$ = []any{Sym("or"), $1, $3} }
   | exp '^'  exp           { $$ = []any{Sym("xor"), $1, $3} }
   | exp ANDAND exp         { $$ = []any{Sym("land"), $1, $3} }
   | exp OROR exp           { $$ = []any{Sym("lor"), $1, $3} }
   | exp ADDEQ exp          { $$ = []any{Sym("load"), $1, []any{Sym("add"), $1, $3}} }
   | exp SUBEQ exp          { $$ = []any{Sym("load"), $1, []any{Sym("sub"), $1, $3}} }
   | exp EQEQ exp           { $$ = []any{Sym("eq"), $1, $3} }
   | exp NEQ exp            { $$ = []any{Sym("ne"), $1, $3} }
   | exp '<'  exp           { $$ = []any{Sym("lt"), $1, $3} }
   | exp '>'  exp           { $$ = []any{Sym("gt"), $1, $3} }
   | exp LEQ exp            { $$ = []any{Sym("le"), $1, $3} }
   | exp GEQ exp            { $$ = []any{Sym("ge"), $1, $3} }
   | exp LSHIFT exp         { $$ = []any{Sym("shift_left"), $1, $3} }
   | exp RSHIFT exp         { $$ = []any{Sym("shift_right"), $1, $3} }
   | '<' type_decl '>' exp  { $$ = []any{Sym("cast"), $4, $2} }
   | '!' exp %prec UMINUS   { $$ = []any{Sym("not"), $2} }
   | '-' exp %prec UMINUS   { $$ = []any{Sym("uminus"), $2} }
   | '+' exp %prec UMINUS   { $$ = $2 }
   | '*' exp %prec UMINUS   { $$ = []any{Sym("deref"), $2} }
   | '&' exp %prec UMINUS   { $$ = []any{Sym("ref"), $2} }
   | exp '(' exp_list ')' opt_block { $$ = []any{Sym("call"), $1, $3, $5} }
   | exp '[' exp ']'        { $$ = []any{Sym("index"), $1, $3} }
   | '[' exp_list ']'       { $$ = []any{Sym("array"), $2} }
   | kINCBIN '(' STRING ')' { $$ = []any{Sym("incbin"), $3} }
   | ARROW type_decl function_block { $$ = []any{Sym("lambda"), $2, $3} }
   | NUMBER
   | IDENT
   | STRING

exp_list: exp_list ',' exp { $$ = cons($1.([]any), $3) }
        | exp { $$ = []any{$1} }
        | /* empty */ { $$ = []any{} }

/****************************************************/
/* option */
opt_options: /* empty */ { $$ = nil }
           | options
options: kOPTIONS '(' option_list ')' { $$ = $3 }

option_list: option_list_sub { $$ = OMapFromPairs($1.([]any)) }
option_list_sub: option_list_sub ',' option { $$ = cons($1.([]any), $3.([]any)...) }
               | option
option: IDENT ':' exp { $$ = []any{$1, $3} }

/****************************************************/
/* var declaration */
opt_var_decl_list: /* empty */ { $$ = []any{} }
                 | var_decl_list
var_decl_list: var_decl_list ',' var_decl { $$ = cons($1.([]any), $3) }
             | var_decl { $$ = []any{$1} }

var_decl: IDENT ':' type_decl opt_var_init opt_options { $$ = []any{$1, $3, $4, $5} }
        | IDENT '=' exp opt_options { $$ = []any{$1, nil, $3, $4} }

opt_var_init: /* empty */ { $$ = nil }
            | '=' exp { $$ = $2 }

/****************************************************/
/* type declaration */
type_decl: type_decl type_modifier { $$ = cons($2.([]any), $1) }
         | IDENT

type_modifier: '[' exp ']'           { $$ = []any{Sym("array"), $2} }
             | '[' ']'               { $$ = []any{Sym("array"), nil} }
             | '*'                   { $$ = []any{Sym("pointer")} }
             | '(' arg_decl_list ')' { $$ = []any{Sym("lambda"), $2} }

arg_decl_list: arg_decl_list ',' arg_decl { $$ = cons($1.([]any), $3) }
             | arg_decl { $$ = []any{$1} }
             | /* empty */ { $$ = []any{} }

arg_decl: type_decl
        | IDENT ':' type_decl { $$ = []any{$1, $3} }

%%
