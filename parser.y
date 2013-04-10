class Fc::Parser
  prechigh
    left '.' '(' '['
    nonassoc UMINUS
    left '*' '/' '%'
    left '+' '-'
    left '<<' '>>'
    left '<' '>' '<=' '>='
    left '==' '!='
    left '&'
    left '^'
    left '|'
    left '&&'
    left '||'
    right '=' '+=' '-='
  preclow
  expect 1 /* if-else-else has 1 shift/reduce conflict,  */
rule

/****************************************************/
/* program */
program: opt_statement_list

/****************************************************/
/* statement */

opt_statement_list: statement_list
                  | { result = [] }

statement_list: statement_list statement_i { result = val[0] + [val[1]] }
              | statement_i { result = [val[0]] }
              
statement_i: statement { info(val[0]) }

statement: 'var' var_decl_list ';'               { result = [:var, val[1]] }
         | 'const' var_decl_list ';'             { result = [:const, val[1]] }
         | 'if' '(' exp ')' block else_block     { result = [:if, val[2], val[4], val[5]] }
         | 'loop' '(' ')' block                  { result = [:loop, val[3]] }
         | 'while' '(' exp ')' block             { result = [:while, val[2], val[4]] }
         | 'for' '(' IDENT ',' exp ',' exp ')' block { result = [:for, val[2], val[4], val[6], val[8]] }
         | 'break' ';'                           { result = [:break] }
         | 'continue' ';'                        { result = [:continue] }
         | 'return' opt_exp ';'                   { result = [:return, val[1]] }
         | 'switch' '(' exp ')' '{' switch_block opt_default_block'}' { result = [:switch, val[2], val[5], val[6]] }
         |  exp ';' { result = [:exp, val[0]] }
         | 'function' IDENT '(' opt_var_decl_list ')' ':' type_decl opt_options function_block
                                                 { result = [:function, val[1], val[3], val[6], val[7], val[8]] }
         | options ';'                           { result = [:options, val[0]] }
         | 'use' IDENT ';'              { result = [:use, val[1] ] }
         | 'include' opt_ident '(' STRING ')' opt_options ';'  { result = [:include, val[3], val[1] ] }

opt_ident: | IDENT

opt_default_block: | 'default' ':' statement_list { result = val[2] }

switch_block: switch_block case_block { result = val[0] + [val[1]] }
            | case_block { result = [val[0]] }

case_block: 'case' exp_list ':' statement_list { result = [val[1], val[3]] }

         
function_block: block
              | ';' { result = nil }
              
block: '{' opt_statement_list '}' { result = val[1] }

opt_block: | block
         
else_block:
     | 'else' block { result = val[1] }
     | 'elsif' '(' exp ')' block else_block { result = [[:if, val[2], val[4], val[5]]] }

opt_exp: | exp

exp: '(' exp ')'            { result = val[1] }
   | exp '.'  exp           { result = [:dot, val[0], val[2]] }
   | exp '='  exp           { result = [:load, val[0], val[2]] }
   | exp '+'  exp           { result = [:add, val[0], val[2]] }
   | exp '-'  exp           { result = [:sub, val[0], val[2]] }
   | exp '*'  exp           { result = [:mul, val[0], val[2]] }
   | exp '/'  exp           { result = [:div, val[0], val[2]] }
   | exp '%'  exp           { result = [:mod, val[0], val[2]] }
   | exp '&'  exp           { result = [:and, val[0], val[2]] }
   | exp '|'  exp           { result = [:or , val[0], val[2]] }
   | exp '^'  exp           { result = [:xor, val[0], val[2]] }
   | exp '&&' exp           { result = [:land, val[0], val[2]] }
   | exp '||' exp           { result = [:lor, val[0], val[2]] }
   | exp '+=' exp           { result = [:load, val[0], [:add, val[0], val[2]]] }
   | exp '-=' exp           { result = [:load, val[0], [:sub, val[0], val[2]]] }
   | exp '==' exp           { result = [:eq, val[0], val[2]] }
   | exp '!=' exp           { result = [:ne, val[0], val[2]] }
   | exp '<'  exp           { result = [:lt, val[0], val[2]] }
   | exp '>'  exp           { result = [:gt, val[0], val[2]] }
   | exp '<=' exp           { result = [:le, val[0], val[2]] }
   | exp '>=' exp           { result = [:ge, val[0], val[2]] }
   | '<' type_decl '>' exp  { result = [:cast, val[3], val[1]] }
   | '!' exp = UMINUS       { result = [:not, val[1]] }
   | '-' exp = UMINUS       { result = [:uminus, val[1]] }
   | '*' exp = UMINUS       { result = [:deref, val[1]] }
   | '&' exp = UMINUS       { result = [:ref, val[1]] }
   | exp '(' exp_list ')' opt_block { result = [:call, val[0], val[2], val[4]] }
   | exp '[' exp ']'        { result = [:index, val[0], val[2]] }
   | '[' exp_list ']'       { result = [:array, val[1]] }
   | 'incbin' '(' STRING ')' { result = [:incbin, val[2]] }
   | '->' type_decl function_block { result = [:lambda, val[1], val[2]] }
   | NUMBER
   | IDENT
   | STRING

exp_list: exp_list ',' exp { result = val[0] + [val[2]] }
        | exp { result = [val[0]] }
        | { result = [] }

/****************************************************/
/* option */
opt_options: | options
options : 'options' '(' option_list ')' { result = val[2] }

option_list: option_list_sub { result = Hash[ *val[0] ] }
option_list_sub: option_list_sub ',' option { result = val[0] + val[2] }
               | option { result = val[0] }
option: IDENT ':' exp { result = [val[0],val[2]] }
    
/****************************************************/
/* var declaration */
opt_var_decl_list: {result = [] } | var_decl_list
var_decl_list: var_decl_list ',' var_decl { result = val[0]+[val[2]] }
             | var_decl { result = [val[0]] }

var_decl: IDENT ':' type_decl opt_var_init opt_options { result = [val[0], val[2], val[3], val[4]] }
        | IDENT '=' exp opt_options { result = [val[0], nil, val[2], val[3]] }

opt_var_init: | '=' exp { result = val[1] }

/****************************************************/
/* type declaration */
type_decl: type_decl type_modifier { result = val[1]+[val[0]]; }
         | IDENT { result = val[0] }

type_modifier: '[' exp ']'            { result = [:array, val[1]] }
             | '[' ']'                { result = [:array, nil] }             
             | '*'                    { result = [:pointer] }
             | '(' arg_decl_list ')'  { result = [:lambda, val[1] ] }

arg_decl_list: arg_decl_list ',' arg_decl { result = val[0] + [val[2]] }
             | arg_decl { result = [val[0]] }
             | { result = [] }

arg_decl: type_decl
        | IDENT ':' type_decl { result = [val[0],val[2]] }
               
end

---- footer
require_relative 'parser_ext'
