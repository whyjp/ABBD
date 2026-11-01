# ABBD 평가 5회차: v3 최종판으로 Orca 다시 재기

대상은 stablyai/orca main `90a537434943a6dacf8cd13125e3f5752142c1b7`(2026-11-01T10:51Z)이다. ABBD는 v3 최종판(main `044ecc5`)을 그대로 썼고, ABBD 코드는 고치지 않았다.

## 1순위 판정: 사람이 Orca의 구조를 빨리 이해하고 판단할 수 있는가

**아직 아니다. v3 설계도는 4회차보다 덜 시끄럽지만, Orca에서 가장 중요한 경계를 겉층에서 지웠다. 사람이 빨리 읽을 수 있는 것은 그림이 아니라 글이다. 모듈 속층 옆의 how 설명, 단위 목록의 책임 문장, `abbd explain`의 출력이 그렇다.**

- 겉층은 4회차보다 조용하다. 폭이 6,640px에서 4,490px로 줄었다. 선은 120개에서 34개로, 추정 점선은 80개에서 0개로 줄었다. 확인 카드 207장과 턴 화면도 없어졌다. "최종 상태만"이라는 규칙은 지켜졌다(측정, `eval/screens-orca-v3/`).
- 그러나 겉층에 남은 계약은 Electron IPC, preload 다리, 런타임 RPC, 클라우드 계약 패키지 셋뿐이다. 선 34개 중 28개(82%)가 계약을 거치지 않는 직접 의존이다. Orca의 진짜 플랫폼 경계는 의미 트랙이 찾았다. 폰과 relay 사이 WebSocket, E2EE 핸드셰이크, 푸시 게이트웨이, 네이티브 헬퍼 소켓이 그것이다. 이것들은 INFERRED라서 v3 규칙대로 겉층에서 빠졌다. 그래서 겉층은 "폰과 데스크톱은 둘 다 런타임 RPC를 쓴다"까지만 말한다. computer-use 헬퍼 셋은 선 하나 없이 떠 있다(측정, 화면 판독).
- 속층은 4회차와 같은 결함을 가진다. desktop main은 선 87개, mobile은 85개다(4회차 86, 85). 내부 단위를 세로 한 줄로 쌓고, 그 옆에 선을 밧줄처럼 꼰다. 어느 단위가 테두리의 어느 계약을 구현하는지 눈으로 따라갈 수 없다. desktop main 속층 글자 44개 중 25개가 잘린다.
- 글은 질문에 답한다. `abbd explain "mobile app (Expo)"`은 목적 한 문장, 쓰는 계약, 내부 단위, 그리고 relay 접속 URL과 푸시 데이터 블록을 증거 인용과 함께 보여 준다. 처음 온 사람이 "폰은 무엇에 기대나"를 묻는다면 이 92줄이 답이다. 반면 `abbd path`는 8문항 중 4문항에서 틀린 그림을 준다(3장).
- 화면을 사람이 직접 보고 잰 것은 아니다. 판정은 내가 스크린샷을 보고 사용자의 규칙에 대어 본 것이다.

## A/B 판정: 실제 미구현 이슈 두 건에서 /abbd가 구현을 바꿨는가

**바꾸지 않았다. 두 조건이 같은 파일, 같은 모듈, 같은 계약 경로를 골랐다. A 조건의 에이전트는 설계 단계에서 설계도에서 답을 하나도 받지 못했다. ABBD가 남긴 것은 PR.md의 "토폴로지 변화 없음" 절 하나다. 판정자는 그 절을 두 번 "무관하다"고 했다.**

