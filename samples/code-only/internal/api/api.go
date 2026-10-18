package api

import (
	"encoding/json"
	"net/http"

	"example.com/heartbeat/internal/kafka"
)

const TopicMessages = "message-events"

type Server struct {
	writer kafka.Writer
	mux    *http.ServeMux
}

func NewKafkaWriter(brokers []string) kafka.Writer {
	return kafka.NewWriter(kafka.WriterConfig{Brokers: brokers, Topic: TopicMessages})
}

func NewServer(w kafka.Writer) *Server {
	s := &Server{writer: w, mux: http.NewServeMux()}
	s.mux.HandleFunc("POST /messages", s.handlePostMessage)
	s.mux.HandleFunc("GET /health", s.handleHealth)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

type message struct {
	Key  string `json:"key"`
	Body string `json:"body"`
}

func (s *Server) handlePostMessage(w http.ResponseWriter, r *http.Request) {
	var m message
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil || m.Body == "" {
		http.Error(w, "invalid message", http.StatusBadRequest)
		return
	}
	if err := s.writer.WriteMessages(r.Context(), kafka.Message{Key: []byte(m.Key), Value: []byte(m.Body)}); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("ok"))
}
