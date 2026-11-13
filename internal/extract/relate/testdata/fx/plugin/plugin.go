package plugin

import "example.com/fx/core"

type Dog struct{}

func (Dog) Speak() string { return "woof" }

var _ core.Speaker = Dog{}
