uint8_p = Type[[:pointer, :uint8]]

$LOAD_PATH << '../nes_tools/lib'
require 'nes_tools'

conv = NesTools::TextConverter.new( nil, File.read('../tmp/font/text.chr.txt', encoding: 'utf-8') )
misc_conv = NesTools::TextConverter.new( nil, File.read('../tmp/font/misc_text.chr.txt', encoding: 'utf-8') )

defmacro( :_T ) do |args|
  text = conv.conv( args[0].base_string ) + [0]
  #puts args[0].base_string
  [:array, text]
end

defmacro( :_M ) do |args|
  text = misc_conv.conv( args[0].base_string ) + [0]
  #puts args[0].base_string
  [:array, text]
end

defmacro( :VERSION_STR ) do |args|
  [:array, misc_conv.conv( 'VERSION ' + IO.read('../VERSION').chomp ) + [0]]
end
