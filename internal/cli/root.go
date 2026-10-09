// Package cli implements the magpie command line.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/veHRz/MagpieMail-Server/internal/buildinfo"
	"github.com/veHRz/MagpieMail-Server/internal/config"
)

// options carries what every command needs from the process.
type options struct {
	env        []string
	configFile string
	stdout     io.Writer
	stderr     io.Writer
}

func (o *options) loadConfig() (config.Config, error) {
	return config.Load(config.Source{File: o.configFile, Environ: o.env})
}

// reportedError marks an error that was already logged, so that Run does not
// print it a second time.
type reportedError struct{ error }

func (e reportedError) Unwrap() error { return e.error }

// Run executes the magpie command line with the given arguments (without the
// program name) and environment, and returns the process exit code.
func Run(ctx context.Context, args, env []string, stdout, stderr io.Writer) int {
	if args == nil {
		args = []string{} // cobra falls back to os.Args on nil
	}
	root := newRootCommand(&options{env: env, stdout: stdout, stderr: stderr})
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	if err := root.ExecuteContext(ctx); err != nil {
		var reported reportedError
		if !errors.As(err, &reported) {
			_, _ = fmt.Fprintf(stderr, "magpie: %v\n", err)
		}
		return 1
	}
	return 0
}

func newRootCommand(opts *options) *cobra.Command {
	root := &cobra.Command{
		Use:           "magpie",
		Short:         "MagpieMail server: self-hosted, privacy-first mail aggregation",
		Version:       fmt.Sprintf("%s (commit %s)", buildinfo.Version, buildinfo.Commit),
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetVersionTemplate("magpie version {{.Version}}\n")
	root.CompletionOptions.DisableDefaultCmd = true
	root.PersistentFlags().StringVar(&opts.configFile, "config", lookupEnv(opts.env, config.EnvConfigFile),
		"path of the YAML configuration file (env "+config.EnvConfigFile+")")

	root.AddCommand(
		newServeCommand(opts),
		placeholder("worker", "Run background jobs"),
		newMigrateCommand(opts),
		newAdminCommand(opts),
		newHealthcheckCommand(opts),
	)
	return root
}

// placeholder returns a command reserved for a later phase.
func placeholder(name, short string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: short + " (not implemented yet)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return fmt.Errorf("%s: not implemented yet", cmd.CommandPath())
		},
	}
}

func lookupEnv(env []string, name string) string {
	for _, entry := range env {
		if value, ok := strings.CutPrefix(entry, name+"="); ok {
			return value
		}
	}
	return ""
}
