# ChangePlan 템플릿 (architect-change 출력)

ChangePlan은 턴의 대화나 작업 메모에 남기고, 그중 IntentDelta에 해당하는 항목은 `abbd intent delta`로 CHG에 기록한다. 절 순서는 그대로 두고, 해당 없는 절에는 "없음"과 한 줄 이유를 쓴다.

```
ChangePlan (CHG-xxx)

1. 요구
   - IntentDelta 요약: additions / changes·removals / preserved_invariants (ID로)
   - rework: WorkflowPlan의 rework 줄과 이번 계획이 그것을 어떻게 다루는지 (되돌림 / 다른 안)

2. 사용 예
   - 이 변경을 쓰는 쪽 코드 한두 줄. 아래 타입은 여기서 끌어낸다

3. 책임 모듈
   - MOD-x 또는 새 디렉터리. responsibility 한 문장 (origin)
   - dependency_policy 확인 결과, 바꾸면 intent module 명령

4. 타입과 시그니처
   - 새 타입, 공개 함수, 오류 방식. 본문 없음

5. 계약 변경
   - 계약 ID, 변경(added/removed/modified), breaking 여부, compatibility, external_consumers

6. 예상 구조 변경 카드
   - kind, subject, 등급. 등급 C가 있으면 구현 전 정지 (impact-review 5절)

7. 후보
   - 고른 후보: 구조 한 줄, 고른 이유
   - 버린 후보: 구조 한 줄, 버린 이유 (카드 수, 의존 정책, 불변조건, 계약 중 무엇 때문인지)
   - 걸린 구조 냄새와 그래도 고른 이유

8. 감수하는 비용
   - 이 설계가 나중에 더 비싸게 만드는 변경, 늘어나는 공개 범위

9. 검증 계약
   - 항목 → 증명 방법 → 검증 수준. UNVERIFIABLE 후보와 이유
   - verification-plan과의 차이와 이유

10. 열린 결정
   - 등급 B 질문(ADR ID), 카드로 받을 결정, 지연 확인 대상
```

작은 변경이면 2, 7, 8절을 한 줄씩으로 줄여도 된다. 6절과 9절은 줄이지 않는다. 구현 뒤 receipt의 카드와 6절을 비교하는 것이 계획이 맞았는지 보는 가장 싼 방법이다.
