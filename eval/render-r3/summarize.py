"""Summarise focus views against round 3's targets: phantom 0, median bends <= 2,
median path crossings <= 2. Reads after.tsv and after.tsv.store.txt."""
import csv, re, statistics, sys
rows = list(csv.DictReader(open('after.tsv', encoding='utf8'), delimiter='\t'))
store = []
for l in open('after.tsv.store.txt', encoding='utf8'):
    m = re.match(r'(.*?): relations (\d+), lost (\d+), phantom (\d+), bends median ([\d.]+) max (\d+), path crossings median ([\d.]+) max (\d+), unmarked T (\d+)', l)
    if m:
        store.append(dict(fixture='orca-store', view=m[1], relations=m[2], lost=m[3], phantom=m[4], bends_med=m[5], cross_med=m[7], unmarked_t=m[9]))
def group(rs, pred):
    return [r for r in rs if pred(r)]
def report(label, rs):
    if not rs: return
    miss = [r for r in rs if int(r['phantom']) > 0 or float(r['bends_med']) > 2 or float(r['cross_med']) > 2]
    print(f"{label}: {len(rs)} views, {len(miss)} miss; phantom sum {sum(int(r['phantom']) for r in rs)}, "
          f"median of bends medians {statistics.median(float(r['bends_med']) for r in rs)}, "
          f"median of crossing medians {statistics.median(float(r['cross_med']) for r in rs)}, "
          f"max crossing median {max(float(r['cross_med']) for r in rs)}, lost {sum(int(r['lost']) for r in rs)}, unmarked {sum(int(r['unmarked_t']) for r in rs)}")
    for r in miss: print('   miss', r['fixture'], r['view'][:70], 'phantom', r['phantom'], 'bends', r['bends_med'], 'cross', r['cross_med'])
allr = rows + store
report('fixture full views', group(rows, lambda r: ' focus ' not in r['view']))
report('fixture focus views', group(rows, lambda r: ' focus ' in r['view']))
report('pstack overview focus', group(rows, lambda r: r['fixture'] == 'pstack' and r['view'].startswith('overview focus')))
report('fixture desktop-main focus', group(rows, lambda r: r['fixture'] == 'orca' and r['view'].startswith('MOD-0042 focus')))
report('orca store desktop-main focus', group(store, lambda r: r['view'].startswith('MOD-0042 focus')))
report('orca store focus views', group(store, lambda r: ' focus ' in r['view']))
report('pstack all focus', group(rows, lambda r: r['fixture'] == 'pstack' and ' focus ' in r['view']))
