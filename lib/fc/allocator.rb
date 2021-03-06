# coding: utf-8

module Fc

  def self.calc_live_range( lmd )
    # ラベルの収集
    labels = Hash.new # labels[ラベル] = op番号
    lmd.ops.each_with_index do |op,i|
      labels[op[1]] = i if op[0] == :label
    end

    # 変数の定義・仕様、制御フローグラフの集計
    use_define = Hash.new {|h,k| h[k] = [[],[]] } # 変数ごとの use/define を記録する
    flow = [] # flow[ジャンプ元op番号] = [ジャンプ先のop番号, ... ]
    lmd.ops.each_with_index do |op,i|
      uses = []
      defines = []
      node = []
      case op[0]
      when :label, :asm, :push_result, :push_fastcall_result
        # DO NOTHING
      when :'if'
        uses << op[1]
        node << labels[op[2]]
      when :jump
        node << labels[op[1]]
      when :return
        uses << op[1]
      when :push_arg, :push_fastcall_arg
        uses << op[2]
      when :load, :uminus, :not, :sign_extension, :ref, :call, :fastcall
        defines << op[1]
        uses << op[2]
      when :add, :sub, :and, :or, :xor, :mul, :div, :mod, :eq, :lt, :shift_left, :shift_right, :index, :pget
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
        use_define[v][0] << i if v and v.kind == :local
      end

      uses.each do |v|
        use_define[v][1] << i if v and v.kind == :local
      end
    end

    # 引数は、最初に定義されているものとする
    use_define.each do |v,info|
      info[0] << 0 if v.opt[:local_type] == :arg
    end

    # live range を求める
    lrc = LiveRangeCalculator.new( flow )
    use_define.each do |v,info|
      v.live_range = lrc.calc_live_range( info )
    end
    
  end

  # 
  # 変数の address, location, unuse が設定される
  def self.allocate_register( lmd )
    calc_live_range( lmd )

    # ref(&演算子)を受けた変数を集める
    refered = Hash.new # 変数がrefを受けたかどうか(refされるならメモリは独立で確保しなくてはいけないため)
    lmd.ops.each do |op|
      refered[op[2]] = true if op[0] == :ref
    end

    # live range を求め、{callをまたぐ|refを受ける|引数}だったら、フレームスタック上に確保する
    frame_size = lmd.type.base.size # 帰り値分を予約しておく
    register_vars = Hash.new # レジスタに割り当てる変数, register_vars[変数:Value] = liverange:Range
    lmd.vars.each do |v|
      beyond_call = false
      if v.live_range
        ((v.live_range.min+1)..(v.live_range.max-1)).each do |i|
          beyond_call = true if lmd.ops[i] and lmd.ops[i][0] == :call and not lmd.ops[i][2].type.fastcall?
        end
      end
      
      if v.opt[:local_type] == :result
        # 返り値
        lmd.result.location = lmd.type.fastcall? ? :fastcall_reg : :frame
        lmd.result.address = 0 
      elsif beyond_call or v.opt[:local_type] == :arg or refered[v]
        # 引数か、関数をまたいでいるなら、フレームに割り当てる
        v.address = frame_size
        v.location = lmd.type.fastcall? ? :fastcall_reg : :frame
        frame_size += v.type.size
      elsif v.live_range
        # それ以外の使われてる変数は、レジスターメモリに割り当てる
        register_vars[v] = { live_range: v.live_range }
      else
        # 未使用フラグをたてる
        v.location = :none
        v.unuse = true
      end
    end

    allocate_cond( lmd, register_vars )
    allocate_a( lmd, register_vars )

    # 各ジスタのアドレスを割り当てる
    reg_size = 0
    allocator = Allocator.new register_vars
    allocator.regs.each do |reg|
      # アドレスを算出する
      raise CompileError.new("frame size over on #{lmd}") if reg_size > 16
      # 割り当てる
      reg[:vars].each do |v|
        if lmd.type.fastcall?
          raise CompileError.new("frame size over on #{lmd}") if frame_size + reg_size > 16
          v.location = :fastcall_reg
          v.address = frame_size + reg_size
        else
          v.location = :reg
          v.address = reg_size
        end
      end
      reg_size += 2
    end

    lmd.frame_size = frame_size
  end

  # Aジスタを割り当てられるなら割り当てる
  def self.allocate_a( lmd, register_vars )
    a_vars = [] # Aレジスタを割り当てる変数
    register_vars.each do |v,info|
      next unless v.type.size == 1
      if v.live_range.max-v.live_range.min == 1
        # Aレジスタを割り当てられる組み合わせでなければスルー
        op = lmd.ops[v.live_range.min]
        next unless op[1] == v
        next unless [:load, :add, :sub, :and, :or, :xor, 
                     :mul, :div, :mod, :uminus, :eq, :lt, :pget].include?( op[0] )

        next_op = lmd.ops[v.live_range.min+1]
        case next_op[0]
        when :load, :sign_extension, :add, :and, :or, :xor, :eq, :lt, :pget, :sub, :push_arg
          next unless next_op[2] == v
        when :if, :return
          next unless next_op[1] == v
        else next
        end

        a_vars << v
      end
    end
    a_vars.each do |v|
      v.location = :a
      register_vars.delete(v)
    end
  end

  # コンディションレジスタを割り当てられるなら割り当てる
  def self.allocate_cond( lmd, register_vars )
    cond_vars = [] # コンディションレジスタを割り当てる変数
    register_vars.each do |v,info|
      next unless v.type.size == 1
      if v.live_range.max-v.live_range.min == 1
        # Aレジスタを割り当てられる組み合わせでなければスルー
        op = lmd.ops[v.live_range.min]
        next unless op[1] == v
        next unless [:eq, :lt, :not].include?( op[0] )

        next_op = lmd.ops[v.live_range.min+1]
        case next_op[0]
        when :'if'
          next unless next_op[1] == v
        when :'not'
          next unless next_op[2] == v
        else next
        end

        case op[0]
        when :eq 
          v.location = :cond
          v.cond_positive = true
          v.cond_reg = :zero
        when :lt
          v.location = :cond
          v.cond_positive = true
          if op[2].type.signed or op[3].type.signed
            next if op[2].type.size > 1 or op[3].type.size > 1 # サイズ2以上の符号付き比較はフラグが特定できない
            v.cond_reg = :negative
          else
            v.cond_reg = :carry
          end
        when :'not'
          next unless op[2].location == :cond
          v.location = :cond
          v.cond_reg = op[2].cond_reg
          v.cond_positive = !op[2].cond_positive
        else raise
        end
        cond_vars << v
      end
    end
    cond_vars.each do |v|
      #v.location = :a
      register_vars.delete(v)
    end
  end

  ######################################################################
  # レジスタアロケータ
  ######################################################################
  class Allocator

    attr_reader :regs

    def initialize( vars )
      # レジスタの割り当て
      @regs = []
      vars.each do |id,var|
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

    # live range を計算する
    # 全く使われていない場合、nilを返す
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
