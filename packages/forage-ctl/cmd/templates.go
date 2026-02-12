package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var templatesCmd = &cobra.Command{
	Use:   "templates",
	Short: "List available sandbox templates",
	RunE:  runTemplates,
}

func init() {
	rootCmd.AddCommand(templatesCmd)
}

func runTemplates(cmd *cobra.Command, args []string) error {
	p := paths()

	templates, err := config.ListTemplates(p.TemplatesDir)
	if err != nil {
		return fmt.Errorf("failed to list templates: %w", err)
	}

	if len(templates) == 0 {
		logInfo("No templates found. Configure templates in your NixOS configuration.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TEMPLATE\tAGENTS\tNETWORK\tDESCRIPTION")
	fmt.Fprintln(w, "--------\t------\t-------\t-----------")

	for _, t := range templates {
		agents := make([]string, 0, len(t.Agents))
		for name := range t.Agents {
			agents = append(agents, name)
		}
		agentStr := strings.Join(agents, ",")
		if agentStr == "" {
			agentStr = "-"
		}

		network := t.Network
		if network == "" {
			network = "full"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", t.Name, agentStr, network, t.Description)
	}

	return w.Flush()
}
