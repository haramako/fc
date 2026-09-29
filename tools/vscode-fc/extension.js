// fc の VS Code 拡張 (薄い実装): シンタックスハイライト、保存時に `fcc check --json` を走らせて Problems に出す、
// `fcc fmt` で整形する (エディタの「ドキュメントのフォーマット」と formatOnSave に乗る)。
// 言語サーバは持たない (fcc check がプログラム全体を検査するので、use で届くモジュールのエラーも出る)。
const vscode = require('vscode');
const { execFile } = require('child_process');
const fs = require('fs');
const os = require('os');
const path = require('path');

let diagnostics;
let output;

function activate(context) {
  diagnostics = vscode.languages.createDiagnosticCollection('fc');
  output = vscode.window.createOutputChannel('fc');
  context.subscriptions.push(diagnostics, output);
  context.subscriptions.push(vscode.workspace.onDidSaveTextDocument((doc) => {
    if (doc.languageId === 'fc' && vscode.workspace.getConfiguration('fc').get('checkOnSave')) {
      check(doc);
    }
  }));
  context.subscriptions.push(vscode.languages.registerDocumentFormattingEditProvider({ language: 'fc' }, {
    provideDocumentFormattingEdits: (doc) => format(doc),
  }));
  context.subscriptions.push(vscode.commands.registerCommand('fc.check', () => {
    const doc = vscode.window.activeTextEditor && vscode.window.activeTextEditor.document;
    if (doc && doc.languageId === 'fc') {
      check(doc);
    }
  }));
}

function check(doc) {
  const cfg = vscode.workspace.getConfiguration('fc');
  const fcc = cfg.get('fccPath') || 'fcc';
  const target = cfg.get('target') || 'auto';
  let main = doc.fileName;
  const mainFile = cfg.get('mainFile');
  const folder = vscode.workspace.getWorkspaceFolder(doc.uri);
  if (mainFile && folder) {
    main = path.join(folder.uri.fsPath, mainFile);
  }
  const cwd = path.dirname(main);
  // auto: -t を渡さない (fcc が fc.toml に [target] があれば nes、無ければ emu にする)
  const args = ['check', '--json'].concat(target === 'auto' ? [] : ['-t', target], [path.basename(main)]);
  output.appendLine('> ' + fcc + ' ' + args.join(' ') + '  (cwd ' + cwd + ')');
  execFile(fcc, args, { cwd, windowsHide: true }, (err, stdout, stderr) => {
    if (err && err.code === 'ENOENT') {
      vscode.window.showErrorMessage('fc: ' + fcc + ' not found (set fc.fccPath)');
      return;
    }
    if (stderr) {
      output.append(stderr);
    }
    const byFile = new Map();
    for (const line of stdout.split(/\r?\n/)) {
      if (!line.startsWith('{')) {
        if (line) output.appendLine(line);
        continue;
      }
      let d;
      try { d = JSON.parse(line); } catch (e) { continue; }
      const file = path.isAbsolute(d.file) ? d.file : path.join(cwd, d.file);
      const line0 = Math.max((d.line || 1) - 1, 0);
      const col0 = Math.max((d.col || 1) - 1, 0);
      const range = new vscode.Range(line0, col0, line0, col0 + 1);
      const sev = d.severity === 'warning' ? vscode.DiagnosticSeverity.Warning : vscode.DiagnosticSeverity.Error;
      const diag = new vscode.Diagnostic(range, d.message, sev);
      diag.source = 'fcc';
      const key = path.normalize(file);
      if (!byFile.has(key)) byFile.set(key, []);
      byFile.get(key).push(diag);
    }
    // 前回の診断は全部消してから今回の分を出す (別のファイルのエラーが直ったときにも消えるように)
    diagnostics.clear();
    for (const [file, ds] of byFile) {
      diagnostics.set(vscode.Uri.file(file), ds);
    }
  });
}

// format は未保存の内容を一時ファイルに書いて `fcc fmt` にかけ、全体を置き換える編集を返す。
// 構文エラーなどで整形できなければ何も変えずに知らせる。
function format(doc) {
  const fcc = vscode.workspace.getConfiguration('fc').get('fccPath') || 'fcc';
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'fc-fmt-'));
  const tmp = path.join(dir, path.basename(doc.fileName) || 'a.fc');
  fs.writeFileSync(tmp, doc.getText(), 'utf8');
  return new Promise((resolve) => {
    execFile(fcc, ['fmt', tmp], { windowsHide: true }, (err, stdout, stderr) => {
      fs.rmSync(dir, { recursive: true, force: true });
      if (err) {
        const msg = err.code === 'ENOENT' ? fcc + ' not found (set fc.fccPath)' : (stderr || stdout || String(err)).split(tmp).join(doc.fileName).trim();
        output.appendLine('fcc fmt: ' + msg);
        vscode.window.setStatusBarMessage('fcc fmt: ' + msg.split(/\r?\n/)[0], 5000);
        resolve([]);
        return;
      }
      if (stdout === doc.getText()) {
        resolve([]);
        return;
      }
      const whole = new vscode.Range(doc.positionAt(0), doc.positionAt(doc.getText().length));
      resolve([vscode.TextEdit.replace(whole, stdout)]);
    });
  });
}

function deactivate() {}

module.exports = { activate, deactivate };
