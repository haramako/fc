# coding: utf-8
#
# golden 生成用の正規形ダンパー。
# lib/ (Ruby版=オラクル) は一切変更せず、モンキーパッチでフックする。
# 出力形式の仕様は doc/go_port_dump_format.md を参照。Go版はこの形式に合わせる。
#
require_relative '../ruby/lib/fc/hlc'
require_relative '../ruby/lib/fc/llc'

module GoldenDump

  module_function

  # ---------------------------------------------------------------
  # 基本シリアライズ
  # ---------------------------------------------------------------

  # 文字列を一意なエスケープ形式にする(バイト単位)
  def esc_str( s )
    r = +'"'
    s.to_s.each_byte do |b|
      case b
      when 0x22 then r << '\\"'
      when 0x5c then r << '\\\\'
      when 0x0a then r << '\\n'
      when 0x20..0x7e then r << b.chr
      else r << format('\\x%02X', b)
      end
    end
    r << '"'
    r
  end

  def type_s( t )
    '#' + esc_str(t.to_s)
  end

  # AST用の汎用S式(compact)
  def sexp( x )
    case x
    when nil then 'nil'
    when true then 'true'
    when false then 'false'
    when Integer then x.to_s
    when Symbol then ':' + x.to_s
    when String then esc_str(x)
    when Array then '(' + x.map{|e| sexp(e)}.join(' ') + ')'
    when Hash then '{' + x.map{|k,v| sexp(k)+' '+sexp(v)}.join(' ') + '}'
    else raise "cannot dump #{x.class}: #{x.inspect}"
    end
  end

  # AST用の整形S式: compact形式が80文字を超えるArrayは要素ごとに改行する
  def pretty_sexp( x, indent = 0 )
    c = sexp(x)
    return ' '*indent + c if c.size <= 80 or !(Array === x)
    r = ' '*indent + '('
    first = true
    x.each do |e|
      ec = sexp(e)
      if first
        # 先頭要素は開き括弧と同じ行に置く(短い場合のみ)
        if !(Array === e) or ec.size <= 80
          r << ec
        else
          r << "\n" << pretty_sexp(e, indent+1)
        end
        first = false
      else
        if !(Array === e) or ec.size <= 80
          r << "\n" << ' '*(indent+1) << ec
        else
          r << "\n" << pretty_sexp(e, indent+1)
        end
      end
    end
    r << ')'
    r
  end

  # ---------------------------------------------------------------
  # AST ダンプ(パース直後の生AST)
  # ---------------------------------------------------------------

  # ソースファイルをパースしてASTダンプ文字列を返す
  def dump_ast_file( path )
    src = File.read( path, encoding: 'utf-8' )
    ast, pos_info = Fc::Parser.new( src, path.to_s ).parse
    ast_str = ast.map{|stmt| pretty_sexp(stmt) }.join("\n") + "\n"
    pos_str = pos_info.map{|k,v| v[1].to_s }.join("\n") + "\n"
    [ast_str, pos_str]
  end

  # ---------------------------------------------------------------
  # 値のシリアライズ (IR用)
  # ---------------------------------------------------------------

  # ctx: { var_index: {object_id=>index} } 現在のLambdaのローカル変数表
  def dump_value( v, ctx )
    case v
    when Fc::CastedValue
      "{cast #{type_s(v.type)} #{v.offset} #{dump_value(v.__getobj__, ctx)}}"
    when Fc::PointeredArray
      "{pa #{dump_value(v.from, ctx)}}"
    when Fc::Value
      idx = ctx && ctx[:var_index][v.object_id]
      if idx
        "{l#{idx} #{v.id}}"
      else
        dump_value_full( v, ctx )
      end
    when Fc::Lambda
      "{lambda #{v.id}}"
    when Fc::Type
      type_s(v)
    when Symbol
      ':' + v.to_s
    when String
      esc_str(v)
    when Integer
      v.to_s
    when nil
      'nil'
    else
      raise "cannot dump value #{v.class}: #{v.inspect}"
    end
  end

  # ローカル表を使わないValueの完全形
  def dump_value_full( v, ctx )
    bs = v.base_string ? ' ' + esc_str(v.base_string) : ''
    case v.kind
    when :literal
      "{lit #{v.id ? v.id : 'nil'} #{dump_gval(v.val, ctx)} #{type_s(v.type)}#{bs}}"
    when :array_literal
      "{arr #{v.id} #{type_s(v.type)} (#{v.val.map{|e| dump_value(e, ctx)}.join(' ')})#{bs}}"
    when :global
      "{g #{v.id} #{type_s(v.type)} #{dump_gval(v.val, ctx)}#{bs}}"
    when :module
      "{mod #{v.id}}"
    when :local
      # 他のLambdaのローカルなど、表にない場合
      "{l? #{v.id} #{type_s(v.type)}}"
    else
      raise "cannot dump value kind #{v.kind}"
    end
  end

  # global/literal の val フィールド
  def dump_gval( val, ctx )
    case val
    when nil then 'nil'
    when Integer then val.to_s
    when Symbol then ':' + val.to_s
    when String then esc_str(val)
    when Fc::Module then "mod:#{val.id}"
    when Proc then 'macro'
    when Array then '(' + val.map{|e| dump_value(e, ctx)}.join(' ') + ')'
    else raise "cannot dump gval #{val.class}"
    end
  end

  # オプションHash( {id:, fastcall:, segment:, address:, ...} )
  def dump_opt( opt )
    return '{}' unless opt
    '{' + opt.map{|k,v|
      vs = case v
           when nil then 'nil'
           when true then 'true'
           when false then 'false'
           when Integer then v.to_s
           when Symbol then ':' + v.to_s
           when String then esc_str(v)
           else raise "cannot dump opt val #{v.class}"
           end
      "#{k} #{vs}"
    }.join(' ') + '}'
  end

  # 変数表の1エントリ
  def dump_var( v, i, ctx, alloc: false )
    r = +"(var #{i} #{v.id ? v.id : 'nil'} #{v.kind} #{type_s(v.type)}"
    r << " val=#{dump_gval(v.val, ctx)}" unless v.kind == :local
    lt = v.opt[:local_type]
    r << " lt=#{lt}" if lt
    r << " pub" if v.public
    if alloc
      r << " loc=#{v.location ? v.location : 'nil'}"
      r << " addr=#{v.address}" if v.address
      r << " cond=#{v.cond_reg},#{v.cond_positive}" if v.cond_reg
      r << " unuse" if v.unuse
    end
    r << ')'
    r
  end

  # defs の1エントリ ( [symbol, kind, type, val] )
  def dump_def( d, ctx )
    sym, kind, type, val = d
    vs = case val
         when nil then 'nil'
         when Integer then val.to_s
         when Symbol then ':' + val.to_s
         when String then esc_str(val)
         when Fc::Lambda then "{lambda #{val.id}}"
         when Hash then dump_opt(val)
         when Array then '(' + val.map{|e| dump_value(e, ctx)}.join(' ') + ')'
         else raise "cannot dump def val #{val.class}"
         end
    "(def #{sym} #{kind} #{type_s(type)} #{vs})"
  end

  # op 1行
  def dump_op( op, ctx )
    return 'nil' if op.nil?
    '(' + op.map{|e| dump_value(e, ctx)}.join(' ') + ')'
  end

  def make_ctx( lmd )
    var_index = {}
    lmd.vars.each_with_index{|v,i| var_index[v.object_id] = i }
    { var_index: var_index }
  end

  # ---------------------------------------------------------------
  # IR ダンプ (HLC完了直後、LLC適用前)
  # ---------------------------------------------------------------

  def dump_ir( hlc )
    r = []
    hlc.options.each do |k,v|
      r << "(option #{k} #{v.is_a?(String) ? esc_str(v) : v})"
    end
    hlc.modules.each do |id, mod|
      r << "(module #{mod.id}"
      r << " (options #{dump_opt(mod.options)})"
      r << " (include_asms (#{mod.include_asms.map{|p| esc_str(p.to_s)}.join(' ')}))"
      r << " (include_chrs (#{mod.include_chrs.map{|p| esc_str(p.to_s)}.join(' ')}))"
      r << " (modules (#{mod.modules.keys.join(' ')}))"
      r << " (defs"
      mod.defs.each{|d| r << '  ' + dump_def(d, nil) }
      r << " )"
      r << " (vars"
      mod.vars.each_with_index{|v,i| r << '  ' + dump_var(v, i, nil) }
      r << " )"
      mod.lambdas.each do |lmd|
        r.concat dump_lambda( lmd )
      end
      r << ")"
    end
    r.join("\n") + "\n"
  end

  def dump_lambda( lmd )
    ctx = make_ctx( lmd )
    r = []
    r << " (lambda #{lmd.id} #{type_s(lmd.type)} opt=#{dump_opt(lmd.opt)}"
    r << "  (args (#{lmd.args.map{|a| dump_value(a, ctx)}.join(' ')}))"
    r << "  (result #{lmd.result ? dump_value(lmd.result, ctx) : 'nil'})"
    r << "  (vars"
    lmd.vars.each_with_index{|v,i| r << '   ' + dump_var(v, i, ctx) }
    r << "  )"
    r << "  (defs"
    lmd.defs.each{|d| r << '   ' + dump_def(d, ctx) }
    r << "  )"
    r << "  (ops"
    lmd.ops.each_with_index{|op,i| r << format('   %04d %s', i, dump_op(op, ctx)) }
    r << "  )"
    r << " )"
    r
  end

  # ---------------------------------------------------------------
  # 割付後IR ダンプ (alloc_register + delete_unuse 直後、optimize_pointer 適用前)
  # ---------------------------------------------------------------

  def dump_alloc_lambda( mod_id, sym, lmd )
    ctx = make_ctx( lmd )
    r = []
    r << "(alloc-lambda #{mod_id} #{sym} #{lmd.id} frame_size=#{lmd.frame_size}"
    r << " (vars"
    lmd.vars.each_with_index{|v,i| r << '  ' + dump_var(v, i, ctx, alloc: true) }
    r << " )"
    r << " (ops"
    lmd.ops.each_with_index{|op,i| r << format('  %04d %s', i, dump_op(op, ctx)) }
    r << " )"
    r << ")"
    r.join("\n") + "\n"
  end

  # ---------------------------------------------------------------
  # asm 正規化: IRコメント行( ^\s*; \d{4}: )を除去する
  # ---------------------------------------------------------------

  def normalize_asm( text )
    text.split("\n", -1).reject{|line| line =~ /\A\s*; \d{4}:/ }.join("\n")
  end

  # ---------------------------------------------------------------
  # 収集フック
  # ---------------------------------------------------------------

  class Collector
    attr_reader :ir, :allocs
    attr_accessor :hlc
    def initialize
      @ir = nil
      @allocs = []
      @hlc = nil
    end
    def add_ir( hlc )
      @hlc = hlc
      @ir = GoldenDump.dump_ir( hlc )
    end
    def add_alloc( mod_id, sym, lmd )
      @allocs << GoldenDump.dump_alloc_lambda( mod_id, sym, lmd )
    end
  end

  @collector = nil
  class << self
    attr_accessor :collector
  end
end

# --- モンキーパッチ(lib/は不変更のままフックする) ---

class Fc::Hlc
  alias_method :golden_orig_compile, :compile
  def compile( filename )
    r = golden_orig_compile( filename )
    GoldenDump.collector&.add_ir( self )
    r
  end
end

class Fc::Llc
  alias_method :golden_orig_compile_lambda, :compile_lambda
  def compile_lambda( sym, lmd )
    @golden_cur_sym = sym
    golden_orig_compile_lambda( sym, lmd )
  end

  alias_method :golden_orig_alloc_register, :alloc_register
  def alloc_register( lmd )
    r = golden_orig_alloc_register( lmd )
    GoldenDump.collector&.add_alloc( @code_segment, @golden_cur_sym, lmd )
    r
  end
end
