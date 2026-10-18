FROM golang:1.22 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /out/heartbeat-worker ./cmd/heartbeat-worker

FROM gcr.io/distroless/static
COPY --from=build /out/heartbeat-worker /heartbeat-worker
ENTRYPOINT ["/heartbeat-worker"]
