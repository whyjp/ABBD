package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"

	"example.com/heartbeat/internal/api"
	"example.com/heartbeat/internal/heartbeat"
	"example.com/heartbeat/internal/kafka"
	"example.com/heartbeat/internal/storage"
)

func main() {
	brokers := strings.Split(os.Getenv("KAFKA_BROKERS"), ",")
	store := storage.NewWriter(os.Getenv("HEARTBEAT_DB_URL"))
	reader := kafka.NewReader(kafka.ReaderConfig{Brokers: brokers, Topic: heartbeat.TopicHeartbeat, GroupID: "heartbeat-worker"})
	defer reader.Close()

	go func() {
		addr, ok := os.LookupEnv("HEARTBEAT_HTTP_ADDR")
		if !ok {
			addr = ":8080"
		}
		log.Fatal(http.ListenAndServe(addr, api.NewServer(api.NewKafkaWriter(brokers))))
	}()

	if err := heartbeat.NewConsumer(reader, store).Run(context.Background()); err != nil {
		log.Fatal(err)
	}
}
