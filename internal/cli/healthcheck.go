package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

const healthcheckTimeout = 3 * time.Second

// newHealthcheckCommand returns the container health probe. The runtime image
// has no shell nor curl, so the binary probes itself. The command is hidden: it
// is meant for HEALTHCHECK instructions, not for people.
func newHealthcheckCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:    "healthcheck",
		Short:  "Exit 0 if the local server is ready, 1 otherwise",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return healthcheck(cmd.Context(), opts)
		},
	}
}

func healthcheck(ctx context.Context, opts *options) error {
	cfg, err := opts.loadConfig()
	if err != nil {
		return err
	}
	url, err := readinessURL(cfg.Server.Listen)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, healthcheckTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}
	client := &http.Client{Transport: &http.Transport{}} //nolint:forbidigo // Loopback probe of this very server, not egress; no proxy.
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("probing %s: %w", url, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("probing %s: status %d", url, resp.StatusCode)
	}
	return nil
}

// readinessURL returns the readiness endpoint of a server listening on listen,
// reached through loopback when it listens on every interface.
func readinessURL(listen string) (string, error) {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", fmt.Errorf("server.listen: %w", err)
	}
	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/readyz", nil
}
