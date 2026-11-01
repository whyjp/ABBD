"""ABBD extraction script for Orca: units and relations from manifests and imports.

Rules, all mechanical:
- One MOD per workspace package (package.json, Cargo.toml, Package.swift, binding.gyp)
  and per Electron process directory under src/ (main, preload, renderer, shared, cli,
  wsl-guest). src/main/runtime is its own MOD because the CLI, mobile and web clients
  reach it over RPC, not the desktop main process.
- Inside units: the INSIDE_MAX largest immediate subdirectories of a main module with
  at least INSIDE_MIN non-test source files become MODs whose parent is that module.
- depends_on: a non-test TS/JS file of one module imports a file or package of another.
- Contracts (IF): Electron IPC channels (main handles, preload invokes), the preload
  bridge (contextBridge names, renderer reads window.<name>), runtime RPC methods
  (defineMethod in the runtime, consumers import shared/rpc-contract), and the cloud
  contract packages (provided by the app of the same stem, consumed by importers).
Prints JSON sorted. No clock, network or model.
"""
import hashlib
import json
import os
import re
import subprocess
import sys

INSIDE_MIN = 15
INSIDE_MAX = 12
# Inside units the round-4 readers named that the size rule misses. abbd v3 refuses a reader's inside MOD whose
# quotes sit under its parent's directory anchor, so the script has to own them.
INSIDE_EXTRA = {'desktop-main': ('agent-hooks', 'computer')}
SRC_EXT = ('.ts', '.tsx', '.mts', '.cts', '.js', '.mjs', '.cjs', '.jsx')
TEST = re.compile(r'(\.test\.|\.spec\.|\.test-fixture\.|/__tests__/|/__fixtures__/|/fixtures/|/goldens/|/test-support/|/orca-runtime-tests/|\.stories\.)')

MODULES = [
    # key, name, root, excluded roots, inside-unit base (None: no inside units)
    ('desktop-main', 'desktop main process', 'src/main', ['src/main/runtime'], 'src/main'),
    ('desktop-runtime', 'Orca runtime (RPC host)', 'src/main/runtime', [], 'src/main/runtime'),
    ('preload', 'desktop preload bridge', 'src/preload', [], None),
    ('renderer', 'desktop renderer UI', 'src/renderer', [], 'src/renderer/src'),
    ('shared', 'shared types and schemas', 'src/shared', [], 'src/shared'),
    ('cli', 'orca CLI', 'src/cli', [], 'src/cli'),
    ('wsl-guest', 'WSL guest agent', 'src/wsl-guest', [], None),
    ('mobile', 'mobile app (Expo)', 'mobile', ['mobile/modules', 'mobile/packages'], 'mobile/src'),
]
ALIASES = {'@renderer/': 'src/renderer/src/'}
AT_ROOT = {'renderer': 'src/renderer/src/', 'mobile': 'mobile/src/', 'preload': 'src/renderer/src/'}
MANIFESTS = ('package.json', 'Cargo.toml', 'Package.swift', 'binding.gyp')
PKG_GLOBS = ('src/packages/', 'mobile/modules/', 'mobile/packages/', 'cloud/apps/', 'cloud/packages/', 'native/')

IMPORT = re.compile(r'''(?:\bfrom\s*|\bimport\s*\(\s*|\brequire\s*\(\s*|^\s*import\s+|\bexport\s+\*\s+from\s*)['"]([^'"\n]+)['"]''', re.M)
IPC_MAIN = re.compile(r'''ipcMain\.(?:handle|handleOnce|on|once)\(\s*['"]([^'"]+)['"]''')
IPC_PRELOAD = re.compile(r'''ipcRenderer\.(?:invoke|send|sendSync|on|once)\(\s*['"]([^'"]+)['"]''')
BRIDGE = re.compile(r'''exposeInMainWorld\(\s*['"]([A-Za-z_$][\w$]*)['"]''')
RPC_METHOD = re.compile(r'''defineMethod\(\s*\{\s*(?:[^{}]*?\b)?(?:method|name)\s*:\s*['"]([A-Za-z][\w.]*)['"]''', re.S)
RPC_STREAM = re.compile(r'''define(?:Streaming)?Method\(\s*\{[^{}]*?(?:method|name)\s*:\s*['"]([A-Za-z][\w.]*)['"]''', re.S)


def sh(*args):
    return subprocess.run(args, capture_output=True, text=True, encoding='utf-8', check=True).stdout


def read(path):
    with open(path, encoding='utf-8', errors='replace') as f:
        return f.read()


