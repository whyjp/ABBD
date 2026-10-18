# 벤치마크 v3 파일럿 진행 기록

단계마다 이 문서에 사실만 추가한다. 점수는 실제 실행 결과만 적는다. 파일럿 부분집합 점수는 공식 리더보드 점수가 아니다.

## 공통 조건

- 모델 `claude-sonnet-5-5`, 구독 인증(`claude setup-token`, Harbor `CLAUDE_FORCE_OAUTH`). API 과금 없음.
- 실행 환경 WSL2 Ubuntu-24.04, Docker 29.4.1, 16코어, 15 GB.
- ABBD 엔진 바이너리는 `5e5eb43` 코드(= `4ec437c`)로 빌드. 플러그인은 `abbd-bench-pub/arms/{C,D,E}`에 복사해 고정.
- 그룹 A 순정 claude-code, B pstack, C ABBD 스킬만, D 스킬 + 훅(그래프 질의 차단), E 전체 플러그인. F는 `abbd domain`이 스텁이라 제외.

## Track A. Terminal-Bench 2.0 (pilot15)

- 데이터셋 `terminal-bench@2.0`, 커밋 `69671fb`. 과제 15개는 `manifests/frozen_task_manifest.json`의 pilot 목록.
- 실행 `-n 2`, 그룹당 1회. 그룹 순서 A, B, C, D, E.
- 2026-10-19 00:16 KST 그룹 A 시작. 01:36 기준 15개 중 12개 완료.
- 그룹 A의 `path-tracing`은 `AgentSetupTimeoutError`로 끝났다. 에이전트 설치 단계의 시간 초과라 에이전트 실패가 아니다. 사용자 지시에 따라 A~E가 모두 끝난 뒤 같은 예외로 끝난 시행을 그룹별로 다시 돌린다(job 이름 `pilot-tb2-<G>-4ec437c-infra-rerun`). 집계는 재실행 결과로 대체하고, 대체했다는 사실을 표에 적는다.

## Track C. SlopCodeBench 준비 (모델 호출 없음, 2026-10-19 완료)

- 고정: slop-code-bench `31ceea3`, scb-problems `9cd9ca3`. 문제 36개, 체크포인트 196개를 다시 셌고 파일럿 문제의 체크포인트 수가 매니페스트와 같다.
- 공식 평가기 무모델 확인(`refcheck.sh`): 참조 해답이 `file_backup` 4개 체크포인트(32/32, 50/50, 68/68, 89/89), `execution_server` 6개 체크포인트 전부 통과. 빈 제출은 `file_backup` 4/32, `execution_server` 0/45.
- 그룹 A~E 설정이 실제 러너 인터페이스(`claude_code`, 구독 OAuth)로 동작한다. 가짜 `claude`로 A와 E를 끝까지 돌렸다. E에서 `abbd init`은 한 번만 실행되고 `.abbd/`는 다음 체크포인트로 이어진다.
- `.abbd/`는 채점에 영향이 없다. 품질 지표는 `.py`만 읽는다. 같은 해답에 `.abbd/`가 있을 때와 없을 때 공식 결과가 시각과 파일 순서를 빼고 같았다.
- 사용량 한도 모의 시험: 체크포인트 3에서 `limit_stopped`로 기록되고, 리셋 뒤 같은 명령으로 그 체크포인트부터 재개된다. 재개 직후 체크포인트의 `lines_added`가 전체 줄 수로 잡히는 러너 특성이 있어 정규화 단계에서 다시 계산해야 한다.
- 미확인: 실제 모델 실행 없음. 2.1.302의 한도 메시지 형식은 추정 패턴이다. 체크포인트당 5~20분(추정)이라 그룹당 파일럿 8문제 58개 체크포인트에 5~15시간이 걸릴 수 있다.

## Track B. SWE-Milestone 준비 (모델 호출 없음, 2026-10-19)

- 고정: SWE-Milestone `17a8f15`, 데이터 `v1.0.2` = `3500478`. 워커에 `ast-grep`이 필요해 `uv sync --extra dev`로 설치했다.
- 이미지는 레지스트리에서 digest로 받는다. go-zero 25개 약 41 GB, Element Web 20개 약 63 GB.
- 공식 평가기 무모델 확인, go-zero M001:
  - 참조 최종 상태: resolved, N2P 134/134, P2P 2425/2425, 111초.
  - 초기 상태: 미해결, N2P 16/134, P2P 2414/2425.
- Element Web `maintenance_ui_ux` 초기 상태 평가는 95분이 지나도 진행 중이었다. 결과는 `~/bench/swe-milestone/gold/`에 남는다.
- 가짜 `claude`로 go-zero 마일스톤 2개를 실제 하네스와 평가기로 돌렸다. A 708초, E 1258초. E의 `abbd init`은 저장소당 한 번 약 100초 걸렸다. 두 그룹 점수는 같았다(아무것도 고치지 않았으니 같아야 한다).
- `.abbd/`는 채점 대상에 들어가지 않는다. 가짜 에이전트가 `git add -A`로 `.abbd/` 2130개 파일을 커밋했지만, 평가기가 받는 스냅샷 두 개 모두 `.abbd` 경로가 0개였다.
- 어댑터는 하네스의 `claude-code` 에이전트 항목을 대체한다. 이 이름일 때만 하네스가 사용량 한도 대기, 비용, 로그 파싱을 하기 때문이다. 모든 그룹에서 보조 모델 슬롯까지 `claude-sonnet-5-5`로 고정했다(B의 pstack 서브에이전트가 Opus로 가지 않게).
- 마일스톤당 에이전트 시간은 측정하지 않았다. 10~40분이면(추정) go-zero 23개 마일스톤에 그룹당 5~15시간이다.
- 디스크: WSL 802 GB 여유, 호스트 C: 약 87 GB 여유(2026-10-19 02시 측정). WSL 디스크 파일이 C:에 있으므로 Element Web까지 받으면 여유가 줄어든다.
