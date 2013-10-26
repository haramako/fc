
task :default => 'lib/fc/parser.rb'

task :test => :default do
	FileUtils.rm_rf ['coverage']
	sh 'rspec test/fc/test_*.rb'
	Dir.chdir('test'){ sh 'ruby test-all' }
end

task :clean do
  sh 'rm -rf parser.output coverage'
end

file 'lib/fc/parser.rb' => 'parser.y' do
  sh 'racc -O parser.output parser.y -o lib/fc/parser.rb'
end