- 고른 이슈는 #36652(브랜치 접두사 구분자 설정)와 #36697(에이전트와 워크스페이스 상태 색을 플러그인 테마 토큰으로)이다. 둘 다 2026-10-31에 열렸다. 둘 다 PR, 브랜치, 댓글이 없고, 프로세스 경계(shared, main, renderer)를 넘는다(`eval/ab-orca/issue-selection.md`).
- 8회 모두 성공했다. 8회 모두 tsc 새 오류가 0이다(node, web, cli 세 설정, 기준선 오류 집합과의 차). 같은 이슈 안에서 A와 B는 같은 최상위 모듈을 건드렸다. #36652는 shared, desktop main, runtime, renderer 넷이고, #36697은 shared, renderer, docs다. 어느 실행도 IPC 채널, preload API, RPC 메서드를 더하지 않았다. 새 설정은 기존 `GlobalSettings`와 런타임 store 계약을 그대로 탔다.
- A 조건 4회는 `abbd explain`을 9번 불렀다. 이름은 모두 파일이나 기능 수준(`worktree-branch-name`, `branch-prefix`, `plugin-theme-artifact`, `theme`)이었다. 9번 모두 "is not a unit of the state"로 끝났다. `abbd path`와 `impact`는 한 번도 쓰지 않았다. 그다음은 B와 똑같이 grep으로 갔다. 4회 모두 마지막에 `abbd sync`와 `abbd diff`를 돌려 "no topology change"를 PR.md에 적었다.
- 모델 판정자(opus, 조건을 모름, 쌍마다 순서를 바꿔 두 번)의 결과는 다음과 같다. 구조 적합성은 A 2쌍, B 1쌍, 1쌍은 순서를 바꾸자 답이 뒤집혀 무승부로 센다. PR 설명은 A 2쌍, B 2쌍이다. 판정 이유는 모두 구현 세부였다. 자동 이름 바꾸기의 두 번째 `lastIndexOf('/')`를 고쳤는가, 번역 키를 `en.json`에 넣었는가, 대비 기준이 Orca의 기본 색을 받아들이는가 같은 것이다. 어느 이유도 모듈 경계나 계약을 들지 않았다.
- 비용은 이슈마다 방향이 다르다. #36652는 A의 fresh 토큰 중앙값이 198k, B는 303k다. #36697은 A 135k, B 130k다. 반복이 둘뿐이고, #36652 B의 두 회는 백그라운드 서브에이전트를 띄웠다. 차이는 그 서브에이전트에서 왔다(4장).

**결론.** 이 두 이슈는 기존 설정 경로 안에서 끝나는 변경이다. 그래서 토폴로지가 바뀌지 않았고, 설계도가 도울 자리도 없었다. ABBD가 도울 수 있는 것은 계약을 바꾸는 변경이다. 4회차 J2처럼 언어나 프로세스를 넘는 결합이 seed에 바로 붙은 경우다. 그런 변경은 이번 표본에 없다. 그리고 에이전트가 설계도에 묻는 말의 단위(파일, 함수)와 설계도가 답하는 단위(모듈, 계약)가 어긋난다(E18).

**가장 큰 경고.** 미구현 이슈라 정답이 없다. 판정자는 모델이고, n은 이슈당 2쌍이다. 진짜 평가는 사용자가 `eval/ab-orca/review/`로 직접 하는 것이다. 그리고 이번 회차는 토큰 예산 3M을 넘겼다(3.67M, 6장).

## 0. 범위와 설정

- Orca. `--depth 1`로 클론한 뒤 `core.longpaths true`를 켰다. 파일 42,283개, 41초. 판단 실행의 공정성을 위해 로컬 커밋 하나로 Orca의 `.claude/settings.json`(자체 플러그인)과 `.mcp.json`(자체 MCP 서버)을 지웠다(`eval-base`). A 조건은 그 위에 `.abbd/`와 CLAUDE.md 블록을 커밋한 `abbd-base`에서 시작한다.
- 의존성. `corepack pnpm@12.14.0 install --frozen-lockfile --ignore-scripts`로 설치했다(71초). 설치 스크립트, postinstall, husky는 돌지 않았다. 워크스페이스 패키지 빌드(`build-workspace-packages.mjs`)는 Orca의 스크립트라 실행이 거부되었고, 돌리지 않았다. 그래서 `@orca/process-host`가 빌드되지 않아 tsc 기준선에 오류가 이미 있다. node 611, web 204, cli 262개이고 모두 그 패키지를 찾지 못하는 오류다. 실행의 tsc는 이 기준선 오류 집합과의 차이로 쟀다. 각 실행 worktree에는 같은 `node_modules`를 junction으로 붙였다.

## 1. 셋업 (abbd-setup을 따라)

