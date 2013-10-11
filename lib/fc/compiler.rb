# -*- coding: utf-8 -*-
require 'rbconfig'
require_relative 'hlc'
require_relative 'llc'
require_relative 'nes'

module Fc
  class Compiler

    if /mswin(?!ce)|mingw|cygwin|bccwin/ === RbConfig::CONFIG['target_os']
      NESASM = FC_HOME + 'bin/nesasm.exe'
    else
      NESASM = 'nesasm'
    end

    def initialize
    end

    def compile( src_filename, opt = Hash.new )
      opt = {out:'a.nes', target:'emu'}.merge(opt)

      base = File.basename(opt[:out],'.*')
      Fc::LIB_PATH << Pathname(Fc::FC_HOME + 'fclib' + opt[:target])

      # ソースコードを中間コードにコンパイル
      hlc = Fc::Hlc.new
      hlc.compile( src_filename )
      mod = hlc.modules
      prog_bank_count = hlc.options[:bank_count] || 2

      if opt[:debug_info]
        open( base+'.html', 'w' ) do |f|
          f.write Fc::HtmlOutput.new.module_to_html( hlc.modules )
        end
      end

      # pp hlc
      # exit


      # 中間コードをアセンブラにコンパイルする
      llc = Fc::Llc.new( opt )
      llc.prog_bank_count = prog_bank_count
      llc.compile( mod )

      if opt[:debug_info]
        open( base+'.html', 'w' ) do |f|
          f.write Fc::HtmlOutput.new.module_to_html( hlc.modules )
        end
      end

      # バンクの情報の補完
      prog_bank_count.times do |i|
        default = if i < prog_bank_count - 2 then 0x8000+(i%2)*0x2000 else 0xc000+(i+2-prog_bank_count)*0x2000 end
        llc.prog_banks[i][0] = default unless llc.prog_banks[i][0]
      end
      # 最後のバンクにランタイムのコードを追加
      llc.prog_banks[prog_bank_count-1][1] << File.read( Fc.find_share('runtime.asm') )
      llc.prog_banks[prog_bank_count-1][1] << File.read( Fc.find_module('runtime_init.asm') )

      case hlc.options[:mapper]
      when nil, "MMC0"
        inesmap = 0
      when "MMC3"
        inesmap = 4
      when Numeric
        inesmap = hlc.options[:mapper]
      end

      # オプションのアセンブラa
      options = { inesprg: (prog_bank_count+1)/2, ineschr: llc.char_banks.size, inesmir: 1, inesmap: inesmap }
      opt_asm = options.map{|k,v| "\t.#{k} #{v}" }

      # 各キャラクターバンクのコードを作成
      chr_asm = []
      llc.char_banks.each_with_index do |bank,i|
        chr_asm << "\t.bank #{prog_bank_count+i}"
        chr_asm << "\t.org $0000"
        chr_asm << bank
      end

      # 各プログラムバンクのコードを作成
      code_asm = ["\t.org $300"] + llc.asm
      llc.prog_banks.each_with_index do |bank,i|
        code_asm << "\t.bank #{i}"
        code_asm << "\t.org $%04X"%bank[0]
        code_asm.concat bank[1]
      end

      opt_asm = opt_asm.join("\n")
      chr_asm = chr_asm.join("\n")
      code_asm = code_asm.join("\n")

      # アセンブラを生成する
      template = File.read( Fc.find_share('base.asm') ) 
      str = ERB.new(template,nil,'-').result(binding)

      open( base+'.asm', 'w' ) do |f|
        f.write str
      end

      return if opt[:asm]

      # nesasmでアセンブルする
      result = `#{NESASM} -s -autozp -m -l3 #{base+'.asm'}`
      if /error/ === result
        raise CompileError.new( "*** can't assemble #{base.to_s+'.asm'} ***\n" + result )
      end
      puts result if opt[:debug_info]

      # size = parse_nes( base+'.nes' )[:prog_size]
      # puts "code=#{size}, var=#{llc.address-0x200}, zeropage=#{llc.address_zeropage-0x10}"

      if opt[:run]
        emu = Fc::FC_HOME + 'bin/emu6502'
        result = `#{emu} #{base}.nes`
        print result
      end
    end
  end
end
