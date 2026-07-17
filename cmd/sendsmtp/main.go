package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/wiz/sendsmtp/internal/engine"
)

func main() {
	configPath := "app.config.yml"
	root := &cobra.Command{
		Use:   "sendsmtp",
		Short: "SendSMTP headless CLI",
	}
	root.PersistentFlags().StringVar(&configPath, "config", "app.config.yml", "path to app.config.yml")

	withEngine := func(run func(*engine.Engine, []string) error) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, args []string) error {
			eng, err := engine.New(configPath, nil)
			if err != nil {
				return err
			}
			defer eng.Close()
			return run(eng, args)
		}
	}

	importCmd := &cobra.Command{Use: "import", Short: "Import data into SQLite"}
	importCmd.AddCommand(
		&cobra.Command{
			Use:   "all",
			Short: "Import all files from config paths",
			RunE: withEngine(func(e *engine.Engine, _ []string) error {
				res, err := e.ImportAll()
				if err != nil {
					return err
				}
				return printJSON(res)
			}),
		},
		&cobra.Command{
			Use:   "smtps [file]",
			Short: "Import SMTPs (goscan or email;password with auto-discover)",
			Args:  cobra.MaximumNArgs(1),
			RunE: withEngine(func(e *engine.Engine, args []string) error {
				path := e.Cfg.Paths.Smtps
				if len(args) > 0 {
					path = args[0]
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				res, err := e.ImportSmtpsText(string(raw))
				if err != nil {
					return err
				}
				return printJSON(res)
			}),
		},
		func() *cobra.Command {
			var validate bool
			c := &cobra.Command{
				Use:   "emails [file]",
				Short: "Import emails; --validate checks syntax + MX/DNS",
				Args:  cobra.MaximumNArgs(1),
				RunE: withEngine(func(e *engine.Engine, args []string) error {
					path := e.Cfg.Paths.Emails
					if len(args) > 0 {
						path = args[0]
					}
					raw, err := os.ReadFile(path)
					if err != nil {
						return err
					}
					res, err := e.ImportEmailsText(string(raw), validate)
					if err != nil {
						return err
					}
					return printJSON(res)
				}),
			}
			c.Flags().BoolVar(&validate, "validate", false, "validate syntax and domain MX/DNS (multithreaded)")
			return c
		}(),
		&cobra.Command{
			Use:  "subjects [file]",
			Args: cobra.MaximumNArgs(1),
			RunE: withEngine(func(e *engine.Engine, args []string) error {
				path := e.Cfg.Paths.Subjects
				if len(args) > 0 {
					path = args[0]
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				res, err := e.ImportSubjectsText(string(raw))
				if err != nil {
					return err
				}
				return printJSON(res)
			}),
		},
		&cobra.Command{
			Use:  "links [file]",
			Args: cobra.MaximumNArgs(1),
			RunE: withEngine(func(e *engine.Engine, args []string) error {
				path := e.Cfg.Paths.Links
				if len(args) > 0 {
					path = args[0]
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				res, err := e.ImportLinksText(string(raw))
				if err != nil {
					return err
				}
				return printJSON(res)
			}),
		},
		&cobra.Command{
			Use:  "html [file]",
			Args: cobra.MaximumNArgs(1),
			RunE: withEngine(func(e *engine.Engine, args []string) error {
				path := e.Cfg.Paths.HTML
				if len(args) > 0 {
					path = args[0]
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				return e.SetHTML(string(raw))
			}),
		},
	)
	root.AddCommand(importCmd)

	root.AddCommand(&cobra.Command{
		Use:   "test-smtp [id]",
		Short: "Test SMTP auth/send",
		Args:  cobra.ExactArgs(1),
		RunE: withEngine(func(e *engine.Engine, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return err
			}
			return e.TestSmtp(id, "")
		}),
	})
	root.AddCommand(&cobra.Command{
		Use:   "analyze-smtp [id]",
		Short: "MailReach free spam test (inbox placement) for one SMTP",
		Args:  cobra.ExactArgs(1),
		RunE: withEngine(func(e *engine.Engine, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return err
			}
			sum, err := e.AnalyzeSmtp(id)
			if err != nil {
				return err
			}
			return printJSON(sum)
		}),
	})
	root.AddCommand(&cobra.Command{
		Use:   "analyze-all-smtps",
		Short: "MailReach check all SMTPs (partitions seeds when SMTPs > seed list)",
		RunE: withEngine(func(e *engine.Engine, _ []string) error {
			res, err := e.AnalyzeAllSmtps()
			if err != nil {
				return err
			}
			return printJSON(res)
		}),
	})

	root.AddCommand(&cobra.Command{
		Use:   "send",
		Short: "Start campaign and wait until done",
		RunE: withEngine(func(e *engine.Engine, _ []string) error {
			if err := e.StartCampaign(); err != nil {
				return err
			}
			for {
				time.Sleep(time.Second)
				st, err := e.GetStats()
				if err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "\rpending=%d sent=%d failed=%d rate=%.1f/s state=%s",
					st.Status.Pending, st.Status.Sent, st.Status.Failed, st.Rate, st.Campaign.State)
				if st.Campaign.State == "done" || st.Campaign.State == "paused" || (st.Status.Pending == 0 && st.Status.Sending == 0 && !e.Runner.IsRunning()) {
					fmt.Fprintln(os.Stderr)
					return printJSON(st)
				}
			}
		}),
	})

	root.AddCommand(&cobra.Command{
		Use: "pause",
		RunE: withEngine(func(e *engine.Engine, _ []string) error {
			return e.PauseCampaign()
		}),
	})
	root.AddCommand(&cobra.Command{
		Use: "resume",
		RunE: withEngine(func(e *engine.Engine, _ []string) error {
			return e.ResumeCampaign()
		}),
	})
	root.AddCommand(&cobra.Command{
		Use: "status",
		RunE: withEngine(func(e *engine.Engine, _ []string) error {
			st, err := e.GetStatus()
			if err != nil {
				return err
			}
			return printJSON(st)
		}),
	})
	root.AddCommand(&cobra.Command{
		Use: "stats",
		RunE: withEngine(func(e *engine.Engine, _ []string) error {
			st, err := e.GetStats()
			if err != nil {
				return err
			}
			return printJSON(st)
		}),
	})
	root.AddCommand(&cobra.Command{
		Use: "reset-failed",
		RunE: withEngine(func(e *engine.Engine, _ []string) error {
			n, err := e.ResetFailed()
			if err != nil {
				return err
			}
			return printJSON(map[string]int64{"reset": n})
		}),
	})
	root.AddCommand(&cobra.Command{
		Use:   "delete-emails",
		Short: "Delete all recipient emails from the queue",
		RunE: withEngine(func(e *engine.Engine, _ []string) error {
			n, err := e.DeleteAllEmails()
			if err != nil {
				return err
			}
			return printJSON(map[string]int64{"deleted": n})
		}),
	})
	root.AddCommand(&cobra.Command{
		Use:   "reenable-smtps",
		Short: "Reactivate all disabled SMTPs",
		RunE: withEngine(func(e *engine.Engine, _ []string) error {
			n, err := e.ReenableSMTPs()
			if err != nil {
				return err
			}
			return printJSON(map[string]int64{"reenabled": n})
		}),
	})
	root.AddCommand(&cobra.Command{
		Use:   "clear-logs",
		Short: "Clear email/SMTP error logs",
		RunE: withEngine(func(e *engine.Engine, _ []string) error {
			r, err := e.ClearErrorLogs()
			if err != nil {
				return err
			}
			return printJSON(r)
		}),
	})

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
