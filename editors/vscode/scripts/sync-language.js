// Both extension entry points ship the same language assets. Run after editing
// the canonical copies in editors/vscode; check-grammar verifies they stay equal.
'use strict';

const fs = require('fs');
const path = require('path');
const root = path.resolve(__dirname, '..');
const target = path.resolve(root, '../../tools/vscode-fc');
for (const file of ['syntaxes/fc.tmLanguage.json', 'snippets/fc.json', 'language-configuration.json']) {
  const dest = path.join(target, file);
  fs.mkdirSync(path.dirname(dest), { recursive: true });
  fs.copyFileSync(path.join(root, file), dest);
}
console.log('FC language assets synchronized');
