# Heartbeat service — legacy architecture notes

| 모듈 | 경로 | 책임 |
|---|---|---|
| heartbeat | internal/heartbeat | heartbeat 메시지를 검증하고 중복을 제거한다 |
| storage | internal/storage | 검증된 heartbeat를 heartbeats 테이블에 저장한다 |
| api | internal/api | 외부 메시지 수신 API를 제공하고 message-events 토픽에 발행한다 |
| legacy-batch | internal/batch | 야간 배치 정산 (현재 코드에 없음) |
