# coding: utf-8
lib = File.expand_path('../lib', __FILE__)
$LOAD_PATH.unshift(lib) unless $LOAD_PATH.include?(lib)
require 'fc/version'

Gem::Specification.new do |spec|
  spec.name          = "fc"
  spec.version       = Fc::VERSION
  spec.authors       = ["HARADA Makoto"]
  spec.email         = ["haramako@gmail.com"]
  spec.description   = %q{A compiler for NES(Family Computer)}
  spec.summary       = %q{A compiler for NES(Family Computer)}
  spec.homepage      = ""
  spec.license       = "MIT"

  spec.files         = `git ls-files`.split($/)
  spec.executables   = spec.files.grep(%r{^bin/}) { |f| File.basename(f) }
  spec.test_files    = spec.files.grep(%r{^(test|spec|features)/})
  spec.require_paths = ["lib"]

  spec.add_dependency "backports"
  spec.add_development_dependency "bundler", "~> 1.3"
  spec.add_development_dependency "rake"
  spec.add_development_dependency "rspec"
  spec.add_development_dependency "simplecov"
end
