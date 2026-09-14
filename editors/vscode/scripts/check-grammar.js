// 文法 (syntaxes/fc.tmLanguage.json) の検査: 読み込めること、リポジトリの .fc 全部をトークン化して
// 落ちないこと、代表的なトークンに期待したスコープが付くこと。`npm run check-grammar`。
'use strict';

const fs = require('fs');
const path = require('path');
const vsctm = require('vscode-textmate');
const oniguruma = require('vscode-oniguruma');

const root = path.resolve(__dirname, '..');
const repo = path.resolve(root, '..', '..');
const grammarPath = path.join(root, 'syntaxes', 'fc.tmLanguage.json');

async function loadRegistry() {
  const wasmBin = fs.readFileSync(path.join(require.resolve('vscode-oniguruma'), '..', 'onig.wasm')).buffer;
  const onigLib = oniguruma.loadWASM(wasmBin).then(() => ({
    createOnigScanner: (s) => new oniguruma.OnigScanner(s),
    createOnigString: (s) => new oniguruma.OnigString(s),
  }));
  return new vsctm.Registry({
    onigLib,
    loadGrammar: async () => vsctm.parseRawGrammar(fs.readFileSync(grammarPath, 'utf8'), grammarPath),
  });
}

function tokenize(grammar, text) {
  let state = vsctm.INITIAL;
  const out = [];
  for (const line of text.split(/\r?\n/)) {
    const r = grammar.tokenizeLine(line, state);
    for (const t of r.tokens) {
      out.push({ text: line.slice(t.startIndex, t.endIndex), scopes: t.scopes });
    }
    state = r.ruleStack;
  }
  return out;
}

function walk(dir, acc) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) {
      if (e.name !== 'node_modules' && !e.name.startsWith('.')) walk(p, acc);
    } else if (e.name.endsWith('.fc')) {
      acc.push(p);
    }
  }
  return acc;
}

const expectations = [
  // [ソース, トークン文字列, 含まれるべきスコープ]
  ['#fc 2\n', '#fc', 'keyword.control.directive.version.fc'],
  ['// hi\n', ' hi', 'comment.line.double-slash.fc'],
  ['/* a\nb */ x\n', 'b ', 'comment.block.fc'],
  ['var s = "a\\nb";\n', '\\n', 'constant.character.escape.fc'],
  ["var t = 'x';\n", "'", 'string.quoted.single.fc'],
  ['const N = 0x1F;\n', '0x1F', 'constant.numeric.hex.fc'],
  ['const B = 0b1010;\n', '0b1010', 'constant.numeric.binary.fc'],
  ['function main():void {}\n', 'main', 'entity.name.function.fc'],
  ['public struct Point { x:int; }\n', 'Point', 'entity.name.type.fc'],
  ['soa const T:[4]Point = [];\n', 'T', 'entity.name.type.fc'],
  ['use * from stdio;\n', 'stdio', 'entity.name.namespace.fc'],
  ['use a, b from m;\n', 'from', 'keyword.control.import.fc'],
  ['options(bank: -1, fastcall: true);\n', 'bank', 'variable.parameter.option.fc'],
  ['options(bank: -1, fastcall: true);\n', 'true', 'constant.language.fc'],
  ['var p:*Point;\n', 'Point', 'entity.name.type.fc'],
  ['var q:geo.Point;\n', 'geo', 'entity.name.namespace.fc'],
  ['var a:[4]int;\n', 'int', 'storage.type.primitive.fc'],
  ['outer: loop {}\n', 'outer', 'entity.name.label.fc'],
  ['x = f(1);\n', 'f', 'entity.name.function.call.fc'],
  ['x <<= 1;\n', '<<=', 'keyword.operator.assignment.compound.fc'],
  ['y = x as int16;\n', 'as', 'keyword.operator.cast.fc'],
  ['p = null;\n', 'null', 'constant.language.fc'],
  ['if (a != b) {}\n', '!=', 'keyword.operator.comparison.fc'],
  ['const MAX = 3;\n', 'MAX', 'variable.other.declaration.fc'],
  ['x = MAX;\n', 'MAX', 'variable.other.constant.fc'],
  ['case 1:\n', 'case', 'keyword.control.flow.fc'],
  ['include("main.asm");\n', 'include', 'keyword.control.include.fc'],
];

(async () => {
  const registry = await loadRegistry();
  const grammar = await registry.loadGrammar('source.fc');
  if (!grammar) throw new Error('grammar not loaded');

  let failed = 0;
  for (const [src, text, scope] of expectations) {
    const toks = tokenize(grammar, src);
    const hit = toks.find((t) => t.text === text && t.scopes.includes(scope));
    if (!hit) {
      failed++;
      console.error(`NG: ${JSON.stringify(src)}: ${JSON.stringify(text)} should have ${scope}`);
      for (const t of toks) console.error(`    ${JSON.stringify(t.text)} -> ${t.scopes.slice(1).join(' ')}`);
    }
  }

  const files = walk(path.join(repo, 'test'), [])
    .concat(walk(path.join(repo, 'fclib'), []), walk(path.join(repo, 'examples'), []));
  let tokens = 0;
  for (const f of files) {
    tokens += tokenize(grammar, fs.readFileSync(f, 'utf8')).length;
  }
  console.log(`corpus: ${files.length} files, ${tokens} tokens`);
  if (failed > 0) {
    console.error(`${failed} expectation(s) failed`);
    process.exit(1);
  }
  console.log('grammar OK');
})().catch((e) => {
  console.error(e);
  process.exit(1);
});