def digest(parts):
    return hashlib.sha256('\n'.join(parts).encode()).hexdigest()[:16]


def main():
    files = sorted({f for f in sh('git', 'ls-files', '--cached', '--others', '--exclude-standard').split('\n')
                    if f and os.path.isfile(f)})
    units, rels = {}, set()
    roots = []  # (root, key, excluded)
    for key, name, root, excl, _ in MODULES:
        roots.append((root, key, excl))
        units[key] = {'key': key, 'kind': 'MOD', 'name': name, 'anchors': []}
    pkg_names = {}
    for f in files:
        g = next((g for g in PKG_GLOBS if f.startswith(g)), None)
        if g is None or f.count('/') <= g.count('/'):
            continue
        d = g + f[len(g):].split('/')[0]
        manifest = os.path.basename(f) in MANIFESTS and os.path.dirname(f) == d
        if not (manifest or g == 'native/'):
            continue
        key = d.split('/')[0] + '-' + d.split('/')[-1] if d.startswith(('cloud/', 'native/')) else d.split('/')[-1]
        if key not in units:
            units[key] = {'key': key, 'kind': 'MOD', 'name': d.split('/')[-1], 'anchors': [{'path': d}]}
            roots.append((d, key, []))
        if manifest or (len(units[key]['anchors']) < 3 and not TEST.search(f) and '_test' not in f):
            units[key]['anchors'].append({'path': f})
        if manifest and f.endswith('package.json'):
            pname = json.loads(read(f)).get('name')
            if pname:
                pkg_names[pname] = d
                units[key]['name'] = pname
    roots.sort(key=lambda r: -len(r[0]))

    def owner(path):
        for root, key, excl in roots:
            if (path == root or path.startswith(root + '/')) and not any(path.startswith(e + '/') for e in excl):
                return key
        return None

    for key, name, root, excl, _ in MODULES:
        subs = sorted({f[len(root) + 1:].split('/')[0] for f in files if f.startswith(root + '/') and owner(f) == key})
        for s in subs:
            p = root + '/' + s
            units[key]['anchors'].append({'path': p})

    inside_of = {}
    for key, _, root, _, base in MODULES:
        if not base:
            continue
        counts = {}
        for f in files:
            if f.startswith(base + '/') and owner(f) == key and f.endswith(SRC_EXT) and not TEST.search(f) and f.count('/') > base.count('/') + 1:
                s = f[len(base) + 1:].split('/')[0]
                counts[s] = counts.get(s, 0) + 1
        top = [s for s, n in sorted(counts.items(), key=lambda kv: (-kv[1], kv[0]))[:INSIDE_MAX] if n >= INSIDE_MIN]
        for s in sorted(set(top) | set(INSIDE_EXTRA.get(key, ()))):
            ik = key + '/' + s
            p = base + '/' + s
            entry = sorted(f for f in files if f.startswith(p + '/') and f.endswith(SRC_EXT) and not TEST.search(f))[0]
            units[ik] = {'key': ik, 'kind': 'MOD', 'name': ik, 'parent': key,
                         'anchors': [{'path': p}, {'path': entry}]}
            inside_of[p] = ik

    def inside(path, key):
        best = None
        for p, ik in inside_of.items():
            if path.startswith(p + '/') and ik.split('/')[0] == key:
                best = ik
        return best

    def resolve(src, spec, key):
        if spec.startswith('.'):
            target = os.path.normpath(os.path.join(os.path.dirname(src), spec)).replace('\\', '/')
        elif spec.startswith('@/') and key in AT_ROOT:
            target = AT_ROOT[key] + spec[2:]
        elif any(spec.startswith(a) for a in ALIASES):
            a = next(a for a in ALIASES if spec.startswith(a))
            target = ALIASES[a] + spec[len(a):]
        else:
            parts = spec.split('/')
            name = '/'.join(parts[:2]) if spec.startswith('@') else parts[0]
            if name not in pkg_names:
                return None
            target = pkg_names[name]
        return target

    ipc_main, ipc_pre, bridge_names, rpc_methods = {}, {}, {}, {}
    rpc_consumers, bridge_users, contract_users = set(), set(), {}
    contract_pkgs = {d: n for n, d in pkg_names.items() if d.startswith('cloud/packages/') and d.endswith('-contract')}
    for f in files:
        if not f.endswith(SRC_EXT) or TEST.search(f):
            continue
        key = owner(f)
        if key is None:
            continue
        t = read(f)
        for m in IMPORT.finditer(t):
            target = resolve(f, m.group(1), key)
            if target is None:
                continue
            tk = owner(target)
            if tk and tk != key:
                rels.add((key, 'depends_on', tk, None))
            if tk == key:
                a, b = inside(f, key), inside(target, key)
                if a and b and a != b:
                    rels.add((a, 'depends_on', b, None))
            if 'shared/rpc-contract' in target and key not in ('desktop-runtime', 'shared'):
                rpc_consumers.add(key)
            for d in contract_pkgs:
                if (target == d or target.startswith(d + '/')) and tk != key:
                    contract_users.setdefault(d, set()).add(key)
        if key == 'desktop-main':
            for c in IPC_MAIN.findall(t):
                ipc_main.setdefault(c, f)
        if key == 'preload':
            for c in IPC_PRELOAD.findall(t):
                ipc_pre.setdefault(c, f)
            for b in BRIDGE.findall(t):
                bridge_names.setdefault(b, f)
        if key == 'desktop-runtime':
            for m in RPC_STREAM.findall(t):
                rpc_methods.setdefault(m, f)
        if key == 'renderer' and 'window.api' in t:
            bridge_users.add(key)

    def contract(key, name, sig_items, all_items, anchor_files):
        anchors = [{'path': p} for p in sorted(set(anchor_files))[:2]]
        units[key] = {'key': key, 'kind': 'IF', 'name': name,
                      'signature': ' '.join(sorted(sig_items)),
                      'digest': digest(sorted(all_items)), 'anchors': anchors}

    ns = lambda names, sep: {n.split(sep)[0] for n in names}
    contract('ipc-channels', 'Electron IPC channels', ns(ipc_main, ':'), ipc_main, ipc_main.values())
    rels.add(('desktop-main', 'provides', 'ipc-channels', None))
    rels.add(('preload', 'consumes', 'ipc-channels', None))
    api_types = 'src/preload/api-types.ts'
    block = re.search(r'^export type PreloadApi = \{(.*?)^\}', read(api_types), re.M | re.S).group(1)
    api_keys = re.findall(r'^\s+(\w+)\??:', block, re.M)
    contract('preload-bridge', 'preload bridge (window.api)', api_keys, sorted(bridge_names) + [block],
             list(bridge_names.values()) + [api_types])
    rels.add(('preload', 'provides', 'preload-bridge', None))
    for u in sorted(bridge_users):
        rels.add((u, 'consumes', 'preload-bridge', None))
    contract('runtime-rpc', 'Orca runtime RPC methods', ns(rpc_methods, '.'), rpc_methods,
             ['src/main/runtime/rpc/core.ts', 'src/main/runtime/rpc/methods'])
    rels.add(('desktop-runtime', 'provides', 'runtime-rpc', None))
    for u in sorted(rpc_consumers):
        rels.add((u, 'consumes', 'runtime-rpc', None))
    for d, pname in sorted(contract_pkgs.items()):
        ck = 'cloud-' + d.split('/')[-1]
        mk = units.pop(ck)
        srcs = sorted(f for f in files if f.startswith(d + '/src/') and f.endswith('.ts') and not TEST.search(f))
        exports = []
        for f in srcs:
            exports += re.findall(r'^export\s+(?:const|type|interface|function|enum|class)\s+(\w+)', read(f), re.M)
        units[ck] = {'key': ck, 'kind': 'IF', 'name': pname, 'signature': ' '.join(sorted(set(exports))),
                     'digest': digest([read(f) for f in srcs]), 'anchors': mk['anchors']}
        roots_keys = [r for r in roots if r[1] == ck]
        app = 'cloud-' + d.split('/')[-1][:-len('-contract')]
        if app in units:
            rels.add((app, 'provides', ck, None))
        for u in sorted(contract_users.get(d, ())):
            if u != app:
                rels.add((u, 'consumes', ck, None))
        rels = {r for r in rels if r[2] != ck or r[1] != 'depends_on'}
        rels = {r for r in rels if r[0] != ck}

    for u in units.values():
        u['anchors'] = sorted({a['path']: a for a in u['anchors']}.values(), key=lambda a: a['path'])
        if len(u['anchors']) < 2:
            sys.exit('unit %s has fewer than two anchors' % u['key'])
        for a in u['anchors']:
            if not os.path.exists(a['path']):
                sys.exit('unit %s anchor %s does not exist' % (u['key'], a['path']))
    out = {'units': [units[k] for k in sorted(units)],
           'relations': [{'from': a, 'kind': k, 'to': b} for a, k, b, _ in sorted(rels)]}
    json.dump(out, sys.stdout, indent=1, sort_keys=True)
    sys.stdout.write('\n')


main()
