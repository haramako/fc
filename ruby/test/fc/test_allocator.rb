require 'rspec'
require_relative '../../lib/fc/allocator'

include Fc

describe LiveRangeCalculator, 'should calc live range' do

  before do
    @lrc = LiveRangeCalculator.new([[],[],[],[],[1],[]])
  end

  it 'should calc live range' do
    expect(@lrc.calc_live_range( [[0,3],[1,4]] )).to eq (0..4) # a
    expect(@lrc.calc_live_range( [[1  ],[3  ]] )).to eq (1..3) # b
    expect(@lrc.calc_live_range( [[3  ],[3,5]] )).to eq (0..5) # c
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
    expect(@alloc.regs.size).to be 3
  end

  it 'should allocate 3 registers' do
    @alloc = Allocator.new( liverange( [[],[],[],[],[1],[]],
                            { a: [[0,3],[1,4]],
                              b: [[1  ],[3  ]],
                              c: [[3  ],[3,5]],
                              d: [[0  ],[0  ]],
                            } ) )
    expect(@alloc.regs.size).to be 3
  end

end
