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

    def initialize( opt = {} )
      @opt = { optimize_level: 2 }.merge( opt )
      @debug = DEBUG_LEVEL
    end

    def dout( level, *args )
      #:nocov:
      if @debug >= level
        puts args.join(" ")
      end
      #:nocov:
    end

    def compile( mod )
      @label_count = 0
      @code_segment = mod.id.to_s

      inc = []
      asm = []
      asm << "\t.setcpu \"6502\""
      asm << "\t.include \"macro.inc\""
      asm << "__MODULE_#{mod.id.upcase}__ = 1"

      inc << ".ifndef __MODULE_#{mod.id.upcase}__"
      inc << "__MODULE_#{mod.id.upcase}__ = 1"

      # 
      mod.modules.each do |_,m|
        inc << "\t.include \"_#{m.id}.inc\""
        asm << "\t.include \"_#{m.id}.inc\""
      end

      # include(.asm)の処理
      mod.include_asms.each do |file|
        asm << "\t.include \"#{file.basename}\""
      end

      mod.defs.each do |sym, kind, type, val|
        case kind
        when :equ
          val = mangle(val) if Symbol === val
          inc << "#{mangle(sym)} = #{val}"
          asm << "#{mangle(sym)} = #{val}"
        when :bss
          inc << "\t.import #{mangle(sym)}"
          asm << "\t.export #{mangle(sym)}"
          asm << ".segment \"BSS\""
          asm << "#{mangle(sym)}: .res #{type.size}"
        when :block
          inc << "\t.import #{mangle(sym)}"
          asm << "\t.export #{mangle(sym)}"
          asm << ".segment \"#{@code_segment}\""
          asm << emit_block(sym,type,val)
        when :code
          inc << "\t.import #{mangle(sym)}"
          asm << "\t.export #{mangle(sym)}"
          lmd = val
          next if lmd.opt[:extern]
          asm << compile_lambda( sym, lmd )
        else
          raise
        end
      end

      # include header(.asm)の処理
      mod.include_headers.each do |file|
        asm << "\t.include \"#{file}\""
      end

      # include(.chr)の処理
      mod.include_chrs.each do |file|
        asm << ".segment \"CHARS\""
        asm << "\t.incbin \"#{file}\""
      end

      inc << ".endif"

      [asm,inc]
    end

    def compile_lambda( sym, lmd )
      # block.optimized_ops = Marshal.load(Marshal.dump(ops))
      # ops = optimize( block, ops )
      # block.optimized_ops = ops
      alloc_register( lmd )
      ops = lmd.ops
      if @opt[:optimize_level] > 0
        ops = optimize_pointer( lmd, ops )
      end
      lmd.ops = ops

      r = []

      r << ";;;============================="
      r << ";;; function #{lmd.id}" 
      r << ";;;============================="

      r << ".segment \"#{@code_segment}\""
      r << ".proc #{mangle(sym)}" 

      ops.each_with_index do |op,op_no| # op=オペランド
        dout 3, op.inspect
        next unless op
        r << "; #{'%04d'%[op_no]}: #{op.inspect}"
        case op[0]

        when :label
          r << op[1] + ':'

        when :if
          if op[1].location == :cond
            # コンディションレジスタの場合
            asm_op = nil
            case op[1].cond_reg
            when :zero then asm_op = ( op[1].cond_positive ? 'bne' : 'beq' )
            when :carry then asm_op = ( op[1].cond_positive ? 'bcs' : 'bcc' )
            when :negative then asm_op = ( op[1].cond_positive ? 'bpl' : 'bmi' )
            else raise
            end
            r << "#{asm_op} #{op[2]}"
          else
            then_label = new_label
            # コンディションレジスタでない場合
            op[1].type.size.times do |i|
              r << load_a( op[1],i)
              r << "bne #{then_label}"
            end
            r << "jmp #{op[2]}"
            r << "#{then_label}:"
          end

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
              r << "sta <S+#{lmd.frame_size}+#{size},x"
              size += 1
            end
          end
          if op[2].kind == :literal
            # 関数を直に呼ぶ
            r << "call #{mangle(op[2].val)}, ##{lmd.frame_size}"
          else
            # 関数ポインタから呼ぶ
            end_label = new_label
            r << load_a( op[2], 0 )
            r << "sta <reg+0"
            r << load_a( op[2], 1 )
            r << "sta <reg+1"
            r << "call jsr_reg, ##{lmd.frame_size}"
          end
          # 帰り値を格納する
          if op[1]
            op[1].type.size.times do |i|
              r << "lda <#{i}+S+#{lmd.frame_size},x"
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

        when :shift_left, :shift_right
          signed = op[2].type.signed
          rotate = (op[0] == :shift_left) ? 'rol' : 'ror'
          if Numeric === op[3].val
            # 定数の場合
            if op[1].type.size == 1
              # サイズが1
              r << load_a(op[2],0)
              op[3].val.times do
                if signed
                  r << "cmp #128"
                else
                  r << "clc"
                end
                r << "#{rotate} a"
              end
              r << store_a(op[1],0)
            else
              # サイズが２以上
              r << load( op[1], op[2] )
              op[3].val.times do
                if op[0] == :shift_left
                  # 左シフト
                  op[1].type.size.times do |i|
                    r << load_a(op[1],i)
                    if i == 0
                      r << "clc"
                    else
                    end
                    r << "rol a"
                    r << store_a(op[1],i)
                  end
                else
                  # 右シフト
                  (op[1].type.size-1).downto(0) do |i|
                    r << load_a(op[1],i)
                    if i == op[1].type.size-1
                      if signed
                        r << "cmp #128"
                      else
                        r << "clc"
                      end
                    end
                    r << "ror a"
                    r << store_a(op[1],i)
                  end
                end
              end
            end
          else
            # 定数でない場合
            # TODO: もうちょっと整理して効率よくできるはず
            raise if op[1].type.size != 1
            loop_label, end_label = new_labels(2)
            r << load_a(op[3],0)
            r << "tay"
            r << load_a(op[2],0)
            r << "#{loop_label}:"
            r << "cpy #0"
            r << "beq #{end_label}"
            if signed
              r << "cmp #128"
            else
              r << "clc"
            end
            r << "#{rotate} a"
            r << "dey"
            r << "jmp #{loop_label}"
            r << "#{end_label}:"
            r << store_a(op[1],0)
          end

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
          size = [ op[2].type.size, op[3].type.size ].max
          size.times do |i|
            r << load_a( op[2],i)
            r << "cmp #{byte(op[3],i)}" 
            r << "bne #{false_label}"
          end
          if op[1].location == :cond
            r.pop # 最後のbneを消す
            r << "#{false_label}:" if size != 1
          else
            # falseのとき
            r << "lda #1"
            r << store_a(op[1],0)
            r << "jmp #{end_label}"
            # trueのとき
            r << "#{false_label}:"
            r << "lda #0"
            r << store_a(op[1],0)
            r << "#{end_label}:"
          end

        when :lt
          true_label, end_label = new_labels(2)
          size = [op[2].type.size, op[3].type.size].max
          signed = op[2].type.signed or op[3].type.signed
          (size-1).downto(0) do |i|
            r << load_a( op[2],i)
            r << "cmp #{byte(op[3],i)}"
            if signed and i == size-1
              r << "bmi #{true_label}"
            else
              r << "bcc #{true_label}"
            end
          end
          if op[1].location == :cond
            r.pop # 最後のbccを消す
            r << "#{true_label}:" if size != 1
          else
            # falseのとき
            r << "lda #0"
            r << store_a(op[1],0)
            r << "jmp #{end_label}"
            # trueのとき
            r << "#{true_label}:"
            r << "lda #1"
            r << store_a(op[1],0)
            r << "#{end_label}:"
          end

        when :'not'
          if op[1].location != :cond
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
          end

        when :asm
          r << op[1]

        when :index
          # raise CompileError.new("2byte index not supported") if op[3].type != Type[:int] and op[3].type != Type[:sint8]
          if op[3].type.size == 1
            # インデックスのサイズが１
            if op[2].type.kind == :array
              r.concat load_y_idx(op[3],op[2])
              r << "sty <reg+0"
              r << "clc"
              r << "lda #.LOBYTE(#{to_asm(op[2])})"
              r << "adc <reg+0"
              r << store_a(op[1],0)
              r << "lda #.HIBYTE(#{to_asm(op[2])})"
              r << "adc #0"
              r << store_a(op[1],1)
            elsif op[2].type.kind == :pointer
              r.concat load_y_idx(op[3],op[2])
              r << "sty <reg+0"
              r << "clc"
              r << load_a(op[2],0)
              r << "adc <reg+0"
              r << store_a(op[1],0)
              r << load_a(op[2],1)
              r << "adc #0"
              r << store_a(op[1],1)
            else
              #:nocov:
              raise
              #:nocov:
            end
          else
            # インデックスのサイズが２
            # TODO: ちゃんとする、テスト作る
            if op[2].type.kind == :array
              r << load_a(op[3],0)
              r << "sta <reg+0"
              r << load_a(op[3],1)
              r << "sta <reg+1"

              if op[2].type.base.size == 2 
                r << "clc"
                r << "rol <reg+0"
                r << "rol <reg+1"
              end

              r << "lda <reg+0"
              r << "clc"
              r << "adc #.LOBYTE(#{to_asm(op[2])})"
              r << store_a(op[1],0)
              r << "lda <reg+1"
              r << "adc #.HIBYTE(#{to_asm(op[2])})"
              r << store_a(op[1],1)
            elsif
              raise
            end
          end

        when :ref
          if op[2].location == :frame
            r << "txa"
            r << "clc"
            r << "adc #.LOBYTE(S+#{op[2].address})"
            r << store_a( op[1], 0 )
            r << "lda #0"
            r << store_a( op[1], 1 )
          else
            r << "lda #.LOBYTE(#{to_asm(op[2])})"
            r << store_a( op[1], 0 )
            r << "lda #.HIBYTE(#{to_asm(op[2])})"
            r << store_a( op[1], 1 )
          end

        when :pget
          r << "lda #{byte(op[2],0)}"
          r << "sta <reg+0"
          r << "lda #{byte(op[2],1)}"
          r << "sta <reg+1"
          op[1].type.size.times do |i|
            r << "ldy ##{i}"
            r << "lda (reg),y"
            r << store_a(op[1],i)
          end

        when :pset
          r << "lda #{byte(op[1],0)}"
          r << "sta <reg+0"
          r << "lda #{byte(op[1],1)}"
          r << "sta <reg+1"
          op[1].type.base.size.times do |i|
            r << load_a( op[2],i)
            r << "ldy ##{i}"
            r << "sta (reg),y"
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

      lmd.defs.each do |sym, kind, type, val|
        case kind
        when :block
          r.concat emit_block( sym, type, val )
        else
          raise
        end
      end

      
      r.flatten! # まとめた行を展開
      r.delete(nil) # 空の行を削除
      # ラベル行,コメント行以外はインデントする
      r = r.map do |line|
        if line =~ /\A([.@_\w][_\w\d]+:|\.segment|\.proc)/
          line
        else
          "\t"+line
        end
      end

      r << '.endproc'

      r = extend_jump(r)

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
        r << "lda #.LOBYTE(#{to_asm(from)})"
        r << "sta #{byte(to,0)}"
        r << "lda #.HIBYTE(#{to_asm(from)})"
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
      if Value === v and v.location == :cond
        raise if n != 0
        case v.cond_reg
        when :zero
          true_label, end_label = new_labels(2)
          ["#{ v.cond_positive ? 'beq' : 'bne' } #{true_label}",
           "lda #0",
           "jmp #{end_label}",
           "#{true_label}:",
           "lda #1",
           "#{end_label}:"]
        when :carry
          if v.cond_positive
            ["lda #0", "rol a", "eor #1"]
          else
            ["lda #0", "rol a"]
          end
        when :negative
          true_label, end_label = new_labels(2)
          ["#{ v.cond_positive ? 'bmi' : 'bpl' } #{true_label}",
           "lda #0",
           "jmp #{end_label}",
           "#{true_label}:",
           "lda #1",
           "#{end_label}:"]
        else raise
        end
      elsif Value === v and v.location == :a
        if n != 0
          "lda #0"
        else
          nil
        end
      else
        "lda #{byte(v,n)}"
      end
    end

    def store_a( v, n )
      if Value === v and v.location == :a
        raise if n != 0
        nil
      else
        "sta #{byte(v,n)}"
      end
    end

    def new_label
      @label_count += 1
      "@#{@label_count}"
    end

    def new_labels( n )
      Array.new(n){new_label }
    end

    def to_asm( v )
      if Value === v or CastedValue === v
        case v.kind
        when :local
          case v.location
          when :frame then "<S+#{v.address},x"
          when :reg then "<L+#{v.address}"
          else 
            # :nocov:
            raise "invalid location #{v.location} of #{v}"
            # :nocov:
          end
        when :global
          raise "invalid #{v}, #{v.val}" unless v.val
          mangle(v.val)
        when :literal
          "##{v.val}"
        else
          #:nocov:
          raise "invalid v #{v}"
          #:nocov:
        end
      elsif Symbol === v
        raise
        mangle v
      else
        #:nocov:
        raise "invalid v = #{v}"
        #:nocov:
      end
    end

    # 名前をアセンブラ用の表現に変更する
    def mangle(str)
      str.to_s.gsub(/\$/){'_D'}
    end

    # 値からn番目のbyteを取得する
    def byte( v, n )
      if PointeredArray === v
        case n
        when 0 then "#.LOBYTE(#{to_asm(v.from)})"
        when 1 then "#.HIBYTE(#{to_asm(v.from)})"
        else 
          #:nocov:
          raise
          #:nocov:
        end
      elsif v.kind == :literal
        case v.val
        when Numeric
          "##{(v.val >> (n*8)) % 256}"
        when Symbol
          case n
          when 0; "#.LOBYTE(#{mangle(v.val)})"
          when 1; "#.HIBYTE(#{mangle(v.val)})"
          else
            #:nocov:
            raise
            #:nocov:
          end
        else
          #:nocov:
          raise
          #:nocov:
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
    def emit_block( sym, type, val )
      r = []
      r << "#{mangle(sym)}:"
      case type.base.size
      when 1; op = '.byte'
      when 2; op = '.word'
      else
        #:nocov:
        raise
        #:nocov:
      end
      val.each_slice(16) do |slice|
        slice.map! do |e|
          if Numeric === e.val or Symbol === e.val then e.val else to_asm(e) end
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
          r << "sta <reg+0+#{i}"
          r << load_a(op[3],i)
          r << "sta <reg+2+#{i}"
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
          r << "lda <reg+4+#{i}"
          r << store_a(op[1],i)
        end
      end
      r
    end

    ############################################
    # レジスター割り当て
    ############################################
    def alloc_register( lmd )

      if @opt[:optimize_level] > 0
        Fc.allocate_register( lmd )
        delete_unuse( lmd )
      else
        # 単純なバージョンのアロケータ(debug用)
        #:nocov:
        size = 0
        lmd.vars.each do |var|
          if var.kind == :local
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

    # index->pget, index->psetの組み合わせを合成できるなら合成する
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
                op[2].kind == :global and # 単純なシンボルで
                op[1].opt[:local_type] == :temp and # その変数をそこでしか使っていない
                op[3].type.size == 1 # インデックスのサイズが1byte
              # puts "replace #{op}, #{next_op}"
              ops[i] = [:index_pget, next_op[1], op[2], op[3]]
              ops[i+1] = nil
            end
          when :pset
            if op[1] == next_op[1] and # 同じ変数を連続で使っていて
                op[2].kind == :global and # 単純なシンボルで
                op[1].opt[:local_type] == :temp and # その変数をそこでしか使っていない
                op[3].type.size == 1 # インデックスのサイズが1byte
              # puts "replace #{op}, #{next_op}"
              ops[i] = [:index_pset, op[2], op[3], next_op[2]]
              ops[i+1] = nil
            end
          end
        end
      end
      ops
    end

    # ブランチ命令のジャンプ先が+-127より遠いかもしれない場合は、２段階ジャンプを可能にする
    # asm: アセンブリ(文字列の配列)
    def extend_jump( asm )
      op_size = {"call"=>10}
      branch_ops = { 'bcc'=>'bcs', 'bcs'=>'bcc', 'beq'=>'bne', 'bne'=>'beq', 'bmi'=>'bpl', 'bpl'=>'bmi'}
      ['brk','clc','cld','clv','dex','dey','inx','iny','nop','pha','php','pla','plp','rti','rts',
      'sec','sec','sed','sei','tax','tay','tsx','txa','txs','tya'].each {|op| op_size[op] = 1 }
      ['adc','and','asl','cmp','cpx','cpy','dec','eor','inc','jmp','jsr',
       'lda','ldx','ldy','lsr','ora','rol','ror','sbc','sta','stx','sty'].each {|op| op_size[op] = 3 }
      ['bcc','bcs','beq','bit','bmi','bne','bpl','bvc','bvs'].each {|op| op_size[op] = 5 } #分割して増えるかもしれない

      # 各ラベルのアドレス候補を求める
      addrs = []
      labels = Hash.new
      n = 0
      asm.each do |line|
        addrs << n
        if line =~/\A([@._\w][_\w\d]+):/
          labels[$1] = n
        elsif line =~ /\A\s+(\w+)/
          if size = op_size[$1]
            n += size
          else
            n += 10 # 知らない命令は、とりえあず10byteとする
          end
        end
      end

      # 書き換えが必要なジャンプを書き換える
      asm.each_with_index do |line,i|
        addr = addrs[i]
        if line =~ /\A\s+(\w+)\s+([@._\w][_\w\d]+)/
          if branch_ops[$1]
            jump_to = labels[$2]
            if (jump_to - addr).abs >= 127
              label = new_label
              asm[i] = ["\t#{branch_ops[$1]} #{label}", "\tjmp #{$2}", "#{label}:"]
            end
          end
        end
      end

      asm.flatten
    end

  end
end
