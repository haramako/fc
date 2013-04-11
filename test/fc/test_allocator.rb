require 'rspec'
require_relative '../../lib/fc/allocator'

include Fc

describe LiveRangeCalculator, 'should calc live range' do

  before do
    @lrc = LiveRangeCalculator.new([[],[],[],[],[1],[]])
  end

  it 'should calc live range' do
    @lrc.calc_live_range( [[0,3],[1,4]] ).should eq (0..4) # a
    @lrc.calc_live_range( [[1  ],[3  ]] ).should eq (1..3) # b
    @lrc.calc_live_range( [[3  ],[3,5]] ).should eq (0..5) # c
  end

end

describe Allocator do

  def liverange( flow, vars )
    lrc = LiveRangeCalculator.new( flow )
    r = Hash.new
    vars.each do |v,info|
      lr = lrc.calc_live_range( info )
      r[v] = { live_range: lr } if lr
    end
    r
  end

  it 'should allocate 3 registers' do
    @alloc = Allocator.new( liverange( [[],[],[],[],[1],[]],
                            { a: [[0,3],[1,4]],
                              b: [[1  ],[3  ]],
                              c: [[3  ],[3,5]] } ) )
    @alloc.regs.size.should be 3
  end

  it 'should allocate 3 registers' do
    @alloc = Allocator.new( liverange( [[],[],[],[],[1],[]],
                            { a: [[0,3],[1,4]],
                              b: [[1  ],[3  ]],
                              c: [[3  ],[3,5]],
                              d: [[0  ],[0  ]],
                            } ) )
    @alloc.regs.size.should be 3
  end

end
