package main

import (
	"fmt"

	"github.com/jessevdk/go-flags"
)

type options struct {
	Message string `long:"msg" short:"m"`
}

func main() {
	var opts options
	parser := flags.NewParser(&opts, flags.Default)
	if _, err := parser.Parse(); err != nil {
		return
	}

	fmt.Println(opts.Message)
}
