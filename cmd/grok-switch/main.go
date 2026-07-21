package main

import (
	"fmt"
	"os"

	"github.com/Gelmezon/grok-switch/internal/cli"
	"github.com/Gelmezon/grok-switch/internal/exitcodes"
	"github.com/Gelmezon/grok-switch/internal/ui/theme"
)

func main() {
	setUmask()
	err := cli.Run(os.Args[1:])
	if err != nil {
		if theme.Enabled() {
			fmt.Fprintln(os.Stderr, theme.Fail(err.Error()))
		} else {
			fmt.Fprintln(os.Stderr, exitcodes.Format(err))
		}
		os.Exit(exitcodes.Code(err))
	}
}
