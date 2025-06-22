package git

import (
	"agent-farmer/config"
	"agent-farmer/log"
	"fmt"
	"os/exec"
	"strings"
)

// runGitCommand executes a git command and returns any error
func (g *GitWorktree) runGitCommand(path string, args ...string) (string, error) {
	baseArgs := []string{"-C", path}
	cmd := exec.Command("git", append(baseArgs, args...)...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git command failed: %s (%w)", output, err)
	}

	return string(output), nil
}

// PushChanges commits and pushes changes in the worktree to the remote branch
func (g *GitWorktree) PushChanges(commitMessage string, open bool) error {
	if err := checkGHCLI(); err != nil {
		return err
	}

	// Check if there are any changes to commit
	isDirty, err := g.IsDirty()
	if err != nil {
		return fmt.Errorf("failed to check for changes: %w", err)
	}

	if isDirty {
		// Stage all changes
		if _, err := g.runGitCommand(g.worktreePath, "add", "."); err != nil {
			log.ErrorLog.Print(err)
			return fmt.Errorf("failed to stage changes: %w", err)
		}

		// Create commit
		if _, err := g.runGitCommand(g.worktreePath, "commit", "-m", commitMessage, "--no-verify"); err != nil {
			log.ErrorLog.Print(err)
			return fmt.Errorf("failed to commit changes: %w", err)
		}
	}

	// First push the branch to remote to ensure it exists
	pushCmd := exec.Command("gh", "repo", "sync", "--source", "-b", g.branchName)
	pushCmd.Dir = g.worktreePath
	if err := pushCmd.Run(); err != nil {
		// If sync fails, try creating the branch on remote first
		gitPushCmd := exec.Command("git", "push", "-u", "origin", g.branchName)
		gitPushCmd.Dir = g.worktreePath
		if pushOutput, pushErr := gitPushCmd.CombinedOutput(); pushErr != nil {
			log.ErrorLog.Print(pushErr)
			return fmt.Errorf("failed to push branch: %s (%w)", pushOutput, pushErr)
		}
	}

	// Now sync with remote
	syncCmd := exec.Command("gh", "repo", "sync", "-b", g.branchName)
	syncCmd.Dir = g.worktreePath
	if output, err := syncCmd.CombinedOutput(); err != nil {
		log.ErrorLog.Print(err)
		return fmt.Errorf("failed to sync changes: %s (%w)", output, err)
	}

	// Open the branch in the browser
	if open {
		if err := g.OpenBranchURL(); err != nil {
			// Just log the error but don't fail the push operation
			log.ErrorLog.Printf("failed to open branch URL: %v", err)
		}
	}

	return nil
}

// IsDirty checks if the worktree has uncommitted changes
func (g *GitWorktree) IsDirty() (bool, error) {
	output, err := g.runGitCommand(g.worktreePath, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("failed to check worktree status: %w", err)
	}
	return len(output) > 0, nil
}

// IsBranchCheckedOut checks if the instance branch is currently checked out
func (g *GitWorktree) IsBranchCheckedOut() (bool, error) {
	output, err := g.runGitCommand(g.repoPath, "branch", "--show-current")
	if err != nil {
		return false, fmt.Errorf("failed to get current branch: %w", err)
	}
	return strings.TrimSpace(string(output)) == g.branchName, nil
}

// OpenBranchURL opens the branch URL in the default browser
func (g *GitWorktree) OpenBranchURL() error {
	// Check if GitHub CLI is available
	if err := checkGHCLI(); err != nil {
		return err
	}

	cmd := exec.Command("gh", "browse", "--branch", g.branchName)
	cmd.Dir = g.worktreePath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to open branch URL: %w", err)
	}
	return nil
}

// RebaseOntoDefault rebases the current branch onto the default branch using git rebase --onto
func (g *GitWorktree) RebaseOntoDefault() error {
	// Get the default branch for this repository
	defaultBranch, err := config.GetDefaultBranch(g.repoPath)
	if err != nil {
		return fmt.Errorf("failed to get default branch: %w", err)
	}

	log.InfoLog.Printf("rebasing branch %s onto default branch %s", g.branchName, defaultBranch)

	// Check if there are any uncommitted changes
	isDirty, err := g.IsDirty()
	if err != nil {
		return fmt.Errorf("failed to check for uncommitted changes: %w", err)
	}

	if isDirty {
		return fmt.Errorf("cannot rebase with uncommitted changes - please commit or stash your changes first")
	}

	// Ensure we have the latest changes from the default branch
	// First fetch the latest changes
	if _, err := g.runGitCommand(g.worktreePath, "fetch", "origin", defaultBranch); err != nil {
		return fmt.Errorf("failed to fetch latest changes: %w", err)
	}

	// Get the current branch name
	currentBranch, err := g.runGitCommand(g.worktreePath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}
	currentBranch = strings.TrimSpace(currentBranch)

	// Get the merge-base fork-point
	forkPoint, err := g.runGitCommand(g.worktreePath, "merge-base", "--fork-point", "origin/"+defaultBranch)
	if err != nil {
		// If fork-point fails, use regular merge-base as fallback
		log.WarningLog.Printf("merge-base --fork-point failed, falling back to regular merge-base: %v", err)
		forkPoint, err = g.runGitCommand(g.worktreePath, "merge-base", "origin/"+defaultBranch, currentBranch)
		if err != nil {
			return fmt.Errorf("failed to find merge-base: %w", err)
		}
	}
	forkPoint = strings.TrimSpace(forkPoint)

	// Perform the rebase using --onto
	// This is equivalent to: git rebase --onto origin/main $(git merge-base --fork-point origin/main) HEAD
	if _, err := g.runGitCommand(g.worktreePath, "rebase", "--onto", "origin/"+defaultBranch, forkPoint, currentBranch); err != nil {
		// If rebase fails, we should abort it to leave the repo in a clean state
		if abortErr := g.abortRebase(); abortErr != nil {
			log.ErrorLog.Printf("failed to abort rebase after failure: %v", abortErr)
		}
		return fmt.Errorf("rebase failed: %w", err)
	}

	log.InfoLog.Printf("successfully rebased %s onto %s", currentBranch, defaultBranch)
	return nil
}

// abortRebase aborts an ongoing rebase operation
func (g *GitWorktree) abortRebase() error {
	_, err := g.runGitCommand(g.worktreePath, "rebase", "--abort")
	return err
}
