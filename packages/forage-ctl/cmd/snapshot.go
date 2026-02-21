package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/workspace"
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Manage workspace snapshots",
	Long: `Create, list, and restore VCS-level snapshots of sandbox workspace state.
Snapshots use jj bookmarks or git tags depending on the workspace backend.`,
}

var snapshotCreateCmd = &cobra.Command{
	Use:   "create <sandbox> <name>",
	Short: "Create a workspace snapshot",
	Args:  cobra.ExactArgs(2),
	RunE:  runSnapshotCreate,
}

var snapshotListCmd = &cobra.Command{
	Use:   "list <sandbox>",
	Short: "List workspace snapshots",
	Args:  cobra.ExactArgs(1),
	RunE:  runSnapshotList,
}

var snapshotRestoreCmd = &cobra.Command{
	Use:   "restore <sandbox> <name>",
	Short: "Restore a workspace snapshot",
	Args:  cobra.ExactArgs(2),
	RunE:  runSnapshotRestore,
}

func init() {
	snapshotCmd.AddCommand(snapshotCreateCmd)
	snapshotCmd.AddCommand(snapshotListCmd)
	snapshotCmd.AddCommand(snapshotRestoreCmd)
	rootCmd.AddCommand(snapshotCmd)
}

func getSnapshotterForSandbox(metadata *config.SandboxMetadata) (workspace.Snapshotter, error) {
	if metadata.WorkspaceMode == "direct" || metadata.SourceRepo == "" {
		return nil, fmt.Errorf("snapshots are not available in direct workspace mode (no VCS backend)")
	}

	backend := workspace.BackendForMode(metadata.WorkspaceMode)
	if backend == nil {
		return nil, fmt.Errorf("unknown workspace mode: %s", metadata.WorkspaceMode)
	}

	snapshotter, ok := backend.(workspace.Snapshotter)
	if !ok {
		return nil, fmt.Errorf("workspace backend %q does not support snapshots", backend.Name())
	}

	return snapshotter, nil
}

func runSnapshotCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	snapshotName := args[1]

	metadata, err := loadSandbox(name)
	if err != nil {
		return err
	}

	snapshotter, err := getSnapshotterForSandbox(metadata)
	if err != nil {
		return err
	}

	if err := snapshotter.Snapshot(metadata.SourceRepo, name, snapshotName); err != nil {
		return fmt.Errorf("failed to create snapshot: %w", err)
	}

	logSuccess("Created snapshot %q for sandbox %s", snapshotName, name)
	return nil
}

func runSnapshotList(cmd *cobra.Command, args []string) error {
	name := args[0]

	metadata, err := loadSandbox(name)
	if err != nil {
		return err
	}

	snapshotter, err := getSnapshotterForSandbox(metadata)
	if err != nil {
		return err
	}

	snapshots, err := snapshotter.ListSnapshots(metadata.SourceRepo, name)
	if err != nil {
		return fmt.Errorf("failed to list snapshots: %w", err)
	}

	if len(snapshots) == 0 {
		logInfo("No snapshots found for sandbox %s", name)
		return nil
	}

	for _, s := range snapshots {
		if s.ChangeID != "" {
			fmt.Printf("  %s  (%s)\n", s.Name, s.ChangeID)
		} else {
			fmt.Printf("  %s\n", s.Name)
		}
	}
	return nil
}

func runSnapshotRestore(cmd *cobra.Command, args []string) error {
	name := args[0]
	snapshotName := args[1]

	metadata, err := loadSandbox(name)
	if err != nil {
		return err
	}

	snapshotter, err := getSnapshotterForSandbox(metadata)
	if err != nil {
		return err
	}

	if err := snapshotter.RestoreSnapshot(metadata.SourceRepo, name, snapshotName); err != nil {
		return fmt.Errorf("failed to restore snapshot: %w", err)
	}

	logSuccess("Restored snapshot %q for sandbox %s", snapshotName, name)
	return nil
}
