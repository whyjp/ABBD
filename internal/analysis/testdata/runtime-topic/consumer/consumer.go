package consumer

import (
	"fmt"
	"os"
)

type ReaderConfig struct{ Topic string }

type Reader struct{ cfg ReaderConfig }

func NewReader(cfg ReaderConfig) *Reader { return &Reader{cfg: cfg} }

func Subscribe(region string) *Reader {
	return NewReader(ReaderConfig{Topic: fmt.Sprintf("events.%s", region)})
}

func RegionURL(region string) string { return os.Getenv(region + "_URL") }
