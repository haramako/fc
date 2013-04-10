# -*- coding: utf-8 -*-

module Fc

  ######################################################################
  # パーサー
  ######################################################################
  class Parser

    def initialize( src, filename='(unknown)' )
      @filename = filename
      @scanner = StringScanner.new(src)
      @line_no = 1
      @pos_info = Hash.new
    end

    def next_token
      # コメントと空白を飛ばす
      while @scanner.scan(/ \s+ | \/\/.+?\n | \/\*.+?\*\/ /mx)
        @scanner[0].gsub(/\n/){ @line_no += 1 }
      end
      if @scanner.eos?
        r = nil
      elsif @scanner.scan(/<=|>=|==|\+=|-=|!=|->|&&|\|\||\(|\)|\{|\}|;|:|<|>|\[|\]|\+|-|\*|\/|%|&|\||\^|=|,|\.|!/)
        # 記号
        r = [@scanner[0], @scanner[0]]
      elsif @scanner.scan(/-?0[xX]([\d\w]+)/)
        # 16進数
        r = [:NUMBER, @scanner[1].to_i(16)]
      elsif @scanner.scan(/-?0[bB](\d+)/)
        # 2進数
        r = [:NUMBER, @scanner[1].to_i(2)]
      elsif @scanner.scan(/-?\d+/)
        # 10進数
        r = [:NUMBER, @scanner[0].to_i]
      elsif @scanner.scan(/\w+/)
        # 識別子/キーワード
        if /^(include|function|const|var|options|if|else|elsif|loop|while|for|return|break|continue|incbin|switch|case|default|use|as)$/ === @scanner[0]
          r = [@scanner[0], @scanner[0]]
        else
          r = [:IDENT, @scanner[0].to_sym ]
        end
      elsif @scanner.scan(/"([^\\"]|\\.)*"/)
        # ""文字列
        str = @scanner[0][1..-2]
        str = str.gsub(/\\n|\\x../) do |s|
          case s
          when '\n'
            "\n"
          when /\\x/
            s[2..-1].to_i(16).chr
          end
        end
        r = [:STRING, str]
      elsif @scanner.scan(/'([^\\']|\\.)*'/)
        # ''文字列
        r = [:STRING, @scanner[0][1..-2]]
      else
        # :nocov:
        raise "invalid token at #{@line_no}"
        # :nocov:
      end
      r
    end

    def info( ast )
      @pos_info[ast] = [@filename,@line_no]
    end

    def parse
      ast = do_parse
      [ast, @pos_info]
    rescue Racc::ParseError
      err = CompileError.new( "#{$!.to_s.strip}" )
      err.filename = @filename
      err.line_no = @line_no
      raise err
    end

    def on_error( token_id, err_val, stack )
      # puts "#{@filename}:#{@line_no}: error with #{token_to_str(token_id)}:#{err_val}"
      # pp token_id, err_val, stack
      super
    end

  end

end
