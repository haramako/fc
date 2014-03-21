# compatibility

unless defined? IO.write
  def IO.write(name, string)
    File.open(name,'w'){|f| f.write string}
  end
end