| 단계 | 벽시계 | fresh 토큰 | 결과 |
|---|---|---|---|
| 얕은 클론 | 41초 | 0 | |
| 추출 스크립트(4회차 것) 첫 실행 | 2분 넘어 중단, 다시 66초 | 0 | 새 클론의 첫 파일 읽기가 느렸다. 원인은 백신 검사로 추정하고 확인하지 않았다. 캐시가 데워진 뒤에는 20초 |
| 의미 보충(4회차 swarm 결과를 새 트리에 다시 확인) | 0.5초 | 0 (4회차에 772,565) | 4회차 새 단위 43개 중 1개, 결합 98개 중 4개가 이틀 사이 인용을 잃어 빠졌다(`eval/runs-5/semantic-check.json`) |
| setup 1차 | 64초 | 0 | 거부: 의미 트랙의 IF가 스크립트 IF와 앵커를 공유한다(E13) |
| 스크립트와 보충 고치기 | 약 5분 | 내 세션 안 | 충돌 3건. 속 MOD 2개(`agent-hooks`, `computer`)는 스크립트가 맡게 했고(`INSIDE_EXTRA`), IF의 인용 1개를 뺐다(`eval/fixsupp5.py`) |
| setup 2차 | 75초 | 0 | 통과. 단위 140개(MOD 94, IF 32, EVT 2, DATA 5, CFG 6, FEAT 1). 관계 EXTRACTED 272, INFERRED 92. 인용 198/198 live. "conflict" 38줄(E14) |
| understand 5개 모듈 (4회차 how 글 재사용) | 44초 | 0 | 통과 |
| install | 1초 미만 | 0 | Orca에는 CLAUDE.md가 없어 새로 만들었다. 그 직후 state가 STALE이 되었다(E15) |
| sync | 30초 | 0 | CURRENT |

- 기계 시간은 약 6분이다. 셋업에 든 내 세션 작업은 약 25분이다. 4회차의 swarm(772,565 토큰, 약 9분)을 재사용했으므로 처음부터 셋업하는 비용은 4회차 표의 약 1.0M과 26분에 이번 수정분을 더한 값이다(추론).
- **v2 마찰의 v3 상태**(`eval/repro5.py`, `eval/runs-5/repro-e.txt`, 두 모듈짜리 저장소에서 재현).

| 4회차 번호 | v3에서 | 재현 결과 |
|---|---|---|
| E1 보충의 부모를 보충에 다시 적어야 함 | 남음 | `parent "app/sub" is not the key of another module`, exit 1 |
| E2 한 앵커만 적은 스크립트 단위 재기술이 부모와 겹침 | 남음 | `its anchors match more than one MOD equally`, exit 1 |
| E11 Windows longpaths | 재현 안 함 | 클론에서 미리 켰다 |
| E12 비계약 단위의 signature | 고쳐짐(일부) | 메시지가 이제 키를 말한다: `unit 0 (app/sub): only contracts carry a signature` |

## 2. 화면 판독성

스크린샷은 `eval/screens-orca-v3/`에 있다. 1440×900에서 겉층 첫 화면(`overview-1440.png`), 데스크톱 중심으로 민 겉층(`overview-center-1440.png`), 겉층 SVG 전체(`overview-whole-svg.png`), desktop main 속층(`inside-desktop-main-1440.png`), mobile 속층(`inside-mobile-1440.png`, SVG 전체 `inside-mobile-svg.png`)을 찍었다. 390×844에서 겉층(`overview-390.png`)과 desktop main 속층(`inside-desktop-main-390.png`)을 찍었다. 모든 장을 직접 봤다.

| 지표 | 4회차 (v2) | 5회차 (v3) |
|---|---|---|
| 겉층 SVG | 6,640×730 | 4,490×730 |
| 겉층 선 | 120 (추정 점선 80) | 34 (계약 경유 6, 직접 의존 28, 점선 0) |
| 겉층 상자 | | 모듈 33, 계약 포트 13 |
| 겉층 잘린 이름 | | 글자 45개 중 11개. 계약 포트는 모두 잘림 |
| desktop main 속층 | 선 86 | 선 87, 글자 44개 중 잘림 25 |
| mobile 속층 | 선 85 | 선 85 |
| 390px 페이지 가로 스크롤 | 없음 | 없음 (scrollWidth 375) |
| 카드, 큐, 턴 화면 | 카드 207장, 턴 화면 | 없음 |

### 2.1 순위를 매긴 비판 (사용자 규칙 기준)

규칙은 셋이다. 겉층은 계약을 거친 모듈 인터페이스 토폴로지다. 속층은 모듈 하나이고 그 계약을 테두리에 둔다. 최종 상태만 보인다.

