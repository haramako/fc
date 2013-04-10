
.PHONY: all
all: lib/fc/parser.rb

.PHONY: test
test: all
	rm -rf coverage
	rspec test/fc/test_*.rb
	cd test ; ./test-all

lib/fc/parser.rb: parser.y
	racc -O parser.output parser.y -o lib/fc/parser.rb

.PHONY: clean
clean:
	rm -rf a.asm a.nes a.html parser.output coverage
