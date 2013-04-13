# -*- coding: utf-8 -*-

module Fc

  ############################################
  # 型関係のユーティリティ
  ############################################
  module TypeUtil

    module_function

    def guess_type( type, val )
      if type
        compatible_type( type, val.type )
      else
        val.type
      end
    end

    def compatible_type?( a, b )
      return a if  a == b
      if a.kind == :int and b.kind == :int
        if a.size == b.size
          return (a.signed)?a:b
        else
          return (a.size>b.size)?a:b
        end
      elsif a.kind == :pointer and b.kind == :array and a.base == b.base
        return a
      elsif a.kind == :array and b.kind == :array and a.base == b.base and a.length == nil
        # 配列の長さを省略した場合
        return b
      elsif a.kind == :array and b.kind == :array and a.base == b.base and a.length != b.length
        return Type[[:pointer, a.base]]
      else
        return nil
      end
    end

    def compatible_type( a, b )
      r = compatible_type?( a, b )
      raise CompileError.new("not compatible type '#{a}' and '#{b}'") unless r
      r
    end

  end
end
