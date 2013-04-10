# coding: utf-8

module Fc

  def self.allocate_register( lmd )
    # ラベルの収集
    labels = Hash.new
    lmd.ops.each_with_index do |op,i|
      labels[op[1]] = i if op[0] == :label
    end

    # 変数の定義・仕様、制御フローグラフの集計
    vars = Hash.new {|h,k| h[k] = [[],[]] }
    flow = []
    lmd.ops.each_with_index do |op,i|
      uses = []
      defines = []
      node = []
      case op[0]
      when :label, :asm
      when :'if'
        uses << op[1]
        node << labels[op[2]]
      when :jump
        node << labels[op[1]]
      when :return
        uses << op[1]
      when :call
        defines << op[1]
        uses.concat op[2..-1]
      when :load, :uminus, :not, :sign_extension, :ref, :deref
        defines << op[1]
        uses << op[2]
      when :add, :sub, :and, :or, :xor, :mul, :div, :mod, :eq, :lt, :index, :pget
        defines << op[1]
        uses << op[2] << op[3]
      when :pset
        uses << op[1] << op[2]
      else
        #:nocov:
        raise "invalid op #{op}"
        #:nocov:
      end

      flow << node

      defines.each do |v|
        vars[v][0] << i if v and v.on_stack?
      end

      uses.each do |v|
        next if Lambda === v
        vars[v][1] << i if v and v.on_stack?
      end
    end

    # 引数は、最初に定義されているものとする
    vars.each do |v,info|
      if v.kind == :arg
        info[0] << 0
      end
    end
    # puts '*'*20+lmd.to_s+'*'*20
    # pp flow, vars

    # 帰り値、引数のアドレスは先に割り当てる
    frame_size = lmd.type.base.size
    # pp lmd.vars
    lmd.result.address = 0 if lmd.result
    lmd.args.each do |arg|
      arg.address = frame_size
      frame_size += arg.type.size
    end

    # サイズ１と２のやつだけ割り当てる
    1.upto(2) do |size|
      fit_vars = Hash.new
      vars.each do |v,info|
        if v.type.size == size
          fit_vars[v.id] = info
        end
      end
      allocator = Allocator.new flow, fit_vars
      # pp [size, flow, fit_vars ]
      # pp [size, allocator.regs]
      # 各ジスタのアドレスを割り当てる
      allocator.regs.each do |reg|
        address = nil
        # 引数が含まれる場合は,すでに割り当てられている
        reg[:vars].each do |id|
          arg = lmd.args.find{|arg| arg.id == id }
          address = arg.address if arg
        end
        # アドレスを算出する
        unless address
          address = frame_size
          frame_size += size
        end
        # 割り当てる
        reg[:vars].each do |id|
          lmd.vars.find{ |v| v.id == id }.address = address
        end
      end
    end

    # サイズ３以上は全部割り当てる
    vars.each do |v,info|
      if v.type.size > 2
        # TODO: 未実装
        #:nocov:
        raise
        #:nocov:
      end
    end

    # 未使用フラグをたてる
    vars.each do |v,info|
      v.unuse = true unless v.address
    end

    lmd.frame_size = frame_size
  end

  ######################################################################
  # レジスタアロケータ
  ######################################################################
  class Allocator

    attr_reader :vars, :regs

    def initialize( flow, vars )
      # live range を求める
      lrc = LiveRangeCalculator.new( flow )
      @vars = Hash.new
      vars.each do |id,var|
        lr = lrc.calc_live_range( vars[id] )
        @vars[id] = { live_range: lrc.calc_live_range( vars[id] ) } if lr
      end

      # レジスタの割り当て
      @regs = []
      @vars.each do |id,var|
        found = false
        @regs.each do |reg|
          unless overlap?( reg[:live_range], var[:live_range] )
            reg[:live_range] = join( reg[:live_range], var[:live_range] )
            reg[:vars] << id
            found = true
            break
          end
        end
        unless found
          @regs << { live_range: var[:live_range], vars: [id] }
        end
      end
      # pp @regs, @vars
    end

    private

    def overlap?( r1, r2 )
      r1.max >= r2.min and r1.min <= r2.max
    end

    def join( r1, r2 )
      ( [r1.min,r2.min].min .. [r1.max,r2.max].max )
    end


  end

  ######################################################################
  # Live Range の計算を行うクラス
  ######################################################################
  class LiveRangeCalculator

    # flow: 制御フローグラフ( [節0情報,節1情報, ... ] の形で、節情報 = [ [後続節番号,...], [先行節番号,...] ] )
    def initialize( flow )
      @flow = flow.map.with_index do |node,i|
        node << i+1 if i < flow.size-1 # 直後の節を後続節とする
        [node,[]] 
      end

      @flow.each_with_index do |node,i|
        node[0].each do |succ|
          @flow[succ][1] << i
        end
      end

    end

    def calc_live_range( var )
      lives = @flow.map { |i| live = Hash.new }
      var[0].each { |i| lives[i][:define] = true }
      var[1].each { |i| lives[i][:use] = true; lives[i][:live_in] = true }
      while true
        finished = true
        (lives.size-1).downto(0) do |i| 
          live = lives[i]
          # 入り口生存なら、先行節で出口生存
          if live[:live_in]
            @flow[i][1].each do |pred|
              unless lives[pred][:live_out]
                lives[pred][:live_out] = true
                finished = false
              end
            end
          end
          # 出口生存で定義節でないなら、入り口生存
          if live[:live_out] and !live[:define]
            unless live[:live_in]
              live[:live_in] = true
              finished = false
            end
          end
        end
        break if finished
      end
      # show_lives lives
      # live range の算出
      max = 0
      min = 1000000
      lives.each_with_index do |live,i|
        if live[:live_in] or live[:live_out]
          max = i if i > max
          min = i if i < min
        end
      end
      if min == 1000000
        nil
      else
        (min..max)
      end
    end
  end

  #:nocov:
  def show_lives( lives )
    t = { true =>'x', false=>' ', nil=>' ' }
    puts '   n U D  i o'
    lives.each_with_index do |live,i|
        puts( '%04d %s %s  %s %s' % [i, t[live[:use]], t[live[:define]], t[live[:live_in]], t[live[:live_out]]] )
    end
  end
  #:nocov:

end
