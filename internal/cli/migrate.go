package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/veHRz/MagpieMail-Server/internal/store/postgres"
)

func newMigrateCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Manage the database schema",
		Args:  cobra.NoArgs,
	}
	withMigrator := func(fn func(ctx context.Context, m *postgres.Migrator) error) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, _ []string) error {
			cfg, err := opts.loadConfig()
			if err != nil {
				return err
			}
			m, err := postgres.NewMigrator(cfg.Database.URL.Reveal())
			if err != nil {
				return err
			}
			defer m.Close()
			return fn(cmd.Context(), m)
		}
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "up",
			Short: "Apply every pending migration",
			Args:  cobra.NoArgs,
			RunE: withMigrator(func(ctx context.Context, m *postgres.Migrator) error {
				applied, err := m.Up(ctx)
				for _, mig := range applied {
					_, _ = fmt.Fprintf(opts.stdout, "applied %s\n", mig.Name)
				}
				if err == nil && len(applied) == 0 {
					_, _ = fmt.Fprintln(opts.stdout, "the database is already up to date")
				}
				return err
			}),
		},
		&cobra.Command{
			Use:   "down",
			Short: "Roll back the last applied migration",
			Args:  cobra.NoArgs,
			RunE: withMigrator(func(ctx context.Context, m *postgres.Migrator) error {
				mig, err := m.Down(ctx)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintf(opts.stdout, "rolled back %s\n", mig.Name)
				return nil
			}),
		},
		&cobra.Command{
			Use:   "status",
			Short: "List migrations and whether they are applied",
			Args:  cobra.NoArgs,
			RunE: withMigrator(func(ctx context.Context, m *postgres.Migrator) error {
				statuses, err := m.Status(ctx)
				if err != nil {
					return err
				}
				for _, s := range statuses {
					state := "pending"
					if s.Applied {
						state = "applied " + s.AppliedAt.UTC().Format("2006-01-02T15:04:05Z")
					}
					_, _ = fmt.Fprintf(opts.stdout, "%-40s %s\n", s.Name, state)
				}
				return nil
			}),
		},
	)
	return cmd
}
