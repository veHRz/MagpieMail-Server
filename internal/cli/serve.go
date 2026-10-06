package cli

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/spf13/cobra"

	"github.com/veHRz/MagpieMail-Server/internal/buildinfo"
	"github.com/veHRz/MagpieMail-Server/internal/config"
	"github.com/veHRz/MagpieMail-Server/internal/observability"
	"github.com/veHRz/MagpieMail-Server/internal/store/postgres"
	"github.com/veHRz/MagpieMail-Server/internal/transport/httpapi"
)

func newServeCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Serve the HTTP API until interrupted",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return serve(cmd.Context(), opts)
		},
	}
}

// serve runs the HTTP server until ctx is cancelled. Logs go to stderr so that
// stdout stays free for command output. Once the logger exists, errors are
// logged as JSON records rather than printed as plain text.
func serve(ctx context.Context, opts *options) error {
	cfg, err := opts.loadConfig()
	if err != nil {
		return err
	}
	logger, err := observability.NewLogger(opts.stderr, cfg.Log.Level, cfg.Log.Format)
	if err != nil {
		return err
	}

	if err := runServer(ctx, cfg, logger); err != nil {
		logger.ErrorContext(ctx, "magpie stopped on error", slog.Any("error", err))
		return reportedError{err}
	}
	logger.InfoContext(ctx, "magpie stopped")
	return nil
}

func runServer(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	logger.InfoContext(ctx, "starting magpie", slog.String("version", buildinfo.Version), slog.Any("config", cfg))

	pool, err := postgres.Open(ctx, cfg.Database.URL.Reveal())
	if err != nil {
		return err
	}
	defer pool.Close()

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", cfg.Server.Listen)
	if err != nil {
		return fmt.Errorf("server.listen: %w", err)
	}
	logger.InfoContext(ctx, "http server listening", slog.String("addr", ln.Addr().String()))

	router := httpapi.NewRouter(httpapi.Deps{Logger: logger, Database: pool})
	return httpapi.Serve(ctx, ln, router, cfg.Server, logger)
}