1. **겉층이 Orca의 플랫폼 경계를 보여 주지 않는다.** 겉층의 계약은 IPC, preload, 런타임 RPC, 클라우드 계약 패키지 셋뿐이다. 폰과 relay, 폰과 데스크톱의 E2EE 채널, 데스크톱과 푸시 게이트웨이, 데스크톱과 네이티브 헬퍼의 소켓은 의미 트랙이 증거와 함께 찾았다. 그러나 INFERRED라서 겉층에 그리지 않는다. 클라우드 계약 패키지 셋은 제공자만 있고 소비자가 없다. 데스크톱이 그 패키지를 import하지 않고 손으로 베낀 사본을 쓰기 때문이다. 결과적으로 computer-use 헬퍼 셋은 선 없이 떠 있다. mobile과 relay 사이에도 선이 없다. 사람이 "이 제품의 플랫폼은 어떻게 이어지나"를 겉층에서 읽을 수 없다. v3 규칙(INFERRED는 겉층에 그리지 않음)과 사용자의 1순위(구조를 빨리 이해)가 Orca에서 충돌한다.
2. **겉층이 계약 토폴로지가 아니라 import 다발로 읽힌다.** 선 34개 중 28개가 주황 직접 의존이다. shared types와 process-host로 10가닥 넘게 몰린다(`overview-center-1440.png`). 계약을 거친 파란 선은 6개뿐이다.
3. **첫 화면이 엉뚱한 곳을 보여 준다.** 1440px 첫 화면에는 클라우드 패키지 다섯과 긴 주황 선 몇 가닥만 있다. 데스크톱 모듈을 보려면 그림 안에서 약 1,700px를 밀어야 한다. 맨 윗줄에 상자 19개가 놓인다. 들어오는 선이 없는 단위를 한 랭크에 모으는 배치 때문이다(4회차 E3과 같은 원인).
4. **속층이 구조를 보여 주지 않는다.** 내부 단위 12~15개를 세로 한 줄로 쌓고 import 선을 밧줄처럼 꼰다. 단위와 테두리 계약 사이의 선을 눈으로 따라갈 수 없다. mobile 속층은 테두리 위쪽이 비어 있고, 쓰는 계약과 닿는 모듈이 맨 아래 한 줄에 몰려 있다. 4회차 E5에서 바뀐 것이 없다.
5. **속층 테두리가 자기 내부 단위를 "바깥"으로 그린다.** desktop main 속층 테두리에 "계약 없이 desktop-main/agent-hooks와 닿는 곳", "계약 없이 desktop-main/ipc와 닿는 곳"이 있다. 둘 다 이 모듈 안의 단위다(E17).
6. **이름이 잘려 무엇인지 알 수 없다.** 겉층 계약 포트는 모두 "Orca runtime R…", "Electron IPC c…"다. 속층 상자는 "desktop-main/clau…"처럼 부모 접두사가 자리를 먹는다.
7. **의미 트랙의 계약에 시그니처가 없다.** "push gateway HTTP API"의 `POST /v1/host/challenge, …` 같은 시그니처를 보충에 적어도 v3는 "code has \"\""라며 버린다. 단위 목록의 "시그니처와 근거"를 펼쳐도 시그니처 칸이 비어 있다(E14).
8. **390px에서는 그림을 쓸 수 없다.** 겉층은 상자 한 개 반, 속층은 첫 화면이 거의 빈 테두리다. 페이지 자체의 가로 스크롤은 없고, 겉층에는 "옆으로 밀어 보세요" 안내가 있다.

**잘 된 것.** 확인 카드, 큐, 턴이 사라져 화면이 최종 상태 하나만 말한다. 속층 오른쪽의 how 설명은 여전히 가장 읽기 쉽다. desktop main 설명은 IPC, 런타임 콜백, WSL stdio, 네이티브 헬퍼 실행 네 길을 한 문단으로 정리한다. 단위 목록은 단위마다 책임 한 문장과 "추정" 배지를 붙여 사실과 추정을 바로 가른다.

## 3. `abbd explain`과 `abbd path`가 새로 온 사람의 질문에 답하는가

8문항을 물었다(`eval/queries5.ps1`, 출력 `eval/runs-5/queries/`). 모두 50ms 안에 끝났다.

