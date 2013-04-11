# coding: utf-8
require 'digest/md5'
require_relative 'allocator'

module Fc
  ######################################################################
  # Low-Level コンパイラ( 中間コード -> アセンブリ へのコンパイルを行う )
  ######################################################################
  class Llc
    attr_reader :asm, :char_banks, :prog_banks
    attr_accessor :prog_bank_count

    def initialize
      @debug = DEBUG_LEVEL
      @module = nil
      @label_count = 0
      @asm = []
      @prog_banks = [] # プログラムバンクごとのアセンブラ
      @char_banks = [] # キャラクタバンクごとのアセンブラ
      @prog_bank_count = nil
    end

    def compile( mods )
      @prog_bank_count.times do |i|
        @prog_banks[i] = [nil,[]]
      end
      @modules = mods
      @modules.each do |k,mod|
        compile_module( mod )
      end
    end

    def dout( level, *args )
      #:nocov:
      if @debug >= level
        puts args.join(" ")
      end
      #:nocov:
    end

    def compile_module( mod )
      bank = mod.options[:bank].to_i
      if bank < 0
        bank = @prog_bank_count + bank
      end
      @bank = bank

      org = mod.options[:org]
      @prog_banks[bank][0] = org if org

      # モジュール定数のコンパイル
      mod.blobs.each do |blob|
        @prog_banks[bank][1].concat emit_blob(blob)
      end

      # モジュール変数のコンパイル
      mod.vars.each do |v|
        case v.kind
        when :global_var
          if v.opt[:address]
            @asm << "#{to_asm(v)} = #{v.opt[:address]}"
          else
            @asm << "#{to_asm(v)}: .ds  #{v.type.size}"
          end
        when :global_const
          if Numeric === v.val
            @asm << "#{to_asm(v)} = #{v.val}"
          end
        end
      end

      # lambdaのコンパイル
      mod.lambdas.each do |lmd|
        next if lmd.opt[:extern]
        @prog_banks[bank][1] << compile_lambda( lmd )
        @prog_banks[bank][1] << ''
      end

      # include(.asm)の処理
      mod.include_asms.each do |file|
        @prog_banks[bank][1] << "\t.include \"#{file}\""
      end

      # include(.chr)の処理
      mod.include_chrs.each do |file|
        @char_banks << "\t.incbin \"#{file}\""
      end

    end

    def compile_lambda( lmd )
      # block.optimized_ops = Marshal.load(Marshal.dump(ops))
      # ops = optimize( block, ops )
      # block.optimized_ops = ops
      alloc_register( lmd )
      ops = lmd.ops
      ops = optimize_pointer( lmd, ops ) if true
      lmd.ops = ops

      r = []

      r << ";;;============================="
      r << ";;; function #{lmd.id}" 
      r << ";;;============================="

      r << "#{to_asm(lmd)}:" 

      ops.each_with_index do |op,op_no| # op=オペランド
        dout 3, op.inspect
        next unless op
        r << "; #{'%04d'%[op_no]}: #{op.inspect}"
        case op[0]

        when :label
          r << op[1] + ':'

        when :if
          then_label = new_label
          op[1].type.size.times do |i|
            r << load_a( op[1],i)
            r << "bne #{then_label}"
          end
          r << "jmp #{op[2]}"
          r << "#{then_label}:"

        when :jump
          r << "jmp #{op[1]}"

        when :return
          r.concat load( lmd.result, op[1] ) if op[1]
          r << "rts"

        when :call
          size = op[2].type.base.size
          op[3..-1].each_with_index do |from,i|
            to = op[2].type.args[i]
            # 通常の代入
            # raise "can't convert from #{from} to #{to}" if from.type != to
            to.size.times do |i|
              r << load_a(from,i)
              r << "sta S+#{lmd.frame_size}+#{size},x"
              size += 1
            end
          end
          if Lambda === op[2].val
            # 関数を直に呼ぶ
            r << "call #{to_asm(op[2].val.id)}, ##{lmd.frame_size}"
          else
            # 関数ポインタから呼ぶ
            end_label = new_label
            r << load_a( op[2], 0 )
            r << "sta reg+0"
            r << load_a( op[2], 1 )
            r << "sta reg+1"
            r << "call jsr_reg, ##{lmd.frame_size}"
          end
          # 帰り値を格納する
          if op[1]
            op[1].type.size.times do |i|
              r << "lda #{i}+S+#{lmd.frame_size},x"
              r << store_a(op[1],i);
            end
          end

        when :load
          r.concat load( op[1], op[2] )

        when :sign_extension
          pls_label, end_label = new_labels(2)
          # TODO: サイズ1->2以上の場合を実装すること、いまはそれしかないから十分だけど
          r << load_a(op[2],0)
          r << store_a(op[1],0)
          r << "bpl #{pls_label}"
          r << "lda #255"
          r << "jmp #{end_label}"
          r << "#{pls_label}:"
          r << "lda #0"
          r << "#{end_label}:"
          r << store_a(op[1],1)

        when :add
          op[1].type.size.times do |i|
            r << "clc" if i == 0
            r << load_a( op[2],i)
            r << "adc #{byte(op[3],i)}"
            r << store_a(op[1],i)
          end

        when :sub
          op[1].type.size.times do |i|
            r << "sec" if i == 0
            r << load_a( op[2],i)
            r << "sbc #{byte(op[3],i)}"
            r << store_a(op[1],i)
          end

        when :and, :or, :xor
          op[1].type.size.times do |i|
            r << load_a( op[2],i)
            as = {and:'and', or:'ora', xor:'eor'}[op[0]]
            r << "#{as} #{byte(op[3],i)}"
            r << store_a(op[1],i)
          end

        when :mul, :div, :mod
          r.concat mul_div_mod( op )

        when :uminus
          raise if op[1].type.kind != :int
          op[1].type.size.times do |i|
            r << "sec" if i == 0
            r << "lda #0"
            r << "sbc #{byte(op[2],i)}" if op[2].type.size > i
            r << store_a(op[1],i)
          end

        when :eq
          false_label, end_label = new_labels(2)
          [ op[2].type.size, op[3].type.size ].max.times do |i|
            r << load_a( op[2],i)
            r << "cmp #{byte(op[3],i)}" 
            r << "bne #{false_label}"
          end
          # falseのとき
          r << "lda #1"
          r << store_a(op[1],0)
          r << "jmp #{end_label}"
          # trueのとき
          r << "#{false_label}:"
          r << "lda #0"
          r << store_a(op[1],0)
          r << "#{end_label}:"

        when :lt
          true_label, end_label = new_labels(2)
          ([op[2].type.size,op[3].type.size].max-1).downto(0) do |i|
            r << load_a( op[2],i)
            r << "cmp #{byte(op[3],i)}" 
            r << "bcc #{true_label}"
          end
          # falseのとき
          r << "lda #0"
          r << store_a(op[1],0)
          r << "jmp #{end_label}"
          # trueのとき
          r << "#{true_label}:"
          r << "lda #1"
          r << store_a(op[1],0)
          r << "#{end_label}:"

        when :'not'
          true_label, end_label = new_labels(2)
          op[2].type.size.times do |i|
            r << load_a( op[2],i)
            r << "beq #{true_label}"
          end
          # falseのとき
          r << "lda #0"
          r << store_a(op[1],0)
          r << "jmp #{end_label}"
          # trueのとき
          r << "#{true_label}:"
          r << "lda #1"
          r << store_a(op[1],0)
          r << "#{end_label}:"

        when :asm
          r << op[1]

        when :index
          raise CompileError.new("2byte index not supported") if op[3].type != Type[:int] and op[3].type != Type[:sint8]
          if op[2].type.kind == :array
            r.concat load_y_idx(op[3],op[2])
            r << "sty reg+0"
            r << "clc"
            r << "lda #LOW(#{to_asm(op[2])})"
            r << "adc reg+0"
            r << "sta 0+#{to_asm(op[1])}"
            r << "lda #HIGH(#{to_asm(op[2])})"
            r << "adc #0"
            r << "sta 1+#{to_asm(op[1])}"
          elsif op[2].type.kind == :pointer
            r.concat load_y_idx(op[3],op[2])
            r << "sty reg+0"
            r << "clc"
            r << "lda #{byte(op[2],0)}"
            r << "adc reg+0"
            r << "sta #{byte(op[1],0)}"
            r << "lda #{byte(op[2],1)}"
            r << "adc #0"
            r << "sta #{byte(op[1],1)}"
          else
            #:nocov:
            raise
            #:nocov:
          end

        when :ref
          r << "lda #LOW(#{to_asm(op[2])})"
          r << store_a( op[1], 0 )
          r << "lda #HIGH(#{to_asm(op[2])})"
          r << store_a( op[1], 1 )

        when :pget
          r << "lda #{byte(op[2],0)}"
          r << "sta reg+0"
          r << "lda #{byte(op[2],1)}"
          r << "sta reg+1"
          op[1].type.size.times do |i|
            r << "ldy ##{i}"
            r << "lda [reg],y"
            r << store_a(op[1],i)
          end

        when :pset
          r << "lda #{byte(op[1],0)}"
          r << "sta reg+0"
          r << "lda #{byte(op[1],1)}"
          r << "sta reg+1"
          op[1].type.base.size.times do |i|
            r << load_a( op[2],i)
            r << "ldy ##{i}"
            r << "sta [reg],y"
          end

          # 最適化後のオペレータ
        when :index_pget
          raise CompileError.new("2byte index not supported") if op[3].type != Type[:int] and op[3].type != Type[:sint8]
          raise if op[2].type.kind != :array
          r.concat load_y_idx(op[3],op[2])
          op[1].type.size.times do |i|
            r << "lda #{to_asm(op[2])}+#{i},y"
            r << store_a(op[1],i)
          end

        when :index_pset
          raise CompileError.new("2byte index not supported") if op[2].type != Type[:int] and op[2].type != Type[:sint8]
          raise if op[1].type.kind != :array
          r.concat load_y_idx(op[2],op[1])
          op[3].type.size.times do |i|
            r << load_a( op[3],i)
            r << "sta #{to_asm(op[1])}+#{i},y"
          end

        else
          #:nocov:
          raise "unknow op #{op}"
          #:nocov:
        end
      end

      lmd.blobs.each do |blob|
        r.concat emit_blob( blob )
      end

      # 空の行を削除
      r.delete(nil)
      # ラベル行,コメント行以外はインデントする
      r = r.map do |line|
        if line.index(':') and line[0] != ';'
          line
        else
          "\t"+line
        end
      end

      lmd.asm = r

      r
    end

    def load_y_idx( idx, ptr )
      r = []
      if ptr.type.base.size == 1
        r << "ldy #{byte(idx,0)}"
      else
        r << "lda #{byte(idx,0)}"
        (ptr.type.base.size-1).times { r << 'asl a'}
        r << "tay"
      end
      r
    end

    def load( to, from )
      r = []
      if to.type.kind == :pointer and from.type.kind == :array 
        raise "can't convert from #{from} to #{to}" unless from.type.base == to.type.base
        # 配列からポインタに変換
        r << "lda #LOW(#{to_asm(from)})"
        r << "sta #{byte(to,0)}"
        r << "lda #HIGH(#{to_asm(from)})"
        r << "sta #{byte(to,1)}"
      else
        # 通常の代入
        if from.type.kind != :int
          raise "can't convert from #{from} to #{to}" unless from.type.base == to.type.base
        end
        to.type.size.times do |i|
          r << load_a(from,i)
          r << store_a(to,i)
        end
      end
      r
    end

    def load_a( v, n )
      if Value === v and v.location == :a
        ''
      else
        return "lda #{byte(v,n)}"
      end
    end

    def store_a( v, n )
      if Value === v and v.location == :a
        ''
      else
        return  "sta #{byte(v,n)}"
      end
    end

    def new_label
      @label_count += 1
      "._#{@label_count}"
    end

    def new_labels( n )
      Array.new(n){new_label }
    end

    def to_asm( v )
      if Value === v or CastedValue === v
        case v.kind
        when :var, :arg, :result, :temp
          case v.location
          when :frame then "S+#{v.address},x"
          when :reg then "L+#{v.address}"
          else 
            # :nocov:
            raise "invalid location #{v.location} of #{v}"
            # :nocov:
          end
        when :symbol, :const, :array_literal
          '.'+mangle(v.id)
        when :global_symbol
          mangle(v.id)
        when :global_const, :global_var
          mangle v.id
        when :literal
          "##{v.val}"
        else
          #:nocov:
          raise "invalid v #{v}"
          #:nocov:
        end
      elsif Symbol === v
        mangle v
      elsif Lambda === v
        mangle v.id
      else
        #:nocov:
        raise "invalid v = #{v}"
        #:nocov:
      end
    end

    # 名前をアセンブラ用の表現に変更する
    def mangle(str)
      '_'+str.to_s.gsub(/\$/){'_D'}
    end

    # 値からn番目のbyteを取得する
    def byte( v, n )
      if PointeredArray === v
        case n
        when 0 then "#LOW(#{to_asm(v.from)})"
        when 1 then "#HIGH(#{to_asm(v.from)})"
        else 
          #:nocov:
          raise
          #:nocov:
        end
      elsif v.val
        if Numeric === v.val
          "##{(v.val >> (n*8)) % 256}"
        else
          case n
          when 0; "#LOW(#{to_asm(v.val)})"
          when 1; "#HIGH(#{to_asm(v.val)})"
          else
            #:nocov:
            raise
            #:nocov:
          end
        end
      else
        if n < v.type.size
          "#{n}+#{to_asm(v)}"
        else
          "#0" # 符号拡張は、:sign_extension オペレータで行うので、存在しないbyteは0扱い
        end
      end
    end

    # v を .db/.dw に変換する
    def emit_blob( v )
      r = []
      base_string = ( v.base_string ? (' ; '+v.base_string.inspect) : '' )
      if v.kind == :symbol or v.kind == :array_literal
        r << ".#{to_asm(v.id)}:" + base_string
      else
        r << "#{to_asm(v.id)}:" + base_string
      end
      case v.type.base.size
      when 1; op = '.db'
      when 2; op = '.dw'
      else
        #:nocov:
        raise
        #:nocov:
      end
      v.val.each_slice(16) do |slice|
        slice.map! do |e|
          if Numeric === e.val then e.val else to_asm(e) end
        end
        r << "\t#{op} #{slice.join(',')}"
      end
      r
    end

    ############################################
    # 一部の複雑なオペレータ用
    ############################################

    # mul, div, mod のコード生成を行う
    # 定数の場合を最適化する
    # TODO: ２の累乗だけでなく、他の場合にも対応できるはず
    def mul_div_mod( op )
      r = []
      op3val = op[3].val
      if Numeric === op3val and [0,1,2,4,8,16,32,64,128].include?(op3val) and op[1].type.size == 1
        # 定数(1byte)の場合の最適化
        n = Math.log(op3val,2).to_i
        r << load_a( op[2], 0 )
        case op[0]
        when :mul
          n.times { r << "asl a" }
        when :div
          if op[2].type.signed
            negative_label, end_label = new_labels(2)
            r << "bmi #{negative_label}"
            n.times { r << "lsr a" }
            r << "jmp #{end_label}"
            r << "#{negative_label}:"
            n.times { r << "lsr a" }
            r << "ora ##{256-2**(8-n)}"
            r << "#{end_label}:"
          else
            n.times { r << "lsr a" }
          end
        when :mod
          r << "and ##{op3val-1}"
        end
        r << store_a( op[1], 0 )
      elsif Numeric === op3val and [0,1,2,4,8,16,32,64,128].include?(op3val) and op[1].type.size > 1
        # 定数(2byte以上)の場合の最適化
        n = Math.log(op3val,2).to_i
        size = op[1].type.size
        case op[0]
        when :mul
          r.concat load( op[1], op[2] )
          n.times do 
            size.times do |i| 
              rot = ( i == 0 ? 'asl' : 'rol' )
              r << "#{rot} #{byte(op[1],i)}" 
            end
          end
        when :div
          r.concat load( op[1], op[2] )
          n.times do 
            (size-1).downto(0) do |i| 
              if op[1].type.signed
                rot = 'ror'
                r << 'cmp $80' if i == 0
              else
                rot = ( i == size-1 ? 'lsr' : 'ror' )
              end
              r << "#{rot} #{byte(op[1],i)}"
            end
          end
        when :mod
          size.times do |i| 
            r << load_a( op[2], i )
            r << "and ##{((op3val-1)>>(i*8))%256}"
            r << store_a( op[1], i )
          end
        end
      else
        # 定数でない場合
        op[1].type.size.times do |i|
          r << load_a(op[2],i)
          r << "sta reg+0+#{i}"
          r << load_a(op[3],i)
          r << "sta reg+2+#{i}"
        end
        if op[1].type.size == 1
          if op[1].type.signed
            r << "jsr __#{op[0]}_8s"
          else
            r << "jsr __#{op[0]}_8"
          end
        else
          r << "jsr __#{op[0]}_16"
        end
        op[1].type.size.times do |i|
          r << "lda reg+4+#{i}"
          r << store_a(op[1],i)
        end
      end
      r
    end

    ############################################
    # レジスター割り当て
    ############################################
    def alloc_register( lmd )

      if true
        Fc.allocate_register( lmd )
        delete_unuse( lmd )
      else
        # 単純なバージョンのアロケータ(debug用)
        #:nocov:
        size = 0
        lmd.vars.each do |var|
          if var.on_stack?
            var.location = :frame
            var.address = size
            size += var.type.size
          end
        end
        lmd.frame_size = size
        #:nocov:
      end
    end

    ############################################
    # 使っていない変数の削除
    ############################################
    def delete_unuse( lmd )
      lmd.ops.each_with_index do |op,i|
        case op[0]
        when :pget, :load
          lmd.ops[i] = nil if op[1] and op[1].unuse
        when :call
          lmd.ops[i][1] = nil if op[1] and op[1].unuse
        end
      end
    end

    ############################################
    # オプティマイザ
    ############################################

    def optimize_pointer( lmd, ops )

      ops = ops.clone

      # index + pget/pset の最適化
      ops.each_with_index do |op,i|
        next unless op
        case op[0] 
        when :index
          next_op = ops[i+1]
          next unless next_op
          case next_op[0]
          when :pget 
            if op[1] == next_op[2] and # 同じ変数を連続で使っていて
                ( op[2].kind == :global_symbol or op[2].kind == :global_var ) and
                op[1].kind == :temp # その変数をそこでしか使っていない
              # puts "replace #{op}, #{next_op}"
              ops[i] = [:index_pget, next_op[1], op[2], op[3]]
              ops[i+1] = nil
            end
          when :pset
            if op[1] == next_op[1] and # 同じ変数を連続で使っていて
                ( op[2].kind == :global_symbol or op[2].kind == :global_var ) and
                op[1].kind == :temp # その変数をそこでしか使っていない
              # puts "replace #{op}, #{next_op}"
              ops[i] = [:index_pset, op[2], op[3], next_op[2]]
              ops[i+1] = nil
            end
          end
        end
      end
      ops
    end

  end
end
