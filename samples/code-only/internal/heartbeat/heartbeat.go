package heartbeat

import (
	"encoding/json"
	"errors"
	"time"
)

const TopicHeartbeat = "heartbeat-events"

type Heartbeat struct {
	ID     string    `json:"id"`
	Source string    `json:"source"`
	At     time.Time `json:"at"`
}

func Parse(data []byte) (Heartbeat, error) {
	var h Heartbeat
	err := json.Unmarshal(data, &h)
	return h, err
}

func Validate(h Heartbeat) error {
	switch {
	case h.ID == "":
		return errors.New("heartbeat: missing id")
	case h.Source == "":
		return errors.New("heartbeat: missing source")
	case h.At.IsZero():
		return errors.New("heartbeat: missing timestamp")
	}
	return nil
}
