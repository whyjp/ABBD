# greenfield 샘플

빈 디렉터리와 자연어 요청 한 줄만 있는 시작점이다 (PRD 01A, 12장 두 번째 E2E).
이 디렉터리를 비어 있는 새 위치로 옮긴 뒤 아래를 실행한다. 사전 문서는 필요 없다.

```sh
mkdir new-service && cd new-service
cccv init .
cccv run "$(cat ../request.txt)"
# 에이전트가 코드와 테스트를 쓴다
cccv verify
cccv sync
cccv status
```

같은 흐름을 자동으로 검증하는 테스트는 `tests/greenfield_test.go`의 `TestGreenfieldE2E`다.
