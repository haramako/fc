# 差分テストの失敗プログラムを小さくする (雑な delta debugging)。
#   python reduce.py prog.fc fcc.exe [O0|panic]
# 1) 行を 1 つずつ消す (ブロックの { } は対で消す)、2) 括弧の部分式を定数に置き換える。症状が残る変更だけ採用。
# 症状: 既定は「-O 0 と -O 2 の出力が違う」。第 3 引数 panic なら「どちらかのレベルで panic する」。
import subprocess, sys, io, re

src_path, fcc = sys.argv[1], sys.argv[2]
mode = sys.argv[3] if len(sys.argv) > 3 else 'diff'

def run(src, o0):
    io.open('red_tmp.fc', 'w', encoding='utf-8').write(src)
    args = [fcc, 'run', '-t', 'emu'] + (['-O', '0'] if o0 else []) + ['red_tmp.fc']
    try:
        r = subprocess.run(args, capture_output=True, text=True, timeout=60)
    except subprocess.TimeoutExpired:
        return 'TIMEOUT'
    if r.returncode != 0:
        return 'PANIC' if 'panic:' in (r.stdout + r.stderr) else 'ERR'
    return r.stdout.strip().split('\n')[-1]

def bad(src):
    a = run(src, True)
    b = run(src, False)
    if mode == 'panic':
        return a == 'PANIC' or b == 'PANIC'
    return a != 'ERR' and b != 'ERR' and a != 'PANIC' and b != 'PANIC' and a != b

src = io.open(src_path, encoding='utf-8').read()
assert bad(src), 'not reproducing'

def try_lines(src):
    lines = src.split('\n')
    i = 0
    changed = False
    while i < len(lines):
        l = lines[i].strip()
        if re.match(r'la\d+\[\d+\] = ', l):
            i += 1; continue  # local array init: removing it makes uninitialized reads (UB)
        if l == '' or l.startswith('#fc') or l.startswith('use ') or l.startswith('function') or l.startswith('printf') or l.startswith('exit') or l in ('{', '}'):
            i += 1
            continue
        if l.endswith('{'):
            # ブロックごと消す
            depth = 0
            j = i
            while j < len(lines):
                depth += lines[j].count('{') - lines[j].count('}')
                if depth == 0:
                    break
                j += 1
            cand = '\n'.join(lines[:i] + lines[j + 1:])
            if bad(cand):
                lines = cand.split('\n'); changed = True; continue
            # ブロックの頭と尻だけ消す (中身は残す)
            cand = '\n'.join(lines[:i] + lines[i + 1:j] + lines[j + 1:])
            if bad(cand):
                lines = cand.split('\n'); changed = True; continue
            i += 1
            continue
        cand = '\n'.join(lines[:i] + lines[i + 1:])
        if bad(cand):
            lines = cand.split('\n'); changed = True; continue
        i += 1
    return '\n'.join(lines), changed

def try_exprs(src):
    stack, spans = [], []
    for i, c in enumerate(src):
        if c == '(':
            stack.append(i)
        elif c == ')' and stack:
            j = stack.pop(); spans.append((j, i))
    spans.sort(key=lambda p: -(p[1] - p[0]))
    for (i, j) in spans:
        inner = src[i + 1:j]
        if len(inner) <= 1 or src[max(0, i - 8):i].rstrip().endswith(('function', 'printf', 'exit', 'if', 'while', 'for', 'switch')):
            continue
        for rep in ['0', '1', '3']:
            cand = src[:i] + rep + src[j + 1:]
            if bad(cand):
                return cand, True
    return src, False

while True:
    src, c1 = try_lines(src)
    src, c2 = try_exprs(src)
    if not (c1 or c2):
        break
io.open(src_path.replace('.fc', '_min.fc'), 'w', encoding='utf-8').write(src)
print(src)
print('-O 0:', run(src, True))
print('-O 2:', run(src, False))
