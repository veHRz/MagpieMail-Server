package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/veHRz/MagpieMail-Server/internal/security/envelope"
	"github.com/veHRz/MagpieMail-Server/internal/store/postgres"
)

func newAdminCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Administer the instance",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "generate-key",
			Short: "Print a new random master key (base64) on stdout",
			Long: "Print a new random master key. Store it as a secret and back it up outside the server:\n" +
				"without it, encrypted account credentials and blobs are lost.",
			Args: cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error {
				_, err := fmt.Fprintln(opts.stdout, envelope.GenerateKey())
				return err
			},
		},
		&cobra.Command{
			Use:   "rotate-key",
			Short: "Rewrap every user key with the current master key",
			Long: "Rotate the master key without downtime:\n" +
				"  1. generate a new key (magpie admin generate-key);\n" +
				"  2. set it as security.master_key and move the old one to security.previous_master_keys;\n" +
				"  3. restart the server, then run this command;\n" +
				"  4. remove the old key from the configuration.",
			Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return rotateKey(cmd, opts)
			},
		},
	)
	return cmd
}

func rotateKey(cmd *cobra.Command, opts *options) error {
	ctx := cmd.Context()
	cfg, err := opts.loadConfig()
	if err != nil {
		return err
	}
	ring, err := cfg.Security.Keyring()
	if err != nil {
		return err
	}
	pool, err := postgres.Open(ctx, cfg.Database.URL.Reveal())
	if err != nil {
		return err
	}
	defer pool.Close()
	store := postgres.NewStore(pool, ring)

	rewrapped, err := store.RotateMasterKey(ctx)
	if err != nil {
		return fmt.Errorf("rotating the master key (nothing was changed): %w", err)
	}
	if err := store.Audit().Append(ctx, postgres.AuditEvent{
		Action:     "admin.master_key_rotated",
		TargetType: "master_key",
		TargetID:   ring.CurrentID(),
		Details:    map[string]any{"rewrapped": rewrapped, "master_key_id": ring.CurrentID()},
	}); err != nil {
		return fmt.Errorf("recording the rotation in the audit log: %w", err)
	}

	noun := "user keys"
	if rewrapped == 1 {
		noun = "user key"
	}
	_, err = fmt.Fprintf(opts.stdout, "rewrapped %d %s; every user key now uses master key %s\n"+
		"Previous master keys can now be removed from the configuration.\n", rewrapped, noun, ring.CurrentID())
	return err
}
