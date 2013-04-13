defmacro :unittest_run_tests do |args|
  r = [[:exp, [:call, :init, []]]]
  @scope.id_list.each do |id|
    if id.to_s.match /^test_/
      r << [:exp, [:call, :print, ["#{id}:"]]]
      r << [:exp, [:call, id, []]]
      r << [:exp, [:call, :print, ["\n"]]]
    end
  end
  r << [:exp, [:call, :exit, [0]]]
  r
end
