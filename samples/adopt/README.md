# adopt 샘플

`samples/code-only` 저장소에 예전 방식의 아키텍처 문서(`legacy/`)가 함께 있는 경우다.
adopt는 이관이 아니라 재구성이다. 구조와 관계는 코드에서 다시 추출하고, 문서의 의미 필드
(책임·목적)만 `origin: ADOPTED`로 가져온다. 충돌과 매칭 실패는 BootstrapReport에 남는다.
adopt 입력이 없거나 깨져도 기본 bootstrap은 그대로 진행된다 (AC-BOOT-05).

```sh
cp -r samples/code-only /tmp/adopt-demo && cp -r samples/adopt/legacy /tmp/adopt-demo/
cd /tmp/adopt-demo && git init -q && git add -A && git commit -qm v0
cccv init . --mode adopt --from legacy --preview   # 쓰지 않고 결과만 본다
cccv init . --mode adopt --from legacy
```

자동 검증은 `tests/greenfield_test.go`의 `TestAdoptMode`다.
