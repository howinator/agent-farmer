package git

import (
	"agent-farmer/config"
	"agent-farmer/log"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// gitMutex prevents concurrent git operations that might cause index.lock conflicts
var gitMutex sync.Mutex

// runGitCommand executes a git command and returns any error
func (g *GitWorktree) runGitCommand(path string, args ...string) (string, error) {
	baseArgs := []string{"-C", path}
	fullArgs := append(baseArgs, args...)
	cmd := exec.Command("git", fullArgs...)

	// Log the command being executed for debugging
	log.DebugLog.Printf("executing git command: git %s", strings.Join(fullArgs, " "))

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.ErrorLog.Printf("git command failed: git %s\nOutput: %s\nError: %v", strings.Join(fullArgs, " "), output, err)
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
	// Use mutex to prevent concurrent git operations
	gitMutex.Lock()
	defer gitMutex.Unlock()

	log.DebugLog.Printf("starting rebase operation for worktree: %s", g.worktreePath)
	log.DebugLog.Printf("rebase working on branch: %s", g.branchName)
	log.DebugLog.Printf("repository path: %s", g.repoPath)

	// Get the default branch for this repository
	defaultBranch, err := config.GetDefaultBranch(g.repoPath)
	if err != nil {
		log.ErrorLog.Printf("failed to get default branch for %s: %v", g.repoPath, err)
		return fmt.Errorf("failed to get default branch: %w", err)
	}

	log.InfoLog.Printf("rebasing branch %s onto default branch %s", g.branchName, defaultBranch)

	// Check if there are any uncommitted changes
	log.DebugLog.Printf("checking for uncommitted changes...")
	isDirty, err := g.IsDirty()
	if err != nil {
		log.ErrorLog.Printf("failed to check dirty status: %v", err)
		return fmt.Errorf("failed to check for uncommitted changes: %w", err)
	}

	if isDirty {
		log.ErrorLog.Printf("cannot rebase with uncommitted changes")
		return fmt.Errorf("cannot rebase with uncommitted changes - please commit or stash your changes first")
	}
	log.DebugLog.Printf("worktree is clean, proceeding with rebase...")

	// Ensure we have the latest changes from the default branch
	// First fetch the latest changes
	log.DebugLog.Printf("fetching latest changes from origin/%s...", defaultBranch)
	if _, err := g.runGitCommand(g.worktreePath, "fetch", "origin", defaultBranch); err != nil {
		log.ErrorLog.Printf("failed to fetch changes: %v", err)
		return fmt.Errorf("failed to fetch latest changes: %w", err)
	}

	// Get the current branch name
	log.DebugLog.Printf("getting current branch name...")
	currentBranch, err := g.runGitCommand(g.worktreePath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		log.ErrorLog.Printf("failed to get current branch: %v", err)
		return fmt.Errorf("failed to get current branch: %w", err)
	}
	currentBranch = strings.TrimSpace(currentBranch)
	log.DebugLog.Printf("current branch: %s", currentBranch)

	// Get the merge-base fork-point
	log.DebugLog.Printf("finding merge-base fork-point...")
	forkPoint, err := g.runGitCommand(g.worktreePath, "merge-base", "--fork-point", "origin/"+defaultBranch)
	if err != nil {
		// If fork-point fails, use regular merge-base as fallback
		log.WarningLog.Printf("merge-base --fork-point failed, falling back to regular merge-base: %v", err)
		forkPoint, err = g.runGitCommand(g.worktreePath, "merge-base", "origin/"+defaultBranch, currentBranch)
		if err != nil {
			log.ErrorLog.Printf("failed to find merge-base: %v", err)
			return fmt.Errorf("failed to find merge-base: %w", err)
		}
	}
	forkPoint = strings.TrimSpace(forkPoint)
	log.DebugLog.Printf("fork point: %s", forkPoint)

	// Perform the rebase using --onto
	// This is equivalent to: git rebase --onto origin/main $(git merge-base --fork-point origin/main) HEAD
	log.DebugLog.Printf("executing rebase: git rebase --onto origin/%s %s %s", defaultBranch, forkPoint, currentBranch)
	if _, err := g.runGitCommand(g.worktreePath, "rebase", "--onto", "origin/"+defaultBranch, forkPoint, currentBranch); err != nil {
		log.ErrorLog.Printf("rebase command failed: %v", err)
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
