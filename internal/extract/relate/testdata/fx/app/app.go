package app

import (
	"example.com/fx/core"
	"example.com/fx/plugin"
	"example.com/fx/shapes"
)

func Main() string {
	var b shapes.Box
	b.N = shapes.Max
	var s core.Speaker = plugin.Dog{}
	return s.Speak() + core.Run(s)
}
