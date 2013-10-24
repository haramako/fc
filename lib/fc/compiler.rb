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
    else
      NESASM = 'nesasm'
    end

    BUILD_PATH = Pathname.new(".fc-build/")

    def initialize
    end

    def compile( src_filename, opt = Hash.new )
      opt = {out:'a.nes', target:'emu'}.merge(opt)

      base = File.basename(opt[:out],'.*')
      Fc::LIB_PATH << Pathname(Fc::FC_HOME + 'fclib' + opt[:target])

      # ソースコードを中間コードにコンパイル
      hlc = Fc::Hlc.new
      hlc.compile( src_filename )
      mods = hlc.modules
      prog_bank_count = hlc.options[:bank_count] || 2

      if opt[:debug_info]
        open( base+'.html', 'w' ) do |f|
          f.write Fc::HtmlOutput.new.module_to_html( hlc.modules )
        end
      end

      # pp hlc
      # exit


      # 中間コードをアセンブラにコンパイルする
      FileUtils.mkdir_p( BUILD_PATH )
      llc = Fc::Llc.new( opt )
      mods.each do |path,mod|
        asm, inc = llc.compile_module( mod )
        IO.write( BUILD_PATH+("_#{mod.id}.inc"), inc.join("\n") )
        IO.write( BUILD_PATH+("_#{mod.id}.s"), asm.join("\n") )
      end

      objs = []
      mods.each do |path,mod|
        #.oファイルの作成
        objs << BUILD_PATH+('_'+mod.id.to_s+'.o')
        sh 'ca65', '-I', BUILD_PATH, BUILD_PATH+('_'+mod.id.to_s+'.s')
      end

      if opt[:debug_info]
        open( base+'.html', 'w' ) do |f|
          f.write Fc::HtmlOutput.new.module_to_html( hlc.modules )
        end
      end

      case hlc.options[:mapper]
      when nil, "MMC0"
        inesmap = 0
      when "MMC3"
        inesmap = 4
      when Numeric
        inesmap = hlc.options[:mapper]
      end

      # オプションのアセンブラ
      options = { inesprg: 2, ineschr: 1, inesmir: 1, inesmap: inesmap }
      template = IO.read( Fc.find_share('base.asm.erb') ) 
      str = ERB.new(template,nil,'-').result(binding)
      IO.write( BUILD_PATH+'base.s', str )
      ca65 BUILD_PATH+'base.s'
      ca65 FC_HOME+'share/runtime.asm'
      ca65 FC_HOME+'fclib'+opt[:target]+'runtime_init.asm'

      cfg = IO.read( FC_HOME+'share/ld65.cfg' )
      IO.write( BUILD_PATH+'ld65.cfg', cfg )

      return if opt[:asm]
      
      sh 'ld65', '-C', BUILD_PATH+'ld65.cfg', '-o', base+'.nes', BUILD_PATH+'base.o', BUILD_PATH+'runtime_init.o', BUILD_PATH+'runtime.o', *objs

      if opt[:run]
        emu = Fc::FC_HOME + 'bin/emu6502'
        result = `#{emu} #{base}.nes`
        print result
      end
    rescue CommandError => err
      raise Fc::CompileError.new( err.message + "\n" + err.result )
    end

    def ca65( path )
      sh 'ca65', '-I', BUILD_PATH, '-o', BUILD_PATH+path.basename.sub_ext('.o'), path
    end

    def sh( *args )
      command = args.map(&:to_s)
      Open3.popen2e( *command ) do |i, oe, th|
        v = th.value
        if v != 0
          raise CommandError.new( "#{args[0]} returns #{v}", command, oe.read )
        end
      end
    end

  end

end