| 질문 | 명령 | 판정 |
|---|---|---|
| 폰 앱은 무엇에 기대나 | explain mobile app | 답한다. 목적, 쓰는 계약, 내부 단위 12개, relay 접속 URL, 푸시 데이터 블록, 네이티브 모듈을 인용과 함께 보인다. 92줄로 길다 |
| 런타임 RPC는 누가 쓰나 | explain Orca runtime RPC methods | 답한다. 이름공간 시그니처, 제공자 하나와 소비자 넷, 문자열로 부르는 소비자 셋을 인용과 함께 보인다 |
| 푸시 게이트웨이는 누가 부르나 | explain push gateway HTTP API | 답한다. 라우트 인용 셋과 그것을 부르는 런타임 쪽 인용 셋 |
| 폰은 데스크톱에 어떻게 닿나 | path mobile → desktop main | 틀린 그림. "둘 다 런타임 RPC를 쓴다"는 두 소비자 사이의 경로를 낸다. WebSocket, 페어링, E2EE는 나오지 않는다(E19) |
| 폰은 relay에 어떻게 닿나 | path mobile → relay | 틀린 그림. gzip 코덱 패키지를 거쳐 돌아간다. mobile이 `mobile/transport`를 품는다는 부모 관계를 경로에 쓰지 않는다(E20) |
| macOS 헬퍼는 main과 어떻게 이어지나 | path computer-use-macos → desktop main | 반쯤. `desktop-main/computer`가 헬퍼를 부른다는 의미 결합은 맞다. 그다음 IPC 계약을 거쳐 main으로 돌아간다 |
| renderer와 main은 어떻게 이야기하나 | path renderer → desktop main | 틀린 그림. "main이 renderer에 의존한다"(import 한 줄)를 낸다. preload와 IPC는 나오지 않는다 |
| CLI와 폰은 무엇을 공유하나 | path orca CLI → mobile | 답한다. 둘 다 런타임 RPC의 소비자다 |

`explain`은 처음 온 사람의 "이것은 무엇이고 무엇에 기대나"에 답한다. `path`는 "어떻게 이어지나"에 자주 틀린 그림을 준다. 최단 경로가 소비자 둘을 같은 계약으로 잇거나, import 한 줄이나 형제 단위로 지름길을 내기 때문이다.

## 4. 구현 A/B

### 4.1 설계

- 이슈마다 조건 둘, 반복 둘. 각 실행은 새 headless `claude -p --model opus`다. 같은 Orca 커밋에서 따로 만든 worktree에서 돈다. MCP는 끄고(`--strict-mcp-config`), 실행당 상한은 12달러다(`eval/ab5.py`).
  - A. 프롬프트가 `/pstack:poteto-mode Use the abbd skill (/abbd:abbd) …`로 시작한다. `--plugin-dir plugins/abbd`, PATH에 abbd, 트리에 `.abbd/`와 CLAUDE.md 블록이 있다.
  - B. 프롬프트가 `/pstack:poteto-mode`로 시작한다. `.abbd/`, 블록, 플러그인, abbd가 없다.
  - 둘 다 그 뒤에 이슈 본문, "Implement this; do not open a PR, write the PR description to PR.md.", 환경 안내 한 문단(의존성은 설치됨, 패키지는 빌드 안 됨, 설치 명령 금지)이 온다.
- 실행 뒤에 같은 도구로 쟀다(`eval/ab5.py measure`). 바뀐 파일과 최상위 모듈(추출 스크립트의 앵커로 소유 모듈을 정함), tsc 새 오류, PR.md, 토큰, 벽시계다. 두 조건의 변경을 같은 ABBD 기준선 위에 얹어 `abbd diff`도 돌렸다(`eval/topo5.py`).

### 4.2 실행별 수치

| 실행 | 벽시계 | fresh 토큰 | 달러 | 턴 | 서브에이전트 | 파일 | 최상위 모듈 | tsc 새 오류 | 질문 | PR.md |
|---|---|---|---|---|---|---|---|---|---|---|
| 36652-A-r1 | 600초 | 139,985 | 2.52 | 63 | 0 | 15 | shared, main, runtime, renderer | 0 | 0 | 있음, 토폴로지 절 |
| 36652-A-r2 | 1,274초 | 257,148 | 4.36 | 63 | 1 | 14 | 같음 | 0 | 0 | 있음, 토폴로지 절 |
| 36652-B-r1 | 995초 | 319,035 | 5.56 | 131 | 1 | 18 | 같음 | 0 | 0 | 있음 |
| 36652-B-r2 | 1,209초 | 287,084 | 5.14 | 99 | 1 | 24 | 같음 | 0 | 0 | 있음 |
| 36697-A-r1 | 651초 | 116,860 | 1.97 | 52 | 0 | 7 | shared, renderer, docs | 0 | 0 | 있음, 토폴로지 절 |
| 36697-A-r2 | 693초 | 153,072 | 2.90 | 76 | 0 | 7 | 같음 | 0 | 0 | 있음, 토폴로지 절 |
| 36697-B-r1 | 324초 | 114,235 | 1.86 | 49 | 0 | 5 | 같음 | 0 | 0 | 있음 |
| 36697-B-r2 | 459초 | 144,811 | 2.47 | 60 | 0 | 6 | 같음 | 0 | 0 | 있음 |

