"""Token spend of round 5.

  python -I eval/costs5.py <subagents transcript dir> <agent id prefixes, comma-separated> <ab-orca dir> [<judge raw dir>]

fresh = input + cache_creation + output; cache reads are reported apart. Transcript assistant messages are
de-duplicated by message id. Headless runs are summed from each result event's modelUsage, which covers the
subagents a run spawned.
"""
import glob
import json
import os
import sys


def transcript(path):
    seen, fresh, read = set(), 0, 0
    for line in open(path, encoding="utf-8"):
        try:
            ev = json.loads(line)
        except ValueError:
            continue
        if ev.get("type") != "assistant":
            continue
        m = ev.get("message", {})
        if m.get("id") in seen:
            continue
        seen.add(m.get("id"))
        u = m.get("usage", {})
        fresh += u.get("input_tokens", 0) + u.get("cache_creation_input_tokens", 0) + u.get("output_tokens", 0)
        read += u.get("cache_read_input_tokens", 0)
    return fresh, read


def headless(pattern):
    n = fresh = read = 0
    usd = 0.0
    for raw in glob.glob(pattern):
        last = None
        for line in open(raw, encoding="utf-8"):
            try:
                ev = json.loads(line)
            except ValueError:
                continue
            if ev.get("type") == "result":
                last = ev
        if last is None:
            continue
        # A run whose background subagent outlives the main turn emits a second, cumulative result event.
        n += 1
        for m in (last.get("modelUsage") or {}).values():
            fresh += m.get("inputTokens", 0) + m.get("cacheCreationInputTokens", 0) + m.get("outputTokens", 0)
            read += m.get("cacheReadInputTokens", 0)
        usd += last.get("total_cost_usd") or 0.0
    return n, fresh, read, usd


def main():
    sub, ids, ab = sys.argv[1:4]
    total = 0
    print("| source | runs | fresh tokens | cache read | usd |")
    print("|---|---|---|---|---|")
    for path in sorted(glob.glob(os.path.join(sub, "agent-*.jsonl"))):
        if not any(i in os.path.basename(path) for i in ids.split(",")):
            continue
        f, r = transcript(path)
        total += f
        print("| session %s | 1 | %d | %d | |" % (os.path.basename(path)[6:16], f, r))
    for name, pat in (("A/B runs", os.path.join(ab, "raw", "*.jsonl")), ("judge", os.path.join(ab, "judge", "raw", "*.jsonl"))):
        n, f, r, usd = headless(pat)
        total += f
        print("| headless %s | %d | %d | %d | %.2f |" % (name, n, f, r, usd))
    print("| total fresh | | %d | | |" % total)


main()
