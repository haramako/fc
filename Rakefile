
task :default => 'lib/fc/parser.rb'

task :test => :default do
	sh 'rm -rf coverage'
	sh 'rspec test/fc/test_*.rb'
	sh 'cd test ; ./test-all'
end

task :clean do
  sh 'rm -rf parser.output coverage'
end

file 'lib/fc/parser.rb' => 'parser.y' do
  sh 'racc -O parser.output parser.y -o lib/fc/parser.rb'
end
