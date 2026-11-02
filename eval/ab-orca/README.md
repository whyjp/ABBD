# 사람 검토 꾸러미: Orca 실제 이슈 두 건의 A/B 구현

미구현 이슈라 정답이 없다. 모델 판정은 대리일 뿐이고, 이 꾸러미로 사용자가 직접 보는 것이 실제 평가다.

## 무엇이 들어 있나

- `issues/36652.md`, `issues/36697.md`. 각 실행이 받은 이슈 본문 그대로.
- `issue-selection.md`. 왜 이 둘을 골랐고, 구현이 없다는 것을 어떻게 확인했는지.
- `review/<이슈>-pair<k>/`. 같은 반복 번호의 두 구현을 `P`와 `Q`로 이름 붙였다. 어느 쪽이 ABBD 조건인지는 `review/key.md`에만 있다. 먼저 보지 말 것.
  - `P.diff`, `Q.diff`: 코드 변경(.abbd/ 제외).
  - `P-PR.md`, `Q-PR.md`: 실행이 쓴 PR 설명.
- `review/sheet.md`. 쌍마다 적을 칸.
- `runs/<실행>/`. 실행별 원자료: `change.diff`, `PR.md`, `tsc-new-errors.txt`, `topology.txt`(모든 실행의 변경을 같은 ABBD 기준선에 얹어 잰 토폴로지 차이), A 조건이면 `abbd-diff.txt`, `final-reply.txt`.
- `raw/`. 실행마다 headless `claude -p`의 stream-json 전사와 메타(벽시계, 프롬프트).
- `judge/`. 모델 판정자의 원자료와 `verdicts.json`.

주의. A 조건의 PR.md에는 "Topology" 절(`abbd diff` 결과)이 있어서 PR.md를 읽으면 조건이 드러난다. 블라인드를 지키려면 2단계에서 그 절을 건너뛴다.

## 보는 순서 (쌍 하나에 10분 안팎)

1. 이슈를 읽는다.
2. `P-PR.md`와 `Q-PR.md`만 읽고, 각 PR이 무엇을 어디서 바꿨는지 한 줄로 적는다. 걸린 시간도 적는다.
3. diff를 열어 2의 한 줄이 맞는지, 그리고 설정이 사는 곳, 프로세스 경계(main, preload, renderer, shared), 이웃 코드의 관례를 따랐는지 본다.
4. `sheet.md`에 판정을 적은 뒤 `key.md`를 연다.
