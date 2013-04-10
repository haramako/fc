
atan = []
sin = []
include Math

16.times do |y|
  r = []
  16.times do |x|
    r << [ Math.atan2(y,x)/Math::PI*128, 63].min.to_i
  end
  atan << r
end

64.times do |i|
  sin << [ Math.sin(Math::PI/2*i/64) *128, 127].min.to_i
end

srand(-999)
rand_table = []
256.times do |i|
  rand_table << rand(256)
end

puts "const atan_table = [#{atan.join(',')}];"
puts "const sin_table = [#{sin.join(',')}];"
puts "const rand_table = [#{rand_table.join(',')}];"

mul_l0 = []
mul_l1 = []
mul_h0 = []
mul_h1 = []
256.times do |i|
  i2 = i+256
  mul_l0 << (i*i/4)%256
  mul_l1 << (i2*i2/4)%256
  mul_h0 << (i*i/4)/256
  mul_h1 << (i2*i2/4)/256
end

puts "__mul_tbl_l0:"
mul_l0.each_slice(16) do |x|
  puts "\t.db #{x.join(",")}"
end

puts "__mul_tbl_l1:"
mul_l1.each_slice(16) do |x|
  puts "\t.db #{x.join(",")}"
end

puts "__mul_tbl_h0:"
mul_h0.each_slice(16) do |x|
  puts "\t.db #{x.join(",")}"
end

puts "__mul_tbl_h1:"
mul_h1.each_slice(16) do |x|
  puts "\t.db #{x.join(",")}"
end
