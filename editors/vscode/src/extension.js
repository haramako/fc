// FC 言語の VS Code 拡張 (ハイライトは syntaxes/、ここは fcc の CLI を呼ぶ部分):
//   - フォーマット: `fcc fmt <tmpfile>` の標準出力で置き換える (エディタの formatOnSave にも乗る)
//   - 診断: `fcc check -t <target> <main>` の出力 `file:line:col: error|warning: msg` を Problems に出す
//   - fcc の場所: 設定 fc.fccPath → PATH の fcc → ワークスペースの bin/windows/fcc.exe | bin/linux/fcc
'use strict';

const vscode = require('vscode');
const cp = require('child_process');
const fs = require('fs');
const os = require('os');
const path = require('path');

const OUTPUT_NAME = 'FC';
let output;
let diagnostics;

function activate(context) {
  output = vscode.window.createOutputChannel(OUTPUT_NAME);
  diagnostics = vscode.languages.createDiagnosticCollection('fc');
  context.subscriptions.push(output, diagnostics);

  context.subscriptions.push(
    vscode.languages.registerDocumentFormattingEditProvider({ language: 'fc' }, {
      provideDocumentFormattingEdits: (doc) => formatDocument(doc),
    }),
    vscode.commands.registerCommand('fc.format', async () => {
      const editor = vscode.window.activeTextEditor;
      if (editor && editor.document.languageId === 'fc') {
        await vscode.commands.executeCommand('editor.action.formatDocument');
      }
    }),
    vscode.commands.registerCommand('fc.check', async () => {
      const editor = vscode.window.activeTextEditor;
      const doc = editor && editor.document.languageId === 'fc' ? editor.document : undefined;
      await runCheck(doc, true);
    }),
    vscode.commands.registerCommand('fc.showVersion', async () => {
      const r = await runFcc(['version'], undefined);
      output.appendLine(r.stdout + r.stderr);
      output.show(true);
    }),
    vscode.workspace.onDidSaveTextDocument((doc) => {
      if (doc.languageId === 'fc' && config().get('checkOnSave', true)) {
        runCheck(doc, false);
      }
    }),
  );
}

function deactivate() {}

function config() {
  return vscode.workspace.getConfiguration('fc');
}

// fccPath は fcc の実行パスを決める (設定 → PATH → ワークスペースの bin/)。
function fccPath(folder) {
  const configured = config().get('fccPath', '');
  if (configured) {
    return configured;
  }
  const exe = process.platform === 'win32' ? 'fcc.exe' : 'fcc';
  for (const dir of (process.env.PATH || '').split(path.delimiter)) {
    if (dir && fs.existsSync(path.join(dir, exe))) {
      return path.join(dir, exe);
    }
  }
  if (folder) {
    const local = path.join(folder, 'bin', process.platform === 'win32' ? 'windows' : 'linux', exe);
    if (fs.existsSync(local)) {
      return local;
    }
  }
  return exe;
}

// runFcc は fcc を起動して {code, stdout, stderr} を返す (起動失敗は code = -1)。
function runFcc(args, cwd) {
  const folder = workspaceFolderOf(cwd);
  const exe = fccPath(folder);
  return new Promise((resolve) => {
    cp.execFile(exe, args, { cwd, maxBuffer: 16 * 1024 * 1024 }, (err, stdout, stderr) => {
      let code = 0;
      if (err) {
        code = typeof err.code === 'number' ? err.code : -1;
        if (code === -1) {
          stderr = `${exe}: ${err.message}\n` + (stderr || '');
        }
      }
      resolve({ code, stdout: stdout || '', stderr: stderr || '', exe });
    });
  });
}

function workspaceFolderOf(dir) {
  const folders = vscode.workspace.workspaceFolders || [];
  if (dir) {
    for (const f of folders) {
      if (dir.startsWith(f.uri.fsPath)) {
        return f.uri.fsPath;
      }
    }
  }
  return folders.length > 0 ? folders[0].uri.fsPath : undefined;
}

