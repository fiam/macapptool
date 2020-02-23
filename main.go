package main

import (
	"context"
	"flag"
	"os"

	"github.com/google/subcommands"
)

var (
	verbose = flag.Int("v", 0, "Verbosity level")
	dryRun  = flag.Bool("n", false, "Dry run mode. Don't perform any changes, just print them to stdout")
)

func main() {
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(subcommands.FlagsCommand(), "")
	subcommands.Register(subcommands.CommandsCommand(), "")
	subcommands.Register(&fixCmd{}, "")
	subcommands.Register(&signCmd{}, "")

	flag.Parse()
	ctx := context.Background()
	os.Exit(int(subcommands.Execute(ctx)))
}
