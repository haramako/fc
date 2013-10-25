# -*- coding: utf-8 -*-
require 'rbconfig'
require 'fileutils'
require 'open3'
require_relative 'hlc'
require_relative 'llc'
require_relative 'nes'

module Fc

  class CommandError < RuntimeError
    attr_reader :command, :result
    def initialize( message, command, result )
      super message
      @command = command
      @result = result
    end
  end

  class Compiler

    if /mswin(?!ce)|mingw|cygwin|bccwin/ === RbConfig::CONFIG['target_os']
      NESASM = FC_HOME + 'bin/nesasm.exe'
      CA65 = FC_HOME + 'bin/ca65'
      LD65 = FC_HOME + 'bin/ld65'
    else
      NESASM = 'nesasm'
      CA65 = 'ca65'
      LD65 = 'ld65'
    end

    def initialize
      @debug = Fc::DEBUG_LEVEL
    end

    def dout( level, *args )
      #:nocov:
      if @debug >= level
        puts args.join(" ")
      end
      #:nocov:
    end

    def build( filename, opt = Hash.new )
      opt = {out:'a.nes', target:'emu'}.merge(opt)
      opt[:out] = Pathname.new(opt[:out])
      opt[:html] = opt[:out].sub_ext('.html')

      FileUtils.mkdir_p( BUILD_PATH )
      hlc = compile( filename, opt )
      compile2( hlc, opt )
      objs = assemble( hlc, opt )

      link hlc, objs, opt

      # 実行する
      if opt[:run]
        emu = Fc::FC_HOME + 'bin/emu6502'
        result = `#{emu} #{opt[:out]}`
        print result
      end
    rescue CommandError => err
      raise Fc::CompileError.new( err.message + "\n" + err.command.to_s + "\n" + err.result )
    end

    # ソースコードを中間コードにコンパイル
    def compile( filename, opt = Hash.new )

      base = File.basename(opt[:out],'.*')
      Fc::LIB_PATH << Pathname(Fc::FC_HOME + 'fclib' + opt[:target])

      hlc = Fc::Hlc.new
      hlc.compile( filename )

      hlc
    end

    # 中間コードをアセンブラファイルにコンパイルする
    def compile2( hlc, opt )
      llc = Fc::Llc.new( opt )

      hlc.modules.each do |path,mod|
        next if mod.from_fcm
        asm, inc = llc.compile( mod )
        IO.write( BUILD_PATH+("_#{mod.id}.inc"), inc.join("\n") )
        IO.write( BUILD_PATH+("_#{mod.id}.s"), asm.join("\n") )
      end
    end

    # アセンブラファイルをオブジェクトファイルにコンパイルする
    def assemble( hlc, opt )
      objs = []
      hlc.modules.each do |path,mod|
        #.oファイルの作成
        objs << BUILD_PATH+('_'+mod.id.to_s+'.o')
        next if mod.from_fcm
        ca65 BUILD_PATH+('_'+mod.id.to_s+'.s')
      end

      objs
    end

    # リンクする
    def link( hlc, objs, opt )

      case hlc.options[:mapper]
      when nil, "MMC0"
        inesmap = 0
      when "MMC3"
        inesmap = 4
      when Numeric
        inesmap = hlc.options[:mapper]
      end

      # オプションのアセンブラ
      inesprg = (hlc.options[:bank_count] || 4 ) / 2
      ineschr = (hlc.options[:char_banks] || 1 )
      options = { inesprg: inesprg, ineschr: ineschr, inesmir: 1, inesmap: inesmap }
      template = IO.read( Fc.find_share('base.asm.erb') ) 
      str = ERB.new(template,nil,'-').result(binding)
      IO.write( BUILD_PATH+'base.s', str )
      ca65 BUILD_PATH+'base.s'
      ca65 FC_HOME+'share/runtime.asm'
      ca65 FC_HOME+'fclib'+opt[:target]+'runtime_init.asm'

      if hlc.options[:bank_count]
        bank_num = hlc.options[:bank_count]
        banks = (0...bank_num).map do |i|
          size = 0x2000
          size -= 6 if i == bank_num-1 # vectors size
          org = 0x8000 + (i%4) * 0x2000
          {size:size, org:org}
        end
      else
        banks = [{size:0x8000-6, org:0x8000}]
      end
      segs = hlc.modules.map do |name,m| 
        bank = m.options[:bank] || 0
        bank = banks.size + bank if bank < 0
        if m.options[:org]
          banks[bank][:org] = m.options[:org]
        end
        {name: m.id.to_s, bank: bank}
      end
      cfg = ERB.new(IO.read( FC_HOME+'share/ld65.cfg' ),nil,'-').result(binding)
      IO.write( BUILD_PATH+'ld65.cfg', cfg )

      sh( LD65, '-m', opt[:out].sub_ext('.map'), '-C', BUILD_PATH+'ld65.cfg', '-o', opt[:out], 
          BUILD_PATH+'base.o', BUILD_PATH+'runtime_init.o', BUILD_PATH+'runtime.o', *objs )

    end

    def ca65( path )
      sh CA65, '-l', '-I', BUILD_PATH, '-o', BUILD_PATH+path.basename.sub_ext('.o'), path
    end

    def sh( *args )
      command = args.map(&:to_s)
      dout 1, command.join(" ")
      Open3.popen2e( *command ) do |i, oe, th|
        v = th.value
        if v != 0
          raise CommandError.new( "#{args[0]} returns #{v}", command, oe.read )
        end
      end
    end

  end

end