// formatDocument は未保存の内容を一時ファイルに書いて `fcc fmt` にかけ、全体を置き換える編集を返す。
async function formatDocument(doc) {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'fc-fmt-'));
  const tmp = path.join(tmpDir, path.basename(doc.fileName) || 'a.fc');
  try {
    fs.writeFileSync(tmp, doc.getText(), 'utf8');
    const r = await runFcc(['fmt', tmp], path.dirname(doc.fileName));
    if (r.code !== 0) {
      // 構文エラーなどは整形せずに知らせるだけ (メッセージの一時ファイル名を元の名前に戻す)
      const msg = (r.stderr || r.stdout).replace(tmp, doc.fileName).trim();
      output.appendLine(msg);
      vscode.window.setStatusBarMessage(`fcc fmt: ${msg.split('\n')[0]}`, 5000);
      return [];
    }
    if (r.stdout === doc.getText()) {
      return [];
    }
    const whole = new vscode.Range(doc.positionAt(0), doc.positionAt(doc.getText().length));
    return [vscode.TextEdit.replace(whole, r.stdout)];
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

// mainFileFor は fcc check の起点にするファイルを決める (設定 → 同じディレクトリの main.fc → そのファイル)。
function mainFileFor(doc) {
  const folder = workspaceFolderOf(doc ? path.dirname(doc.fileName) : undefined);
  const configured = config().get('mainFile', '');
  if (configured) {
    return folder ? path.resolve(folder, configured) : path.resolve(configured);
  }
  if (!doc) {
    return undefined;
  }
  const sibling = path.join(path.dirname(doc.fileName), 'main.fc');
  if (path.basename(doc.fileName) !== 'main.fc' && fs.existsSync(sibling)) {
    return sibling;
  }
  return doc.fileName;
}

const DIAG_RE = /^(.+?):(\d+)(?::(\d+))?: (error|warning): (.*)$/;

// runCheck は fcc check を走らせ、結果を Problems に反映する。
async function runCheck(doc, showOutput) {
  const main = mainFileFor(doc);
  if (!main) {
    vscode.window.showInformationMessage('FC: .fc ファイルを開くか、設定 fc.mainFile を指定してください');
    return;
  }
  if (doc && doc.isDirty) {
    await doc.save();
  }
  const cwd = path.dirname(main);
  const target = config().get('target', 'emu');
  const r = await runFcc(['check', '-t', target, path.basename(main)], cwd);

  diagnostics.clear();
  const byFile = new Map();
  let other = [];
  for (const line of (r.stdout + '\n' + r.stderr).split(/\r?\n/)) {
    if (!line.trim()) continue;
    const m = DIAG_RE.exec(line);
    if (!m) {
      other.push(line);
      continue;
    }
    const file = path.resolve(cwd, m[1]);
    const lineNo = Math.max(0, parseInt(m[2], 10) - 1);
    const col = m[3] ? Math.max(0, parseInt(m[3], 10) - 1) : 0;
    const range = new vscode.Range(lineNo, col, lineNo, col + 1);
    const sev = m[4] === 'error' ? vscode.DiagnosticSeverity.Error : vscode.DiagnosticSeverity.Warning;
    const d = new vscode.Diagnostic(range, m[5], sev);
    d.source = 'fcc';
    if (!byFile.has(file)) byFile.set(file, []);
    byFile.get(file).push(d);
  }
  for (const [file, ds] of byFile) {
    diagnostics.set(vscode.Uri.file(file), ds);
  }
  if (r.code === -1 || (r.code !== 0 && byFile.size === 0)) {
    // fcc が起動できない / 位置の無いエラー
    const msg = other.join('\n') || `fcc check returned ${r.code}`;
    output.appendLine(msg);
    vscode.window.showErrorMessage(`FC: ${msg.split('\n')[0]}`);
    return;
  }
  if (showOutput) {
    const n = [...byFile.values()].reduce((a, ds) => a + ds.length, 0);
    output.appendLine(`fcc check -t ${target} ${main}: ${n === 0 ? 'OK' : n + ' problem(s)'}`);
    for (const line of other) output.appendLine(line);
    vscode.window.setStatusBarMessage(`fcc check: ${n === 0 ? 'OK' : n + ' problem(s)'}`, 5000);
  }
}

module.exports = { activate, deactivate };
