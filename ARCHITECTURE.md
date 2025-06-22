# Claude Squad Architecture

This document describes the technical architecture and design patterns used in Claude Squad.

## Overview

Claude Squad is a terminal-based session manager for AI coding assistants built in Go. It uses a combination of tmux for terminal session management and git worktrees for isolated workspaces.

## Core Components

### Session Management (`session/`)
- `Instance` struct represents a running AI assistant session
- Sessions have states: Running, Ready, Loading, Paused
- Each session gets its own git worktree and tmux session

### Git Worktree Integration (`session/git/`)
- `GitWorktree` manages isolated git branches per session
- Creates branches with configurable prefixes (default: `<username>/`)
- Supports pausing/resuming by removing/restoring worktrees
- Implements rebase functionality using `git rebase --onto` pattern

### Tmux Integration (`session/tmux/`)
- `TmuxSession` wraps tmux sessions for AI assistants
- Supports multiple programs: Claude Code, Aider, Codex
- Monitors session status and handles auto-yes mode

### Terminal UI (`ui/`)
- Built with Bubble Tea framework
- Tabbed interface showing session list, preview, and diffs
- Keyboard shortcuts for session management

### Application State (`app/`)
- Main application loop and state management
- Handles user interactions and navigation
- Coordinates between UI components and session management

## Repository-Specific Caching

Claude Squad implements an intelligent caching system for repository-specific metadata that is expensive to compute but rarely changes.

### Cached Settings

The application caches the following per-repository:

- **Default Branch**: The repository's main branch (e.g., "main", "master")
  - Detected via `git remote show origin | sed -n '/HEAD branch/s/.*: //p'`
  - Cached to avoid expensive remote queries on every rebase operation

### Cache Storage

Repository-specific settings are stored in `~/.agent-farmer/repo-<hash>.json` where:
- `<hash>` is an MD5 hash of the repository's absolute path
- Each cache file contains a `RepoConfig` struct with repository metadata
- Includes timestamps for potential future cache invalidation

### Cache Management

**Automatic Population**: 
- Cache is populated lazily when first needed
- No upfront scanning or initialization required
- Settings are cached immediately after first successful detection

**Manual Reset**:
- `agent-farmer reset` - Preserves cached repository settings
- `agent-farmer reset -f` - Clears all cached settings and starts fresh

### Implementation Details

The caching system is implemented in `config/config.go`:

- `RepoConfig` struct defines the cached data structure
- `GetDefaultBranch()` implements cache-first lookup with fallback
- `LoadRepoConfig()` and `SaveRepoConfig()` handle persistence
- `DeleteAllRepoConfigs()` provides cache reset functionality

This design ensures that expensive git operations (like `git remote show origin`) are only performed once per repository, significantly improving performance for operations like rebasing that need to know the default branch.

## Key Features

- **Isolated Workspaces**: Each session runs in its own git worktree, preventing conflicts
- **Background Execution**: Sessions can run tasks in background with auto-accept mode
- **Session Persistence**: Sessions can be paused (preserving branch) and resumed later
- **Multi-Assistant Support**: Works with Claude Code, Aider, Codex and other local agents
- **Smart Rebasing**: Uses `git rebase --onto` pattern with cached default branch detection

## Configuration

- Config stored in `~/.agent-farmer/` directory
- Application state persisted in JSON format
- Repository-specific cache files for performance optimization
- Branch naming configurable via `BranchPrefix` setting
- Default program and auto-yes mode configurable

## Prerequisites

The application requires:
- tmux (for session management)
- gh (GitHub CLI, for git operations)
- Must be run from within a git repository