package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/slashobserve/slashobserve/internal/agent/prompts/skill"
)

const skillFileName = "SKILL.md"

func (a *app) skillCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Install the /observe skill for your agents",
		Args:  groupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(a.skillInstallCommand())
	return cmd
}

func (a *app) skillInstallCommand() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:     "install",
		Short:   "Write the /observe skill where your agents read skills",
		Long:    "install writes the /observe skill, the guide outside agents follow to work in SlashObserve, to ~/.agents/skills/observe/SKILL.md and ~/.claude/skills/observe/SKILL.md, replacing an earlier copy.\nThe skill is part of this program and is never fetched from a server, so run install again after you upgrade observe.",
		Example: "  observe skill install\n  observe skill install --dir ./skills",
		Args:    noArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			roots, err := skillRoots(dir)
			if err != nil {
				return err
			}
			for _, root := range roots {
				path, err := writeSkill(root)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintf(a.std.Out, "Installed %s\n", path)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "write <path>/observe/SKILL.md instead of the default agent skill directories")
	return cmd
}

func skillRoots(dir string) ([]string, error) {
	if dir != "" {
		return []string{dir}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, errors.New("cannot find your home directory; pass --dir")
	}
	return []string{filepath.Join(home, ".agents", "skills"), filepath.Join(home, ".claude", "skills")}, nil
}

func writeSkill(root string) (string, error) {
	folder := filepath.Join(root, skill.Name)
	path := filepath.Join(folder, skillFileName)
	if err := os.MkdirAll(folder, 0o755); err != nil { //nolint:gosec // skill folders are read by the person's agents, like any directory under ~/.claude
		return "", fmt.Errorf("cannot create %s: %w", folder, err)
	}
	if err := os.WriteFile(path, skill.File(), 0o644); err != nil { //nolint:gosec // a skill is text the person's agents read, not a secret
		return "", fmt.Errorf("cannot write %s: %w", path, err)
	}
	return path, nil
}
