package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/kmrgirish/pprof-adv/internal/cpu"
	"github.com/kmrgirish/pprof-adv/pb"
	"github.com/kmrgirish/pprof-adv/profiler"
)

type Cmd struct {
	Profile string `arg:"--profile"  help:"path to pprof file"`
	Type    string `arg:"--type"     help:"type of pprof"  default:"cpu"`
	AttrCPU bool   `arg:"--attr-cpu" help:"Attribute the cpu usages by child functions of stdlib/third-party functions to the parent function" default:"true"`

	DdApiKey string `arg:"--dd-api-key,env:DD_API_KEY" help:"Datadog API key" default:""`
	DdAppKey string `arg:"--dd-app-key,env:DD_APP_KEY" help:"Datadog application key" default:""`

	Service     string `arg:"--apm" help:"Datadog apm name, for which to download cpu profile, (this option isn't used if --profile is provided)" default:""`
	Environment string `arg:"--environment" help:"Environment name" default:"production"`
	Runtime     string `arg:"--runtime" help:"Runtime name" default:"go"`
}

func main() {
	var cmd Cmd
	arg.MustParse(&cmd)

	var f io.Reader
	if cmd.Profile != "" {
		ff, err := os.Open(cmd.Profile)
		if err != nil {
			fail("Error opening file: %s", err)
		}
		defer ff.Close()

		f = ff
	} else if cmd.Service != "" {
		client, err := profiler.NewClient(cmd.DdApiKey, cmd.DdAppKey, "")
		if err != nil {
			fail("Error creating profiler client: %s", err)
		}

		ff, err := client.GetCPUProfile(context.Background(), cmd.Service, cmd.Environment, cmd.Runtime, time.Hour, 1)
		if err != nil {
			fail("Error getting CPU profile: %s", err)
		}

		f = ff
	} else {
		fail("Either --profile or --apm must be provided")
	}

	cmd.processPprof(f)
}

func (cmd *Cmd) processPprof(f io.Reader) {
	profile, err := pb.Parse(f)
	if err != nil {
		fail("Error parsing file: %s", err)
	}

	switch cmd.Type {
	case "cpu":
		if err := cpu.Transform(profile, os.Stdout, cmd.AttrCPU); err != nil {
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
