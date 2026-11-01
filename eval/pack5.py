"""Build the human review pack: each A/B pair of the same rep as P and Q, the key kept apart.

  python -I eval/pack5.py <ab-orca dir>
"""
import hashlib
import json
import os
import shutil
import sys

ISSUES = ("36652", "36697")


def main():
    ab = sys.argv[1]
    review = os.path.join(ab, "review")
    os.makedirs(review, exist_ok=True)
    rows = json.load(open(os.path.join(ab, "measure.json"), encoding="utf-8"))
    by_run = {r["run"]: r for r in rows}
    key, sheet = [], ["# 검토 기록지", "", "쌍마다 먼저 PR.md만 읽고 걸린 분을 적은 뒤 diff를 본다. 다 적은 다음 key.md를 연다.", ""]
    for issue in ISSUES:
        for k in (1, 2):
            pq = "AB" if int(hashlib.sha256(("pack-%s-%d" % (issue, k)).encode()).hexdigest(), 16) % 2 else "BA"
            d = os.path.join(review, "%s-pair%d" % (issue, k))
            os.makedirs(d, exist_ok=True)
            for label, cond in zip("PQ", pq):
                src = os.path.join(ab, "runs", "%s-%s-r%d" % (issue, cond, k))
                shutil.copyfile(os.path.join(src, "change.diff"), os.path.join(d, label + ".diff"))
                shutil.copyfile(os.path.join(src, "PR.md"), os.path.join(d, label + "-PR.md"))
                r = by_run["%s-%s-r%d" % (issue, cond, k)]
                key.append("| %s-pair%d | %s | %s-%s-r%d | %s | %d files, %s |" % (issue, k, label, issue, cond, k,
                           "A (ABBD)" if cond == "A" else "B (plain)", r["files"], ", ".join(r["modules"])))
            sheet += ["## %s pair %d" % (issue, k), "",
                      "- PR.md만 읽고 P가 바꾼 것 한 줄 / 걸린 분:", "- PR.md만 읽고 Q가 바꾼 것 한 줄 / 걸린 분:",
                      "- 코드베이스 구조에 더 맞는 쪽 (P / Q / 같음)과 이유:",
                      "- 리뷰어가 더 빨리 판단하게 하는 PR.md (P / Q / 같음)과 이유:",
                      "- 그대로 병합해도 되나 (P: 예/아니오, Q: 예/아니오), 막는 것:", ""]
    open(os.path.join(review, "key.md"), "w", encoding="utf-8").write(
        "# 열쇠 (판정을 적은 뒤에 열 것)\n\n| 쌍 | 이름 | 실행 | 조건 | 크기, 모듈 |\n|---|---|---|---|---|\n" + "\n".join(key) + "\n")
    open(os.path.join(review, "sheet.md"), "w", encoding="utf-8").write("\n".join(sheet))
    print("\n".join(key))


main()
