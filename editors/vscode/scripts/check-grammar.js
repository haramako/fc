// 文法 (syntaxes/fc.tmLanguage.json) の検査: 読み込めること、リポジトリの .fc 全部をトークン化して
// 落ちないこと、代表的なトークンに期待したスコープが付くこと。`npm run check-grammar`。
'use strict';

const fs = require('fs');
const path = require('path');
const vsctm = require('vscode-textmate');
const oniguruma = require('vscode-oniguruma');
const assert = require('assert/strict');

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
  ['options(bank: -1, inline: true);\n', 'bank', 'variable.parameter.option.fc'],
  ['options(bank: -1, inline: true);\n', 'true', 'constant.language.fc'],
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
  ['public alias work:Work = shared.memory;', 'alias', 'storage.type.alias.fc'],
  ['public alias work:Work = shared.memory;', 'work', 'variable.other.declaration.fc'],
  ['alias /* shared */ work:Work = memory;', 'alias', 'storage.type.alias.fc'],
  ['alias work:[32]uint8 = memory;', 'uint8', 'storage.type.primitive.fc'],
  ['function alias():void {}', 'alias', 'entity.name.function.fc'],
  ['var alias:uint8; alias = 1;', 'alias', 'variable.other.fc'],
  ['alias();', 'alias', 'entity.name.function.call.fc'],
  ['block { var a:uint8; } options(bss: "BSS_EX");', 'block', 'keyword.other.placement.fc'],
  ['block { var a:uint8; } options(bss: "BSS_EX");', 'bss', 'variable.parameter.option.fc'],
  ['var block:uint8; block = 1;', 'block', 'variable.other.fc'],
  ['block();', 'block', 'entity.name.function.call.fc'],
  ['var handler:farfn(uint8):void;', 'farfn', 'storage.type.fc'],
  ['var handler:fn(uint8):void;', 'fn', 'storage.type.fc'],
  ['function draw(n:uint8 = 1 << 2):void {}', '<<', 'keyword.operator.shift.fc'],
  ['function draw(n:uint8 = 4):void {}', '=', 'keyword.operator.assignment.fc'],
  ['function draw(n:uint8 = 4):void {}', '4', 'constant.numeric.decimal.fc'],
  ['const _T = textmap("font.txt");', 'textmap', 'support.function.builtin.fc'],
  ['asm("lda #1");', 'asm', 'support.function.builtin.fc'],
  ['var textmap:uint8;', 'textmap', 'variable.other.declaration.fc'],
  ['options(address: (BASE + sizeof(Work)), bss: "BSS_EX");', 'bss', 'variable.parameter.option.fc'],
  ['options(address: (BASE + sizeof(Work)), bss: "BSS_EX");', 'sizeof', 'keyword.operator.cast.fc'],
  ['options(// bss: ignored\n bss: "BSS_EX");', ' bss: ignored', 'comment.line.double-slash.fc'],
  ['use /* module */ shared;', ' module ', 'comment.block.fc'],
  ['const MASK = 0xff >> 1;', '>>', 'keyword.operator.shift.fc'],
];

(async () => {
  // The tools/ extension has a different diagnostics entry point, but uses the
  // exact same grammar/snippets/configuration as the packaged extension.
  const otherRoot = path.join(repo, 'tools/vscode-fc');
  for (const file of ['syntaxes/fc.tmLanguage.json', 'snippets/fc.json', 'language-configuration.json']) {
    assert.deepEqual(JSON.parse(fs.readFileSync(path.join(root, file), 'utf8')),
      JSON.parse(fs.readFileSync(path.join(otherRoot, file), 'utf8')), `${file}: run npm run sync-language`);
  }
  for (const dir of [root, otherRoot]) {
    const pkg = JSON.parse(fs.readFileSync(path.join(dir, 'package.json'), 'utf8'));
    assert(pkg.contributes.snippets.some((s) => s.language === 'fc' && s.path === './snippets/fc.json'));
  }
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
