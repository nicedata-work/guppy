// Command guppy is the CLI for scaffolding projects from templates
// (https://nicedata.work/guppy). One binary, two faces: `guppy` is the
// global management CLI; `guppy project` (shimmed as `gup`) is the
// project-scoped face.
package main

import (
	"os"

	"github.com/nicedata-work/guppy/internal/cli"
)

func main() {
	os.Exit(cli.Execute(cli.ArgsFor(os.Args[0], os.Args[1:]), os.LookupEnv, os.Stdin, os.Stdout, os.Stderr))
}
