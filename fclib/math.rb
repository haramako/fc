sin = @scope.find(:sin)
defmacro(:cos) do |args|
  [:call, sin, [[:add, args[0], 64]], nil]
end

