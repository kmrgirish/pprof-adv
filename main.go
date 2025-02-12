package main

import (
	"fmt"
	"os"

	"github.com/alexflint/go-arg"
	"github.com/kmrgirish/pprof-adv/internal/cpu"
	"github.com/kmrgirish/pprof-adv/pb"
)

type Cmd struct {
	Profile string `arg:"--profile" help:"path to pprof file"`
	Type    string `arg:"--type" help:"type of pprof" default:"cpu"`
}

func main() {
	var cmd Cmd
	arg.MustParse(&cmd)

	f, err := os.Open(cmd.Profile)
	if err != nil {
		fail("Error opening file: %s", err)
	}
	defer f.Close()

	profile, err := pb.Parse(f)
	if err != nil {
		fail("Error parsing file: %s", err)
	}

	switch cmd.Type {
	case "cpu":
		if err := cpu.Transform(profile, os.Stdout); err != nil {
			fail("Error transforming profile: %s", err)
		}
	default:
		fail("Unsupported type: %s", cmd.Type)
	}
}

func fail(format string, values ...any) {
	fmt.Printf(format, values...)
	os.Exit(1)
}
