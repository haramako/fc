# backports for compatibility.

unless defined? IO.write
  def IO.write(name, string)
    File.open(name,'w'){|f| f.write string }
  end
end

unless defined? IO.binread
  def IO.binread(name)
    File.open(name,'rb:ASCII-8BIT'){|f| f.read }
  end
end

unless defined? IO.binwrite
  def IO.binwrite(name, string)
    File.open(name,'wb:ASCII-8BIT'){|f| f.write string }
  end
end
