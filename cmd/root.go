package cmd

import (
	"fmt"
	"os"

	"arbor/internal/gitgraph"
	"arbor/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
	git "github.com/go-git/go-git/v5"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "arbor",
	Short: "Visualize Git commit history as an interactive tree",
	RunE: func(cmd *cobra.Command, args []string) error {
		includeAll, _ := cmd.Flags().GetBool("all")
		limit, _ := cmd.Flags().GetInt("limit")

		repo, path, err := openRepo()
		if err != nil {
			return err
		}

		provider, err := gitgraph.NewCommitProvider(repo, includeAll, limit)
		if err != nil {
			return err
		}

		headName := headLabel(repo)
		model := tui.NewModel(path, provider, headName)
		program := tea.NewProgram(model, tea.WithAltScreen())
		_, err = program.Run()
		return err
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().Bool("all", false, "include all local and remote branches")
	rootCmd.Flags().Int("limit", 0, "limit the number of commits to parse (0 = no limit)")
}

func openRepo() (*git.Repository, string, error) {
	repo, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, "", fmt.Errorf("open git repository: %w", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return repo, "", nil
	}
	return repo, wt.Filesystem.Root(), nil
}

func headLabel(repo *git.Repository) string {
	head, err := repo.Head()
	if err != nil {
		return ""
	}
	if head.Name().IsBranch() {
		return head.Name().Short()
	}
	hash := head.Hash()
	if hash.IsZero() {
		return "detached"
	}
	return fmt.Sprintf("detached@%s", hash.String()[:7])
}
