# coding: utf-8

require 'pp'
require 'strscan'
require 'erb'
require 'json'
require_relative 'base'
require_relative 'parser'
require_relative 'type_util'

module Fc

  ######################################################################
  # High-Level コンパイラ( FCソース -> 中間コード へのコンパイルを行う )
  ######################################################################
  class Hlc
    attr_reader :modules, :options

    def initialize
      @debug = DEBUG_LEVEL
      @pos_info = Hash.new
      @modules = Hash.new
      @options = Hash.new
      @cur_tmp_count = 0
      @global_scope = Scope.new # グローバルスコープ
      @scope = @global_scope # 現在のスコープ
      @loops = [] # ループの脱出用ラベル( [ continueラベル、breakラベル ] の配列 )
      @fastcalling = false # fastcall中かどうか、fastcallの引数を積んでいる途中でfastcallはできないようにするため

      defmacro :asm do |args|
        args.each do |line|
          emit :asm, line.base_string
        end
        nil
      end
        
    end

    def compile( filename )
      compile_module( filename )
      
      # 関数はあとからコンパイル
      @modules.each do |name,mod|
        next if mod.from_fcm
        @module = mod
        attach_scope( @module.scope ) do
          dout 1, "compiling functions in #{name}"
          @module.lambdas.each do |lmd|
            dout 1, "compiling function #{lmd.id}"
            compile_lambda( lmd )
          end
        end
      end
        
    rescue CompileError
      $!.filename ||= @cur_filename
      $!.line_no ||= @cur_line_no 
      $!.backtrace.unshift "#{$!.filename}:#{$!.line_no}"
      raise
    rescue Exception
      #:nocov:
      $!.backtrace.unshift "#{@cur_filename}:#{@cur_line_no}"
      raise
      #:nocov:
    end
    
    ############################################
    # ユーティリティ
    ############################################

    # 現在コンパイル中の行を更新する
    def update_pos( ast )
      if @pos_info[ast]
        pos_info = @pos_info[ast]
        @cur_filename, @cur_line_no = @pos_info[ast]
      end
    end

    def dout( level, *args )
      #:nocov:
      if @debug >= level
        puts args.join(" ")
      end
      #:nocov:
    end

    def tmp_count
      @cur_tmp_count += 1
    end

    def defmacro( name, &block )
      var = add_var Value.new( :global, name, Type[:macro], block, {} )
      var.public = true
      var
    end

    # 変数/定数を追加する
    def add_var( var )
      if @lmd
        @lmd.vars << var 
      elsif @module
        @module.vars << var
      end
      @scope.declare( var )
    end

    def add_def( id, kind, type, val = nil)
      if @lmd
        symbol = id
        @lmd.defs << [symbol, kind, type, val] unless @lmd.defs.find{|d| d[0] == symbol}
      else
        symbol = "_#{@module.id}_#{id}".to_sym
        @module.defs << [symbol, kind, type, val] unless @module.defs.find{|d| d[0] == symbol}
      end
      symbol
    end

    def add_def_module( symbol, kind, type, val = nil)
      @module.defs << [symbol, kind, type, val] unless @module.defs.find{|d| d[0] == symbol}
    end

    # 新しいスコープを作成し、その中でyieldする
    def in_scope
      old_scope = @scope
      @scope = Scope.new( old_scope )
      yield
      @scope = old_scope
    end

    # スコープを指定して、その中でyieldする
    def attach_scope( new_scope )
      old_scope, @scope = @scope, new_scope
      yield
      @scope = old_scope
    end

    ############################################
    # モジュールのコンパイル
    ############################################

    def compile_module( filename )
      path = Fc.find_module(filename)
      id = File.basename(filename,'.fc').to_sym
      return @modules[id] if @modules[id]

      dout 1, "compiling module #{filename}"
      old_module, @module = @module, Module.new( @global_scope )
      @module.path = path

      fcm_path = BUILD_PATH+path.basename.sub_ext('.fcm')
      if false && fcm_path.exist? && fcm_path.mtime > path.mtime # fcmはとりあえず使わない
        # .fcmが新しいなら、それを使う
        dout 1, "load from #{fcm_path}"
        @module.from_json( JSON.parse( IO.read( fcm_path ) ) )
        @module.from_fcm = true
        attach_scope @module.scope do
          @module.modules.each do |k,_|
            mod = compile_module( "#{k}.fc" )
            @module.modules[k] = mod
            add_var Value.new( :module, k.to_sym, Type[:module], mod, nil )
          end
          @module.vars.each do |var|
            var.public = true
            if var.type != Type[:macro] and var.type != Type[:module]
              @scope.declare var
            end
          end
          @module.depends.clone.each do |dep|
            if /\.rb$/ === dep
              compile_statement( [:include, dep, nil, nil] )
            end
          end
        end
        @modules[path] = @module
      else
        # .fcmがないか古いのでコンパイルしなおす
        @module.id = id
        @modules[@module.id] = @module
        src = File.read( path, encoding: 'utf-8' )
        ast, pos_info = Parser.new(src,path).parse
        @pos_info.merge! pos_info

        attach_scope( @module.scope ) do
          compile_block( ast )
          dout 1, "finished module #{filename}"
        end

        # pp @module.id, @module.defs, @module.vars.map{|v| [v.id, v.val]}

        # IO.write( fcm_path, JSON.dump( @module.to_json ) ) # .fcm の書き込み
      end

      new_module = @module
      @module = old_module
      new_module
    end

    ############################################
    # Lambdaのコンパイル
    ############################################

    def compile_lambda( lmd )
      old_lmd, @lmd = @lmd, lmd
      raise unless @loops.empty?
      in_scope do

        # 帰り値の追加
        if @lmd.type.base.kind != :void
          @lmd.result = Value.new( :local, :'$result', @lmd.type.base, nil, {local_type: :result} )
          @lmd.vars.unshift @lmd.result
        end

        # 引数の追加
        @lmd.args.map! do |id,type|
          add_var Value.new( :local, id, type, nil, {local_type: :arg} )
        end

        ast = Marshal.load( Marshal.dump( lmd.ast ) ) # deep copy ast
        compile_block( ast )

        # returnを追加する
        if @lmd.ops[-1].nil? or @lmd.ops[-1][0] != :return
          if @lmd.type.base == Type[:void]
            emit :return
            #else
            #  raise CompileError.new( "no return" )
          end
        end
      end
      @lmd = old_lmd
    end


    ############################################
    # 文のコンパイル( ラムダとモジュールの両方で共通 )
    ############################################

    # ブロックをコンパイルする
    def compile_block( ast )
      return unless ast
      ast.each do |stat|
        compile_statement( stat )
      end
    end

    # ラムダのなかでなければ、例外を投げる
    def must_in_lmd
      raise "not in lambda" unless @lmd
    end

    # モジュール直下でなければ、例外を投げる
    def must_in_module
      raise "not in module" if @lmd
    end

    # 文をコンパイルする
    def compile_statement( ast )
      update_pos( ast )

      case ast[0]

      when :block
        compile_block(ast[1])

      when :blank
        # DO NOTHING
        
      when :options
        must_in_module
        ast[1].each do |k,e| 
          val = const_eval(e)
          if val.base_string
            val = val.base_string
          else
            val = val.val
          end
          @options[k] = val
          @module.options[k] = val
        end

      when :include
        must_in_module
        _, filename, opt_ident, options = ast
        unless opt_ident
          opt_ident = case File.extname(filename)
                      when '.asm' then :asm
                      when '.inc' then :asm
                      when '.chr' then :chr
                      when '.rb' then :macro
                      else raise
                      end
        end
        case opt_ident
        when :asm
          @module.include_asms << Pathname.new( filename )
        when :macro
          path = Fc::find_module( filename )
          src = File.read( path )
          self.instance_eval do
            eval( src, binding, path.to_s )
          end
        when :chr
          @module.include_chrs << Fc::find_module( filename )
        else
          #:nocov:
          raise CompileError.new("invalid keyword #{opt_ident}")
          #:nocov:
        end
        @module.depends << filename

      when :use
        must_in_module
        _, id, as, from = ast
        m = compile_module( "#{id}.fc" )
        @module.modules[id] = m
        raise if as and from
        if from == '*'
          @scope.use m.scope
        else
          id = as || id
          var = add_var Value.new( :global, id, Type[:module], m, {} )
          var.public = true
        end

      when :function
        _, pub, id, args, base_type, opt, block = ast
        compile_statement [:const, [[id, nil, [:lambda, [:lambda, args, base_type], 
                                               block, {id: id}.merge(opt||{})]] ], pub]

      when :var
        ast[1].each do |v|
          id, type, init, opt = v
          opt ||= {}
          init = init && rval( v[2] )
          raise CompileError.new("can't init global variable") if init and !@lmd
          type = type_eval(type)
          type = TypeUtil.guess_type( type, init ) unless type
          TypeUtil.compatible_type( type, init.type ) if type and init
          val = nil
          unless @lmd
            if opt[:address]
              val = add_def( id, :equ, type, opt[:address] )
            else
              val = add_def( id, :bss, type, {segment: opt[:segment]} )
            end
          end
          var = add_var Value.new( (@lmd ? :local : :global), id, type, val, opt )
          var.public = true if (ast[2] || @module.current_scope) == :public 
          emit :load, var, init if init
        end

      when :const
        ast[1].each do |v|
          id, type, val, opt = v
          raise CompileError.new("cannot define const without value #{v[0]}") unless val
          val = const_eval(val) if val
          type = TypeUtil.guess_type(type_eval(type),val)
          if Array === val.val
            symbol = add_def( id, :block, type, val.val )
            new_val = add_var Value.new( :global, id, type, symbol, opt )
          else
            new_val = add_var Value.new( :literal, id, type, val.val, opt )
            unless @lmd
              symbol = add_def( id, :equ, type, val.val ) 
            end
          end
          new_val.public = true if (ast[2] || @module.current_scope) == :public
        end

      when :'if'
        then_label, else_label, end_label = new_labels('then', 'else', 'end')
        cond = rval(ast[1])
        emit :if, cond, else_label
        emit :label, then_label
        in_scope { compile_statement(ast[2]) }
        emit :jump, end_label
        emit :label, else_label
        if ast[3]
          in_scope { compile_statement(ast[3]) }
        end
        emit :label, end_label

      when :loop
        in_scope do
          begin_label, end_label = new_labels('begin', 'end')
          @loops << [begin_label, end_label]
          emit :label, begin_label
          compile_statement ast[1]
          emit :jump, begin_label
          emit :label, end_label
          @loops.pop
        end
        
      when :'while'
        compile_statement( [:loop, [:if, ast[1], ast[2], [:break] ]] )

      when :for
        compile_block( [[:exp, [:load, ast[1], ast[2]]],
                        [:while, 
                         [:lt, ast[1], ast[3]],
                         [:block, ast[4] + [[:exp, [:load, ast[1], [:add, ast[1], 1] ]]]]
                        ] ] )

      when :break
        raise CompileError.new("cannot break without loop") if @loops.empty?
        emit :jump, @loops[-1][1]

      when :continue
        raise CompileError.new("cannot break without loop") if @loops.empty?
        emit :jump, @loops[-1][0]

      when :return
        if @lmd.type.base != Type[:void]
          # 非void関数
          raise CompileError.new("can't return without value") unless ast[1]
          emit :return, rval(ast[1])
        else
          # void関数
          raise CompileError.new("can't return with value from void function") if ast[1]
          emit :return
        end

      when :exp
        lval( ast[1] )

      when :switch
        # TODO: jumptableを使った実装をいれる
        _, cond, cases, default = *ast
        cond = rval( cond )
        tmp = new_tmp( Type[:int] )
        end_label = new_label 'end'
        cases.each do |_case|
          then_label, else_label = new_labels( 'then', 'else' )
          _case[0].each do |v|
            emit :eq, tmp, cond, const_eval(v)
            emit :not, tmp, tmp
            emit :if, tmp, then_label
          end
          emit :jump, else_label
          emit :label, then_label
          compile_block( _case[1] )
          emit :jump, end_label
          emit :label, else_label
        end
        compile_block( default )
        emit :label, end_label

        # TODO: 一時的に、public/privateの切り替えを可能にしている。そのうち消すこと
      when :public
        @module.current_scope = :public

      when :private
        @module.current_scope = :private

      else
        #:nocov:
        raise "unknow op #{ast}"
        #:nocov:
      end
    end

    ############################################
    # 定数式の評価
    ############################################

    # 定数の評価.
    # Valueオブジェクト もしくは Array(AST) を返す.
    # 再帰的に評価し、定数として評価できなかったものは、ASTのまま返す.
    # このメソッドで AST からは、Numeric, Symbol, String はなくなる.
    def const_eval( ast )
      r = ast

      case ast

      when Numeric
        r = Value.new_int( ast )

      when Symbol
        r = @scope.find!( ast )

      when String
        r = const_eval( [:array, ast.unpack('c*') +[0] ] )
        r.base_string = ast

      when Array
        case ast[0]
        when :array
          val = ast[1].map{|v| const_eval(v) }
          type = val.map(&:type).inject { |a,b| type = TypeUtil.compatible_type(a,b) }
          r = Value.new( :array_literal, :"$#{tmp_count}", Type[[:array,val.size,type]], val, nil )

        when :incbin
          data = IO.binread( Fc.find_module( ast[1] ) )
          r = const_eval([:array, data.unpack('C*') ])

        when :lambda
          _, type, block, opt = *ast
          opt ||= {}
          raise CompileError.new('must be lambda type') unless type[0] == :lambda
          _, args, base_type = *type
          args = type[1].map { |arg| [arg[0], Type[arg[1]]] }
          arg_types = args.map { |arg| arg[1] }
          base_type = Type[ base_type ]
          if opt[:symbol]
            id = opt[:symbol]
          elsif opt[:id] == :main
            id = :_main
          elsif opt[:id]
            id = "_#{@module.id}_#{opt[:id]}".to_sym
          else
            id = "$#{tmp_count}".to_sym
          end
          opt[:extern] = true unless block
          lmd = Lambda.new( id, args, base_type, opt, block )
          @module.lambdas << lmd
          add_def_module(id, :code, lmd.type, lmd)
          r = Value.new( :literal, nil, lmd.type, id, nil )

        when :dot
          left, lv = const_eval(ast[1])
          raise unless Module === left.val
          r = left.val.scope.find!(ast[2])

        when :add, :sub, :mul, :div, :mod, :eq, :ne, :lt, :gt, :le, :ge, :rsh, :lsh, 
          :and, :or, :xor, :land, :lor, :not, :uminus, :shift_left, :shift_right
          ast[1] = const_eval( ast[1] )
          ast[2] = const_eval( ast[2] ) if ast[2]
          if (Value === ast[1] and ast[1].kind == :literal and Numeric === ast[1].val) and
              (ast[2].nil? or ( Value === ast[2] and ast[2].kind == :literal and Numeric === ast[2].val) )
            v1 = ast[1].val
            v2 = ast[2].val if ast[2]
            case ast[0]
            when :add then n = v1 + v2
            when :sub then n = v1 - v2
            when :mul then n = v1 * v2
            when :div then n = v1 / v2
            when :mod then n = v1 % v2
            when :eq  then n = v1 == v2
            when :ne  then n = v1 != v2
            when :lt  then n = v1 < v2
            when :gt  then n = v1 > v2
            when :le  then n = v1 <= v2
            when :ge  then n = v1 >= v2
            when :and then n = v1 & v2
            when :or  then n = v1 | v2
            when :xor then n = v1 ^ v2
            when :land then n = (v1!=0 && v2!=0 )
            when :lor then n = (v1!=0 || v2!=0 )
            when :not then n = (v1==0)
            when :uminus then n = -v1
            when :shift_left then n = v1 << v2
            when :shift_right then n = v1 >> v2
            else
              #:nocov:
              raise
              #:nocov:
            end
            n = 1 if n === true
            n = 0 if n === false
            r = Value.new_int( n )
          end
        when :call
          ast[1] = const_eval( ast[1] )
          ast[2] = ast[2].map {|exp| const_eval(exp)}
        when :load, :index
          1.upto(ast.size-1) do |i|
            ast[i] = const_eval( ast[i] )
          end
        when :cast
          ast[1] = const_eval( ast[1] )
          ast[2] = type_eval( ast[2] )
          r = Value.new( :literal, nil, ast[2], ast[1].val, nil ) if Value === ast[1] and ast[1].kind == :literal and Numeric === ast[1].val
        when :ref, :deref
          ast[1] = const_eval( ast[1] )
        else
          #:nocov:
          raise "invalid op #{ast}"
          #:nocov:
        end
      end
      r
    end

    # 型を評価する
    # astで指定された ASTオブジェクト から評価された Type を返す。
    def type_eval( ast )
      if Array === ast and ast[0] === :array
        size = const_eval(ast[1])
        size = size.val if size
        Type[ [:array, size, ast[2]] ]
      elsif ast
        Type[ast]
      else
        nil
      end
    end

    ############################################
    # 式のコンパイル
    ############################################

    # 右辺値として評価し、Valueを返す.
    def rval( ast )
      v, left = lval( ast )
      if left
        r = new_tmp( v.type.base )
        emit :pget, r, v
        r
      else
        v
      end
    end

    # 左辺値として評価し、[Value, 左辺値かどうか] を返す.
    # 左辺値とは、*,[]演算子を受けた値が、左辺の役割をするか右辺の役割をするかを表す
    def lval( ast )
      left_value = false
      ast = const_eval( ast )

      case ast

      when Value
        if ast.kind == :array_literal
          symbol = add_def("_#{tmp_count}".to_sym, :block, ast.type, ast.val)
          r = Value.new( :global, "$#{tmp_count}", ast.type, symbol, nil )
        else
          r = ast
        end

      when Array
        case ast[0]

        when :load
          left, lv = lval(ast[1])
          right = rval(ast[2])
          if lv
            TypeUtil.compatible_type( left.type.base, right.type )
            right = cast( right, left.type.base )
            emit :pset, left, right
            r = left
            left_value = true
          else
            TypeUtil.compatible_type( left.type, right.type )
            raise CompileError.new("#{left} is not left value") unless left.assignable?
            right = cast( right, left.type )
            emit :load, left, right
            r = left
          end

        when :not, :uminus
          left = rval(ast[1])
          r = new_tmp( left.type )
          emit ast[0], r, left

        when :add, :sub, :mul, :div, :mod, :and, :or, :xor, :shift_left, :shift_right
          left = rval(ast[1])
          right = rval(ast[2])
          begin
            type, left, right = make_compatible( left, right )
          rescue CompileError => err
            if [:add,:sub].include?(ast[0]) and left.type.kind == :pointer and right.type.kind == :int
              type = left.type
            else
              raise
            end
          end
          r = new_tmp( type )
          emit ast[0], r, left, right

        when :eq, :lt
          left = rval(ast[1])
          right = rval(ast[2])
          _, left, right = make_compatible( left, right )
          r = new_tmp( Type[:int] )
          emit ast[0], r, left, right

        when :ne, :gt, :le, :ge
          # これらは、eq,lt の引数の順番とnotを組合せて合成する
          left = ast[1]
          right = ast[2]
          case ast[0]
          when :ne
            r = rval([:not, [:eq, left, right]])
          when :gt
            r = rval([:lt, right, left])
          when :le
            r = rval([:not, [:lt, right, left]])
          when :ge
            r = rval([:not, [:lt, left, right]])
          end

        when :land
          end_label = new_label('end')
          r = new_tmp( Type[:int] )
          left = rval(ast[1])
          emit :load, r, left
          emit :if, r, end_label
          right = rval(ast[2])
          emit :load, r, right
          emit :label, end_label

        when :lor
          end_label = new_label('end')
          r = new_tmp( Type[:int] )
          r2 = new_tmp( Type[:int] )
          left = rval(ast[1])
          emit :load, r, left
          emit :not, r2, r
          emit :if, r2, end_label
          right = rval(ast[2])
          emit :load, r, right
          emit :label, end_label

        when :call

          lmd = rval( ast[1] )
          if lmd.type.kind == :macro
            # マクロの実行
            # ast[2].map!{ |x| rval(x) } # TODO: マクロ前に評価するか考える
            x = self.instance_exec( ast[2], ast[3], &lmd.val )
            if x
              if x[0] == :block
                compile_block(x[1..-1]) 
              else
                r = rval(x) 
              end
            end
          else
            # 普通の関数コール
            if lmd.type.base != Type[:void]
              r = new_tmp( lmd.type.base )
            else
              r = nil
            end
            raise CompileError.new("#{lmd} has #{lmd.type.args.size} but #{ast[2].size}") if ast[2].size != lmd.type.args.size
            raise CompileError.new("cannot call function from fastcall") if @lmd.type.fastcall?
            
            if lmd.type.fastcall?
              raise CompileError.new("cannot fastcall in fastcalling") if @fast_calling
              @fast_calling = true
              emit :push_fastcall_result, lmd.type.base
              ast[2].each_with_index do |arg,i|
                v = rval(arg)
                TypeUtil.compatible_type( lmd.type.args[i], v.type )
                v = cast( v, lmd.type.args[i] )
                emit :push_fastcall_arg, lmd.type.args[i], v
              end
              emit :fastcall, r, lmd
              @fast_calling = false
            else
              emit :push_result, lmd.type.base
              ast[2].each_with_index do |arg,i|
                v = rval(arg)
                TypeUtil.compatible_type( lmd.type.args[i], v.type )
                v = cast( v, lmd.type.args[i] )
                emit :push_arg, lmd.type.args[i], v
              end
              emit :call, r, lmd
            end
          end

        when :ref # &演算子
          left, lv = lval(ast[1])
          if lv
            r = left
          else
            raise CompileError.new("#{left} is not left value") unless left.assignable?
            r = new_tmp( Type[[:pointer, left.type]] )
            emit :ref, r, left
          end

        when :deref # *演算子
          r = rval(ast[1])
          raise CompileError.new("#{left} is not pointer") unless r.type.kind == :pointer
          left_value = true

        when :index # []演算子
          left = rval(ast[1])
          right = rval(ast[2])
          raise CompileError.new("index must be pointer or array") unless left.type.kind == :pointer or left.type.kind == :array
          raise CompileError.new("index must be int") if right.type.kind != :int
          r = new_tmp( Type[[:pointer, left.type.base]] )
          emit :index, r, left, right
          left_value = true

        when :cast
          r = CastedValue.new( rval( ast[1] ), Type[ast[2]], 0 )

        else
          #:nocov:#
          raise "unknown op #{ast}"
          #:nocov:#
        end
      else
        #:nocov:#
        raise "unknown op #{ast}"
        #:nocov:#
      end
      [r,left_value]
    end


    def emit( *op )
      @lmd.ops << op
    end

    def new_label( name )
      new_labels( name )[0]
    end

    def new_labels( *names )
      r = names.map { |n| '@'+n+'_'+tmp_count.to_s }
    end

    def new_tmp( type )
      add_var Value.new(:local, "$#{tmp_count}".to_sym, type, nil, {local_type: :temp} )
    end

    # キャストする
    # v(Value) を type(Type) で指定された型にキャストする
    def cast( v, type )
      if type.kind == :int
        # int の変換
        return v if type == v.type
        return v if type.size <= v.type.size
        return v unless v.type.signed
        return v if v.kind == :literal
        new_v = new_tmp( type )
        emit :sign_extension, new_v, v
        new_v
      elsif type.kind == :pointer and v.type.kind == :array and v.type.base == type.base
        PointeredArray.new(v)
      else
        v
      end
    end

    # 型aとbが互換性のある場合、変換先の型を取得する.
    # キャストが必要な場合は、キャストするコードも生成する
    def make_compatible( a, b )
      type = TypeUtil.compatible_type( a.type, b.type )
      a = cast( a, type )
      b = cast( b, type )
      return [type, a, b]
    end

  end

  ############################################
  # HTML出力用クラス
  ############################################
  class HtmlOutput
    require 'cgi'

    def initialize
    end

    def h( s )
      CGI.escapeHTML( s.to_s )
    end

    def module_to_html( mods )
      path = Fc.find_share('main.html.erb')
      @template_main ||= File.read( path )
      erb = ERB.new(@template_main,trim_mode:'-')
      erb.filename = path.to_s
      erb.result(binding)
    end

  end

end
