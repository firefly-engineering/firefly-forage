package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/audit"
)

var auditLogCmd = &cobra.Command{
	Use:   "audit-log <name>",
	Short: "Display the audit trail for a sandbox",
	Args:  cobra.ExactArgs(1),
	RunE:  runAuditLog,
}

var auditLogJSON bool

func init() {
	auditLogCmd.Flags().BoolVar(&auditLogJSON, "json", false, "Output events as JSON lines")
	rootCmd.AddCommand(auditLogCmd)
}

func runAuditLog(cmd *cobra.Command, args []string) error {
	name := args[0]
	p := paths()

	auditLogger := audit.NewLogger(p.StateDir)
	events, err := auditLogger.Events(name)
	if err != nil {
		return fmt.Errorf("failed to read audit log: %w", err)
	}

	if len(events) == 0 {
		logInfo("No events found for sandbox %s", name)
		return nil
	}

	for _, e := range events {
		if auditLogJSON {
			data, err := json.Marshal(e)
			if err != nil {
				return fmt.Errorf("failed to marshal event: %w", err)
			}
			fmt.Println(string(data))
		} else {
			ts := e.Timestamp.Local().Format("2006-01-02 15:04:05")
			if e.Details != "" {
				fmt.Printf("[%s] %-8s %s (%s)\n", ts, e.Type, e.Sandbox, e.Details)
			} else {
				fmt.Printf("[%s] %-8s %s\n", ts, e.Type, e.Sandbox)
			}
		}
	}

	return nil
}
