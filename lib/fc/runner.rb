# coding: utf-8
# インタープリタ

class Runner
  attr_reader :labels, :block, :pc

  def initialize( compiler )
    @compiler = compiler
    @stack = []

    @vars = Hash.new
    @compiler.module.lambdas.each do |lmd|
      lmd.vars.each do |id,v|
        @vars[v] = new_var( v.type )
      end
    end

    @labels = Hash.new
    @compiler.module.lambdas.each do |lmd|
      lmd.ops.each_with_index do |op,i|
        @labels[lmd.id.to_s+op[1]] = [lmd.id,i] if op[0] == :label
      end
    end

    main = @compiler.module.lambdas.find { |lmd| lmd.id == :main }

    @block = main
    @pc = 0
    @ret = nil
  end

  def new_var( type )
    case type.kind
    when :int
      0
    when :array
      Array.new(type.length){ new_var( type.base ) }
    when :pointer
      nil
    else
      raise
    end
  end

  def run
    while true
      run_one
      break unless @block and @block.ops[@pc]
    end
    show
  end

  def run_one
    op = @block.ops[@pc]
    @pc += 1
    case op[0]
    when :'if'
      if get(op[1]) == 0
        @pc = @labels[@block.id.to_s+op[2]][1]
      end
    when :load
      put op[1], get(op[2])
    when :add, :sub, :mul, :div, :eq, :lt
      hash = { add: :+, sub: :-, mul: :*, div: :/, eq: :==, lt: :< }
      v = get(op[2]).__send__(hash[op[0]], get(op[3]) )
      v = 1 if v === true
      v = 0 if v === false
      put op[1], v
    when :index
      ptr = get(op[2])
      put op[1], [ptr, get(op[3])]
    when :pget
      ptr = get(op[2])
      put op[1], ptr[0][ ptr[1] ]
    when :pset
      ptr = get(op[1])
      ptr[0][ ptr[1] ] = get(op[2])
    when :return
      @ret = get(op[1])
      @block, @pc = @stack.pop
    when :call
      unless @ret
        if op[2].id == :print
          puts "OUTPUT: #{get(op[3])}"
        else
          @stack << [@block,@pc-1]
          @block = op[2].block
          @pc = 0
        end
      else
        put op[1], @ret
        @ret = nil
      end
    when :jump
      @pc = @labels[@block.id.to_s+op[1]][1]
    when :label
      # DO NOTHING
    else
      show
      raise "unknow op #{op}"
    end
  end

  def get( val )
    if val.id
      @vars[val.id]
    else
      val.val
    end
  end

  def put( var, v )
    @vars[var.id] = v
  end

  def show
    puts "STACK:"
    if @block 
      puts "  #{@block.id}:#{@pc}: #{@block.ops[@pc] || 'finished' }"
    end
    @stack.each do |s|
      puts "  #{s[0].id}:#{s[1]}: #{s[0].ops[s[1]]}"
    end
    puts "VARS:"
    @vars.each do |v|
      puts "  #{v[0]} = #{v[1]}"
    end
  end
end


