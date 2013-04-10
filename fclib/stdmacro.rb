defmacro :times do |args,block|
  limit = args[0]
  v = :__loops2__
  [
   [:var, [[v, :int, limit, {} ]]],
   [:loop, 
    block +
    [
     [:exp, [:load, v, [:sub, v, 1]]],
     [:if, [:not, v], [[:break]]]
    ]
   ]
  ]
end
