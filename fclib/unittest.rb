
stdio = @scope.find(:stdio).val
print = stdio.scope.find(:print)
exit = stdio.scope.find(:exit)
init = stdio.scope.find(:init)

defmacro :unittest_run_tests do |args|
  r = [:block, [:exp, [:call, init, []]]]
  @scope.id_list.each do |id|
    if id.to_s.match /^test_/
      r << [:exp, [:call, print, ["#{id}:"]]]
      r << [:exp, [:call, id, []]]
      r << [:exp, [:call, print, ["\n"]]]
    end
  end
  r << [:exp, [:call, exit, [0]]]
  r
end
