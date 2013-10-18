# coding: utf-8

require 'pathname'
require 'delegate'

module Fc

  DEBUG_LEVEL = 1

  FC_HOME = Pathname(File.dirname( __FILE__ )) + '../..'
  LIB_PATH = [Pathname('.'), FC_HOME+'fclib' ]

  # share以下のファイルを検索する
  def self.find_share( path )
    FC_HOME + 'share' + path
  end

  # fclib以下のファイルを検索する
  def self.find_module( file )
    LIB_PATH.each do |path|
      return path + file if File.exists?( path + file )
    end
    raise CompileError.new( "file #{file} not found" );
  end

  ######################################################################
  # 型
  # Type.new() ではなく Type[] で生成すること
  ######################################################################
  class Type

    attr_reader :kind # 種類( :void, :int, :pointer, :array, :lambda のいずれか )
    attr_reader :size # サイズ(単位:byte)
    attr_reader :signed # signedかどうか(intの場合のみ)
    attr_reader :base # ベース型(pointer,array,lambdaの場合のみ)
    attr_reader :length # 配列の要素数(arrayのみ)
    attr_reader :args # 引数クラスのリスト(lambdaの場合のみ)

    BASIC_TYPES = { 
      int:[1,false], uint:[1,false], sint:[1,true],
      int8:[1,false], sint8:[1,true], uint8:[1,false], 
      int16:[2,false], sint16:[2,true], uint16:[2,false] }

    private_class_method :new

    def initialize( ast )
      if ast == :void
        @kind = :void
        @size = 0
      elsif ast == :bool
        @kind = :bool
        @size = 1
      elsif ast == :module
        @kind = :module
        @size = 0
      elsif ast == :macro
        @kind = :macro
        @size = 0
      elsif Symbol === ast
        @kind = :int
        if BASIC_TYPES[ast]
          @size = BASIC_TYPES[ast][0]
          @signed = BASIC_TYPES[ast][1]
        else
          #:nocov:
          raise
          #:nocov:
        end
      elsif Array === ast
        if ast[0] == :pointer
          @kind = :pointer
          @base = Type[ ast[1] ]
          @size = 2
        elsif ast[0] == :array
          @kind = :array
          @base = Type[ ast[2] ]
          @length = ast[1]
          @size = @length && @base.size * @length
        elsif ast[0] == :lambda
          @kind = :lambda
          @base = Type[ ast[2] ]
          @args = ast[1].map{|t| Type[t] }
          @size = 2
        end
      end

      case @kind
      when :void
        @str = "void"
      when :int, :void, :bool
        @str = "#{signed ? 's' : 'u' }#{@kind}#{size*8}"
      when :module
        @str = "module"
      when :macro
        @str = "macro"
      when :pointer
        @str = "#{@base}*"
      when :array
        @str = "#{@base}[#{@length||''}]"
      when :lambda
        @str = "#{@base}(#{args.join(",")})"
      else
        #:nocov:
        raise "invalid type declaration #{ast}"
        #:nocov:
      end
    end

    def to_s
      @str
    end
    alias inspect to_s

    def self.[]( ast_or_type )
      return ast_or_type if Type === ast_or_type
      @@cache = Hash.new unless defined?(@@cache)
      type = new( ast_or_type )
      @@cache[type.to_s] = type unless @@cache[type.to_s]
      @@cache[type.to_s]
    end

  end

  ######################################################################
  # 変数、定数など識別子で区別されるもの
  #
  # 区別したいのは以下のもの
  #                 例                代入 id   val    kind          asm
  # 引数            (arg:int)         o    arg  -      arg           __STACK__+0,x
  # 帰り値          return 0;         o    $result -   result        __STACK__-N,x
  # ローカル変数    var i:int;        o    i    -      var           __STACK__+N,x
  # テンポラリ変数                    x    $0   -      temp          __STACK__+N,x
  # ローカル定数    const c = 1;      x    c    1      const         #1
  # ローカル定数2   const c = [1,2]   x    c    [1,2]  symbol        .c
  # 文字列リテラル  "hoge"            x    $0   [1,2]  symbol        .a0 ( int[]の定数として保持 )
  # グローバル変数  var i:int;        o    i    -      global_var    i
  # グローバル定数  const c = 1;      x    c    1      global_const  c
  # グローバル定数2 function f():void x    f    f      global_symbol f
  # リテラル        1                 x    -    1      literal       #1
  #
  # シンボルをもつか、値をもつか
  # アセンブラでシンボルを使うか、スタックを使うか
  # 定数か変数か
  # 代入可能か？
  #
  ######################################################################
  class Value
    
    attr_reader :kind # 種類
    attr_reader :type # Type
    attr_reader :id   # 変数名
    attr_accessor :long_id   # 変数名(モジュール名を含む)
    attr_reader :val  # 定数の場合はその値( Fixnum or Array or Lambda or Proc(マクロ) )
    attr_reader :opt  # オプション
    attr_accessor :base_string # 元の値が文字列だった場合、その文字列

    # 以下は、アセンブラで使用
    attr_accessor :address # アドレス
    attr_accessor :location # 格納場所( :frame, :reg(実際はゼロページメモリ), :mem, :none, :a, :cond のいずれか )
    attr_accessor :unuse # 未使用かどうか
    attr_accessor :live_range
    attr_accessor :cond_reg # コンディションレジスタの種類, location==:condの時のみ使用, (:carry, :zero, :negative) のいずれか
    attr_accessor :cond_positive # コンディションレジスタがどちらの状態を表すか( true/false ), location==:condの時のみ使用
    attr_accessor :public # public かどうか

    def initialize( kind, id, type, val, opt )
      raise CompileError.new("invalid type, #{type}") unless Type === type
      unless [:arg, :result, :var, :temp, :const, :symbol, 
              :global_var, :global_const, :global_symbol, :literal, :array_literal ].include?( kind )
        #:nocov:
        raise "invalid kind, #{kind}" 
        #:nocov:
      end
      @kind = kind
      @id = id
      @type = type
      @val = val
      @opt = opt || Hash.new
      @public = false
      @long_id = id
    end

    def self.new_int( n )
      if n >= 256
        type = Type[:int16]
      elsif n < -127
        type = Type[:sint16]
      elsif n < 0
        type = Type[:sint8]
      else
        type = Type[:int8]
      end
      Value.new( :literal, nil, type, n, nil )
    end

    def assignable?
      [:arg, :result, :var, :global_var].include?( @kind )
    end
    
    def const?
      [:const, :global_const, :literal, :global_symbol ].include?( @kind )
    end
    
    def on_stack?
      [:var, :temp, :arg, :result].include?( @kind )
    end

    def inspect
      if @id
        "{#{id}:#{type}}"
      elsif @base_string
        '{"'+@base_string+'"}'
      else
        "{#{val}}"
      end
    end

    def to_s
      if @id
        "{#{id}}"
      else
        inspect
      end
    end

  end

  # c++でいうところの reintepret_cast<> を表す
  class CastedValue < Delegator

    attr_reader :from, :type, :offset, :val

    def initialize( from, type, offset )
      @from = from
      @type = type
      @offset = offset
      @val = nil
    end

    def to_s
      if @offset == 0
        "<#{@type}>#{@from}"
      else
        "<#{@type}+#{@offset}>#{@from}"
      end
    end
    alias inspect to_s

    def __getobj__
      @from
    end

  end

  # 配列からポインタへの自動変換された値を表す
  class PointeredArray

    attr_reader :from, :type, :val

    def initialize( from )
      @from = from
      @type = Type[[:pointer, from.type.base]]
      @val = nil
    end

    def to_s
      "#{@from}#p"
    end
    alias inspect to_s

    def assignable?
      false
    end

    def const?
      false
    end

    def on_stack?
      false
    end

  end

  ######################################################################
  # スコープ
  ######################################################################
  class Scope
    attr_reader :declares # 宣言( keyはSymbol, valはValue )
    attr_reader :parent   # 親スコープ( Scopeオブジェクト )

    def initialize( parent = nil )
      @parent = parent
      @declares = Hash.new
      @uses = []
      @finding = false
    end

    def find( id, with_private = true )
      return nil if @finding
      begin
        @finding = true
        if @declares[id] and (with_private or @declares[id].public)
          return @declares[id]
        else
          @uses.each do |scope|
            var = scope.find( id, false )
            return var if var
          end
          return @parent.find(id, with_private) if parent
        end
      ensure
        @finding = false
      end
      nil
    end

    def find!( id, with_private = true )
      find(id, with_private) or raise CompileError.new( "#{id} not found" )
    end

    def declare( val )
      raise CompileError.new("#{val.id} already defined") if @declares[val.id]
      @declares[val.id] = val
    end

    def use( scope )
      @uses << scope
    end

    def id_list
      r = @declares.keys # + @uses.map{|x| x.id_list}.flatten 
      r += @parent.id_list if @parent
      r.flatten
    end

    def inspect
      "#<Scope #{id_list}>"
    end
    alias to_s inspect

  end


  ######################################################################
  # モジュール
  ######################################################################
  class Module
    attr_reader :vars, :lambdas, :options, :include_chrs, :modules, :blobs, :include_asms, :scope
    attr_reader :include_headers
    attr_accessor :id
    attr_accessor :current_scope

    def initialize( global_scope )
      @vars = []
      @lambdas = []
      @options = Hash.new
      @includes = []
      @include_chrs = []
      @include_asms = []
      @include_headers = []
      @modules = Hash.new
      @blobs = []
      @scope = Scope.new( global_scope )
      @current_scope = :public
    end

  end

  ######################################################################
  # 関数
  ######################################################################
  class Lambda
    attr_reader :args, :type, :opt, :ast, :vars, :blobs
    attr_accessor :id, :asm, :frame_size, :bank, :ops, :result

    def initialize( id, args, base_type, opt, ast )
      @id = id
      @args = args
      @type = Type[[:lambda, args.map{|arg|arg[1]}, base_type]]
      @opt = opt || Hash.new
      @ast = ast
      @ops = []
      @vars = []
      @bank = 0
      @blobs = []
      @result = nil
    end

    def to_s
      "<Lambda:#{@id} #{@type}>"
    end

  end

  ######################################################################
  # エラークラス
  ######################################################################
  class CompileError < RuntimeError
    attr_accessor :line_no
    attr_accessor :filename
  end


end
