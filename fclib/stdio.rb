uint8_p = Type[[:pointer, :uint8]]

defmacro( :printf ) do |args|
  r = []
  args.each do |arg|
    if TypeUtil.compatible_type?( uint8_p, arg.type )
      r << [:exp, [:call, :print, [arg]]]
    elsif arg.type.kind == :int
      r << [:exp, [:call, :print_int16, [arg]]]
    end
  end
  r
end
