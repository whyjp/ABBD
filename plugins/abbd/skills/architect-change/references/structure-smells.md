# 구조 냄새 (architect-change 3절)

후보 설계를 고르기 전에 훑는 목록이다. 항목마다 그 냄새가 ABBD의 어느 기록이나 카드로 드러나는지 적었다. "엔진이 잡음"이 있는 항목은 sync나 inspect가 실제로 표시하므로, 설계 단계에서 피하면 리뷰 화면의 카드가 줄어든다. 나머지는 에이전트가 판단한다.

| 냄새 | 알아보는 법 | 엔진이 잡음 | 대신 할 것 |
|---|---|---|---|
| 계획에 없는 구조 변경 | 후보를 그려 보니 ChangePlan의 예상 카드 밖에 의존·계약·모듈이 생긴다 | sync 후 receipt의 `structural_deltas` | 그 변경이 요구에 필요한지 다시 따지고, 필요하면 예상 카드와 IntentDelta에 넣는다 |
| 정책 밖 의존 | 새 import가 `dependency_policy.forbidden`에 있거나 `allowed` 목록 밖이다 | `inspect`의 `dependency_violations`, `forbidden_dependency` 카드(B), `check` 경고 | 의존 방향을 뒤집거나(인터페이스를 소비하는 쪽에 둠), 정책 변경을 등급 B로 묻는다 |
| 정하지 않은 계약 | 새 IF/EVT/DATA/설정 키인데 compatibility와 external_consumers를 정하지 않았다 | 확인 대기 목록의 "미상", `contract_added` 카드의 추정 배지 | `intent contract`로 호환성 초안을 남기고, 외부 소비자 여부는 지연 확인으로 넘긴다 |
| 책임이 두 문장 | 모듈 responsibility를 쓰려면 "그리고"로 이어진 두 가지 일이 필요하다 | 없음 | 모듈을 나누는 후보를 하나 더 만든다. 나누면 `module_split` 카드(B)가 생긴다는 비용도 적는다 |
| 겹치는 책임 | 새 모듈의 responsibility가 기존 모듈의 것과 같은 명사를 소유한다 | 없음 | 기존 모듈에 넣는 후보와 비교한다 |
| 길어진 기능 경로 | 같은 기능이 전보다 많은 모듈·계약을 지나는데, 늘어난 칸이 새 책임을 갖지 않는다 | `feature_path_changed` 카드(A)의 이전/이후 경로 | 그냥 전달만 하는 칸을 없앤다 |
| 소유 이동 | 계약이나 기능의 소유 모듈이 바뀌는데 IntentDelta가 그 이동을 말하지 않는다 | `ownership_moved` 카드(B) | 이동이 의도면 IntentDelta에 적고, 아니면 원래 모듈에 둔다 |
| 같은 설정을 여러 곳이 읽음 | 한 설정 키를 두 모듈이 각자 읽고 기본값을 따로 둔다 | 설정 키 계약의 소비자 목록(`contracts list`) | 한 모듈이 읽어 타입으로 넘긴다 |
| 배포 단위가 슬쩍 바뀜 | 요청이 배포를 말하지 않는데 모듈과 배포 단위의 매핑이 바뀐다 | `deploy_mapping_changed` 카드(B) | 진입점이나 Dockerfile 변경을 계획에서 빼거나 이유를 적는다 |
| 산문뿐인 불변조건 | preserved_invariants에 연결된 테스트가 없다 | `inspect FEAT-x`의 불변조건에 test가 비어 있음 | 테스트를 만들고 `intent invariant --test TEST-x`로 잇는다 |
| 같이 있으면 안 되는 필드 | 타입에 bool 두 개나 "있을 수도 있는" 필드가 있어 말이 안 되는 조합이 표현된다 | 없음 | 상태마다 다른 타입이나 열거형으로 바꾼다 |
| 검증이 계획보다 좁음 | 검증 계약이 `verification-plan`의 명령보다 적다 | `verification-plan` 출력 | 빠진 명령을 넣거나 빼는 이유를 적는다 |

냄새 하나가 후보를 탈락시키지는 않는다. 걸린 항목과, 그래도 그 후보를 고른 이유를 ChangePlan에 적는다.
