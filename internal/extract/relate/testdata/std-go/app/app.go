package app

import (
	"fmt"
	"os"

	"example.com/std/lib"
)

func Main() string {
	fmt.Println(os.Args)
	return lib.Name()
}