**숫자를 어떻게 읽을 것인가.**
- "tsc 새 오류 0"이 묶는 것. 기준선에 이미 오류 611/204/262개가 있다. 새 파일의 오류가 기준선과 같은 문장이면 차집합에서 빠진다. 바뀐 파일이 빌드 안 된 `@orca/process-host`를 import하면 그 파일의 다른 오류가 가려질 수 있다. 그러니 "새 타입 오류 없음"의 하한으로만 읽는다. 단위 테스트는 실행마다 스스로 일부를 돌렸다. Orca의 vitest 설정이 빌드 안 된 패키지를 import해서, 여러 실행이 저장소 밖에 임시 설정을 만들어 돌렸다. 나는 테스트를 다시 돌리지 않았다.
- 파일 수 차이(#36652 B-r2 24개)는 번역 파일 10개다. B-r2만 모든 로캘에 새 문자열을 넣었다. A-r2는 `en.json`에도 넣지 않아, 판정자가 "카탈로그 CI가 실패할 것"이라고 짚었다.
- 토큰과 시간. #36652의 B 두 회와 A-r2는 백그라운드 서브에이전트를 하나 띄웠다. 그 회들이 가장 비싸다. 서브에이전트를 띄웠는지가 비용을 정했고, 조건은 정하지 않았다. 이슈당 n=2라 조건 차이를 말할 수 없다.
- 질문 0. headless에는 AskUserQuestion이 없다. 최종 답에 물음표로 끝나는 줄도 없었다. 그러니 이 숫자는 "되묻지 않았다"보다 "되물을 수 없었다"에 가깝다.

### 4.3 계약을 따랐는가 (`abbd explain`과 같은 기준선의 `abbd diff`로)

- 8회 모두 새 설정이나 토큰을 기존 경로에 실었다. #36652는 `src/shared/global-settings-types.ts`와 `default-global-settings.ts`에 필드를 더하고, 런타임 store 계약(`runtime-store-contract.ts`)의 설정 조각 타입에 실었다. 그다음 main의 이름 짓기와 renderer의 Git 설정 화면을 고쳤다. #36697은 shared의 플러그인 테마 artifact 검사와 renderer의 토큰 목록, 문서를 고쳤다.
- `abbd explain "Orca runtime RPC methods"`와 `explain "preload bridge (window.api)"`가 보여 주는 계약 중 어느 것도 바뀌지 않았다. B의 변경까지 같은 기준선(`abbd-base`)에 얹어 `abbd sync`와 `abbd diff`를 돌리면 8회 모두 "no topology change", `--check` exit 0이다(`eval/ab-orca/runs/*/topology.txt`). 계약 digest 변화도 없다. 첫 시도는 `git apply`가 줄바꿈 차이로 모든 패치를 거부한 채 "변화 없음"을 냈다. 그래서 바뀐 파일을 실행 worktree에서 그대로 복사하는 방식으로 다시 쟀다.
- 계약의 의미에서 보면 두 조건 모두 "기존 계약 안의 변경"이다. ABBD의 `diff --check`는 이것을 정확히 조용히 넘긴다. 다만 #36652는 런타임 store 계약을 통해 CLI와 폰이 받는 설정 모양을 바꾼다. 이 변화는 시그니처가 이름공간 목록인 이번 추출 스크립트에는 보이지 않는다(4회차 E6과 같은 성질).

### 4.4 판정 (모델, 조건을 모름)

`eval/judge5.py`, 원자료 `eval/ab-orca/judge/`. 판정자는 opus이고, 변경 전 Orca 트리(`.abbd/` 없음)를 읽을 수 있다. 쌍마다 X/Y 순서를 바꿔 두 번 물었다. 첫 순서는 해시로 정했다.

| 쌍 | 구조 적합 (순서1 / 순서2) | 집계 | PR 설명 (순서1 / 순서2) | 집계 |
|---|---|---|---|---|
| 36652 r1 | B / B | B | B / B | B |
| 36652 r2 | A / B | 무승부 (두 번 다 두 번째 것을 고름) | A / A | A |
| 36697 r1 | A / A | A | A / A | A |
| 36697 r2 | A / A | A | B / B | B |

- 구조 적합은 A 2, B 1, 무승부 1이다. PR 설명은 2 대 2다.
- 이유는 모두 구현 세부다. 36652 r1에서 B가 이긴 것은 자동 이름 바꾸기의 두 번째 `lastIndexOf('/')`(204행)까지 고쳤기 때문이다. A-r1은 한 곳만 고치고 PR에는 "고쳤다"고 썼다. 36697에서 A가 이긴 것은 대비 기준을 Orca의 기본 색이 통과하도록 정했고(r1), 상태 글자를 실제로 칠해지는 색조 위에서 검사했기 때문이다(r2).
- 판정자는 A의 "Topology" 절을 두 번 "irrelevant"라고 했다(36652 r2 순서1, 36697 r2 두 번). 내용이 "변화 없음"뿐이었기 때문이다.
- 판정자가 PR.md를 보므로 A의 토폴로지 절이 조건을 드러낸다. 판정자는 그것이 무엇인지 듣지 못했지만, 이 절이 PR 판정에서 감점 요인으로 작동했다.

### 4.5 사람 검토 꾸러미

`eval/ab-orca/README.md`에서 시작한다. 이슈와 쌍마다 `review/<이슈>-pair<k>/`에 두 diff(`P.diff`, `Q.diff`)와 두 PR.md가 있고, 어느 쪽이 A인지는 `review/key.md`에만 있다. 기록지는 `review/sheet.md`다. 쌍 하나에 10분 안팎을 예상한다(추정).

## 5. ABBD 결함 (재현 포함, 고치지 않음)

| # | 종류 | 내용 | 재현 |
|---|---|---|---|
| E1 | 4회차에서 남음 | 보충 단위의 부모를 보충에 다시 적지 않으면 거부한다 | `eval/repro5.py` 첫 사례 |
| E2 | 4회차에서 남음 | 스크립트 단위를 앵커 하나로 다시 적을 때 부모도 그 앵커를 가지면 "match more than one MOD equally" | `eval/repro5.py` 둘째 사례 |
| E13 | 새로, 설계 공백 | abbd-setup이 권하는 앵커(패키지 디렉터리와 그 안의 모든 파일)를 쓰면, 리더가 찾은 속 MOD나 계약의 인용은 언제나 부모 모듈의 앵커 위에 놓인다. setup은 그것을 "would merge into it"으로 거부한다. 의미 트랙이 스크립트 모듈 안에서 찾은 단위를 기록할 길이 없다. 스크립트가 맡게 하거나 인용을 버려야 한다 | Orca setup 1차(`eval/runs-5/setup1.txt`), `eval/repro5.py` E13 사례 |
| E14 | 새로 | 보충의 계약이 적은 signature를 "conflict: … code has \"\""라며 버린다. 스크립트가 볼 수 없는 계약에는 코드 쪽 signature가 없으므로, 리더가 찾은 계약은 언제나 시그니처가 빈다. Orca에서 38줄이 나왔고, 블루프린트의 해당 계약 칸이 모두 비었다 | `eval/repro5.py` E14 사례: `stored signature: ['']` |
| E15 | 새로 | setup 다음 단계인 `abbd install`이 CLAUDE.md를 쓰면 작업 트리 지문이 바뀌어 state가 곧바로 STALE이 된다. 스킬 순서대로 하면 sync를 한 번 더 돌려야 한다 | Orca에서 setup → install → status |
| E16 | 새로, 잡음 | `abbd sync`가 단위마다 "matched" 한 줄을 찍는다. Orca에서 140줄이다 | Orca에서 sync |
| E17 | 새로, 화면 | 속층 테두리가 그 모듈 자신의 내부 단위를 "계약 없이 닿는 곳"으로 그린다 | `#m-MOD-0042`의 테두리 칸 제목: "계약 없이 desktop-main/agent-hooks와 닿는 곳", "… desktop-main/ipc와 닿는 곳" |
| E18 | 새로, 사용성 | `abbd explain`에 상태에 없는 이름(파일, 함수, 기능)을 주면 "is not a unit of the state"로만 끝난다. 그 파일을 품은 단위를 알려 주지 않는다. A 실행 4회가 9번 이렇게 실패하고 설계도를 떠났다 | `abbd explain worktree-branch-name` |
| E19 | 새로, 질의 | `abbd path`가 같은 계약의 소비자 둘을 "이어진 것"으로 낸다. 통신 경로가 아니다 | `abbd path "mobile app (Expo)" "desktop main process"` |
| E20 | 새로, 질의 | `abbd path`가 parent 관계를 경로에 쓰지 않아, 형제 패키지를 거치는 우회로를 최단 경로로 낸다 | `abbd path "mobile app (Expo)" "@orca-cloud/relay"` |
| E3, E5 | 4회차에서 남음 | 겉층의 한 랭크 배치, 속층의 세로 쌓기와 꼬인 선 | 2장 |

E12는 일부 고쳐졌다. 메시지가 이제 단위 키를 말한다.

## 6. 비용

기준. fresh 토큰은 input, cache_creation, output의 합이다. 캐시 읽기는 따로 센다(`eval/costs5.py`, `eval/runs-5/costs.md`).

| 항목 | 실행 | fresh 토큰 | 캐시 읽기 | 달러 |
|---|---|---|---|---|
| 내 세션 (셋업, 화면, 하네스, 측정, 보고서) | 1 | 1,771,903 | 40.6M | |
| A/B 구현 실행 | 8 | 1,532,230 | 56.1M | 26.78 |
| 모델 판정 | 8 | 369,442 | 1.6M | 3.58 |
| 합계 | | 3,673,575 | | |

- **예산 3M을 약 0.67M 넘겼다.** 넘긴 곳은 내 세션이다. A/B 실행을 기다리는 동안 백그라운드 알림을 여러 번 30분씩 기다렸다. 그때마다 깨어날 때 캐시가 식어 맥락 전체를 다시 썼다(추론, 세션 fresh가 측정 시점마다 304k에서 1.77M으로 뛴 것에서). 실행 비용(1.53M)은 첫 파일럿으로 실행당 약 140k를 보고 잡은 계획 안이었다. 첫 실행 계산(`costs5.py` 첫판)은 결과 이벤트를 두 번 세어 2.40M으로 부풀렸다. 서브에이전트가 메인 턴보다 늦게 끝나면 누적 결과 이벤트가 하나 더 나오기 때문이다. 고친 값이 위 표다.
- 의미 보충은 4회차 swarm 결과를 재사용해 이번에 0이다. 처음부터라면 4회차 기준 약 0.77M이 더 든다.

## 7. 타당성 위협

- **판정자는 사람이 아니다.** 모델 판정자는 diff와 PR을 텍스트로 읽는다. "구조 적합"이라는 말을 들어도 구현 세부로 판정했다. 사람 꾸러미가 진짜 평가다.
- **n이 작다.** 이슈 둘, 조건 둘, 반복 둘. 판정 쌍 넷.
- **정답이 없다.** 미구현 이슈라 무엇이 맞는 구현인지 아무도 모른다. tsc 새 오류 0과 판정자의 의견이 전부다.
- **표본이 ABBD가 도울 자리를 비켜 갔다.** 두 이슈 모두 기존 계약 안의 변경이다. 고를 때 "모듈 경계를 넘는가"는 봤지만 "계약을 바꾸는가"는 보지 않았다. 계약을 바꾸는 이슈였다면 결과가 달랐을 수 있다(추측).
- **A 조건의 조작은 프롬프트 한 줄, 플러그인, 블록, 상태 넷이 한꺼번에다.** 어느 것이 효과를 냈는지 가를 수 없다. 이번에는 효과 자체가 보이지 않았다.
- **사용자 전역 설정이 두 조건에 다 들어갔다.** OMC와 RTK 지시가 담긴 `~/.claude/CLAUDE.md`, 사용자 플러그인과 훅이다. 두 조건에 같다.
- **화면 판정은 내 눈이다.** 사용자 규칙에 대어 봤지만 사람 판독 시간을 재지 않았다.
- **추출 스크립트와 보충은 4회차 것이고, 이번에 내가 고쳤다.** `INSIDE_EXTRA` 두 단위는 손으로 넣었다.

## 8. 재현

```
git clone --depth 1 https://github.com/stablyai/orca <orca>; git -C <orca> config core.longpaths true
sh eval/setup5.sh <orca> <abbd.exe> <abs eval dir> <round-4 runs-4/semantic dir>
python -I eval/understand5.py <orca> <abbd.exe> <round-4 runs-4/semantic dir>
abbd install; abbd sync
python -I eval/repro5.py <two-module repro repo> <abbd.exe>
powershell -File eval/queries5.ps1 <orca> <abbd.exe> <abs out dir>
python -I eval/ab5.py prepare <orca> <worktrees>
python -I eval/ab5.py run <orca> <worktrees> <abs eval/ab-orca> <abs plugins/abbd> <dir with abbd.exe> <run ids...>
python -I eval/ab5.py measure <orca> <worktrees> <abs eval/ab-orca> <abbd.exe> eval/runs-5/script-extraction.json
python -I eval/topo5.py <topo worktree on abbd-base> <abbd.exe> <abs eval/ab-orca> <worktrees>
python -I eval/judge5.py run <abs eval/ab-orca> <judge tree on eval-base> <work dir>; python -I eval/judge5.py score eval/ab-orca
python -I eval/pack5.py eval/ab-orca
python -I eval/costs5.py <subagents transcript dir> <agent id> eval/ab-orca
```
