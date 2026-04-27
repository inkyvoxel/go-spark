package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/inkyvoxel/go-spark/internal/generator"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		logger.Error("go-spark failed", "err", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("missing command; use new")
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "new":
		return runNew(args[1:], stdin, stdout)
	default:
		return fmt.Errorf("unknown command %q; use new", args[0])
	}
}

func runNew(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var opts generator.ProjectOptions
	var features string
	target := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		target = args[0]
		args = args[1:]
	}

	fs.StringVar(&opts.ProjectName, "project-name", "", "project name for docs, README, and UI")
	fs.StringVar(&opts.ModulePath, "module-path", "", "Go module path")
	fs.StringVar(&opts.DatabasePath, "database-path", "", "default SQLite database path")
	fs.StringVar(&opts.EmailFrom, "email-from", "", "default email sender address")
	fs.StringVar(&features, "features", "", "comma-separated feature IDs, or all")
	fs.BoolVar(&opts.Yes, "yes", false, "accept defaults for omitted options")
	fs.BoolVar(&opts.Force, "force", false, "write into an existing non-empty directory")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if target == "" {
		if fs.NArg() != 1 {
			return fmt.Errorf("new requires exactly one target path")
		}
		target = fs.Arg(0)
	} else if fs.NArg() != 0 {
		return fmt.Errorf("new requires exactly one target path")
	}
	opts.TargetPath = target
	if strings.TrimSpace(features) != "" {
		opts.Features = []string{features}
	}

	gen := generator.New()
	gen.Stdin = stdin
	gen.Stdout = stdout
	_, err := gen.NewProject(opts)
	return err
}
