uint8_p = Type[[:pointer, :uint8]]

print = @scope.find(:print)
print_int16 = @scope.find(:print_int16)

defmacro( :printf ) do |args|
  r = [:block]
  args.each do |arg|
    if TypeUtil.compatible_type?( uint8_p, arg.type )
      r << [:exp, [:call, print, [arg]]]
    elsif arg.type.kind == :int
      r << [:exp, [:call, print_int16, [arg]]]
    end
  end
  r
end
