package config

import (
	"agent-farmer/log"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	ConfigFileName     = "config.json"
	RepoConfigFileName = "repo-config.json"
	defaultProgram     = "claude"
)

// GetConfigDir returns the path to the application's configuration directory
func GetConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get config home directory: %w", err)
	}
	return filepath.Join(homeDir, ".agent-farmer"), nil
}

// GetRepoConfigDir returns the path to the repository-local configuration directory
func GetRepoConfigDir(repoPath string) (string, error) {
	if repoPath == "" {
		return "", fmt.Errorf("repository path cannot be empty")
	}

	// Clean the path to ensure it's absolute
	absRepoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path for repo: %w", err)
	}

	return filepath.Join(absRepoPath, ".agent-farmer"), nil
}

// Config represents the application configuration
type Config struct {
	// DefaultProgram is the default program to run in new instances
	DefaultProgram string `json:"default_program"`
	// AutoYes is a flag to automatically accept all prompts.
	AutoYes bool `json:"auto_yes"`
	// DaemonPollInterval is the interval (ms) at which the daemon polls sessions for autoyes mode.
	DaemonPollInterval int `json:"daemon_poll_interval"`
	// BranchPrefix is the prefix used for git branches created by the application.
	BranchPrefix string `json:"branch_prefix"`
}

// RepoConfig represents repository-specific cached settings
type RepoConfig struct {
	// RepoPath is the absolute path to the repository root
	RepoPath string `json:"repo_path"`
	// DefaultBranch is the cached default branch name (e.g., "main", "master")
	DefaultBranch string `json:"default_branch"`
	// LastUpdated is a timestamp of when this cache was last updated
	LastUpdated int64 `json:"last_updated"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	program, err := GetClaudeCommand()
	if err != nil {
		log.ErrorLog.Printf("failed to get claude command: %v", err)
		program = defaultProgram
	}

	return &Config{
		DefaultProgram:     program,
		AutoYes:            false,
		DaemonPollInterval: 1000,
		BranchPrefix: func() string {
			user, err := user.Current()
			if err != nil || user == nil || user.Username == "" {
				log.ErrorLog.Printf("failed to get current user: %v", err)
				return "session/"
			}
			return fmt.Sprintf("%s/", strings.ToLower(user.Username))
		}(),
	}
}

// GetClaudeCommand attempts to find the "claude" command in the user's shell
// It checks in the following order:
// 1. Shell alias resolution: using "which" command
// 2. PATH lookup
//
// If both fail, it returns an error.
func GetClaudeCommand() (string, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash" // Default to bash if SHELL is not set
	}

	// Force the shell to load the user's profile and then run the command
	// For zsh, source .zshrc; for bash, source .bashrc
	var shellCmd string
	if strings.Contains(shell, "zsh") {
		shellCmd = "source ~/.zshrc 2>/dev/null || true; which claude"
	} else if strings.Contains(shell, "bash") {
		shellCmd = "source ~/.bashrc 2>/dev/null || true; which claude"
	} else {
		shellCmd = "which claude"
	}

	cmd := exec.Command(shell, "-c", shellCmd)
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		path := strings.TrimSpace(string(output))
		if path != "" {
			// Check if the output is an alias definition and extract the actual path
			// Handle formats like "claude: aliased to /path/to/claude" or other shell-specific formats
			aliasRegex := regexp.MustCompile(`(?:aliased to|->|=)\s*([^\s]+)`)
			matches := aliasRegex.FindStringSubmatch(path)
			if len(matches) > 1 {
				path = matches[1]
			}
			return path, nil
		}
	}

	// Otherwise, try to find in PATH directly
	claudePath, err := exec.LookPath("claude")
	if err == nil {
		return claudePath, nil
	}

	return "", fmt.Errorf("claude command not found in aliases or PATH")
}

func LoadConfig() *Config {
	configDir, err := GetConfigDir()
	if err != nil {
		log.ErrorLog.Printf("failed to get config directory: %v", err)
		return DefaultConfig()
	}

	configPath := filepath.Join(configDir, ConfigFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create and save default config if file doesn't exist
			defaultCfg := DefaultConfig()
			if saveErr := saveConfig(defaultCfg); saveErr != nil {
				log.WarningLog.Printf("failed to save default config: %v", saveErr)
			}
			return defaultCfg
		}

		log.WarningLog.Printf("failed to get config file: %v", err)
		return DefaultConfig()
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		log.ErrorLog.Printf("failed to parse config file: %v", err)
		return DefaultConfig()
	}

	return &config
}

// saveConfig saves the configuration to disk
func saveConfig(config *Config) error {
	configDir, err := GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, ConfigFileName)
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(configPath, data, 0644)
}

// SaveConfig exports the saveConfig function for use by other packages
func SaveConfig(config *Config) error {
	return saveConfig(config)
}

// getRepoConfigPath returns the path to the repo-specific config file
func getRepoConfigPath(repoPath string) (string, error) {
	repoConfigDir, err := GetRepoConfigDir(repoPath)
	if err != nil {
		return "", fmt.Errorf("failed to get repo config directory: %w", err)
	}

	return filepath.Join(repoConfigDir, RepoConfigFileName), nil
}

// getLegacyRepoConfigPath returns the path to the legacy repo-specific config file (for migration)
func getLegacyRepoConfigPath(repoPath string) (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get config directory: %w", err)
	}

	// Create a hash of the repo path to use as filename (legacy format)
	hash := md5.Sum([]byte(repoPath))
	filename := fmt.Sprintf("repo-%x.json", hash)

	return filepath.Join(configDir, filename), nil
}

// LoadRepoConfig loads the repository-specific configuration
func LoadRepoConfig(repoPath string) (*RepoConfig, error) {
	// First try the new repo-local location
	configPath, err := getRepoConfigPath(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get repo config path: %w", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Try the legacy location for backward compatibility
			legacyPath, legacyErr := getLegacyRepoConfigPath(repoPath)
			if legacyErr != nil {
				return nil, nil // No config found, return nil
			}

			legacyData, legacyReadErr := os.ReadFile(legacyPath)
			if legacyReadErr != nil {
				if os.IsNotExist(legacyReadErr) {
					return nil, nil // No config found in either location
				}
				return nil, fmt.Errorf("failed to read legacy repo config file: %w", legacyReadErr)
			}

			// Parse legacy config
			var config RepoConfig
			if err := json.Unmarshal(legacyData, &config); err != nil {
				return nil, fmt.Errorf("failed to parse legacy repo config file: %w", err)
			}

			// Migrate to new location
			log.DebugLog.Printf("migrating repo config from legacy location: %s -> %s", legacyPath, configPath)
			if migrateErr := SaveRepoConfig(&config); migrateErr != nil {
				log.WarningLog.Printf("failed to migrate repo config to new location: %v", migrateErr)
			} else {
				// Remove legacy file after successful migration
				if removeErr := os.Remove(legacyPath); removeErr != nil {
					log.WarningLog.Printf("failed to remove legacy repo config file: %v", removeErr)
				}
			}

			return &config, nil
		}
		return nil, fmt.Errorf("failed to read repo config file: %w", err)
	}

	var config RepoConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse repo config file: %w", err)
	}

	return &config, nil
}

// SaveRepoConfig saves the repository-specific configuration
func SaveRepoConfig(config *RepoConfig) error {
	configPath, err := getRepoConfigPath(config.RepoPath)
	if err != nil {
		return fmt.Errorf("failed to get repo config path: %w", err)
	}

	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Update the timestamp
	config.LastUpdated = time.Now().Unix()

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal repo config: %w", err)
	}

	return os.WriteFile(configPath, data, 0644)
}

// DeleteRepoConfig deletes the repository-specific configuration
func DeleteRepoConfig(repoPath string) error {
	// Delete from new location
	configPath, err := getRepoConfigPath(repoPath)
	if err != nil {
		return fmt.Errorf("failed to get repo config path: %w", err)
	}

	err = os.Remove(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete repo config file: %w", err)
	}

	// Also try to delete from legacy location if it exists
	legacyPath, legacyErr := getLegacyRepoConfigPath(repoPath)
	if legacyErr == nil {
		legacyRemoveErr := os.Remove(legacyPath)
		if legacyRemoveErr != nil && !os.IsNotExist(legacyRemoveErr) {
			log.WarningLog.Printf("failed to delete legacy repo config file: %v", legacyRemoveErr)
		}
	}

	return nil
}

// DeleteAllRepoConfigs deletes all repository-specific configurations
func DeleteAllRepoConfigs() error {
	configDir, err := GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	pattern := filepath.Join(configDir, "repo-*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to find repo config files: %w", err)
	}

	for _, match := range matches {
		if err := os.Remove(match); err != nil && !os.IsNotExist(err) {
			log.WarningLog.Printf("failed to delete repo config file %s: %v", match, err)
		}
	}

	return nil
}

// GetDefaultBranch returns the default branch for the given repository, with caching
func GetDefaultBranch(repoPath string) (string, error) {
	// First, try to load from cache
	repoConfig, err := LoadRepoConfig(repoPath)
	if err != nil {
		log.WarningLog.Printf("failed to load repo config: %v", err)
	}

	// If we have a cached value, return it
	if repoConfig != nil && repoConfig.DefaultBranch != "" {
		log.DebugLog.Printf("using cached default branch: %s", repoConfig.DefaultBranch)
		return repoConfig.DefaultBranch, nil
	}

	// Otherwise, fetch it from git
	log.DebugLog.Printf("fetching default branch from git for repo: %s", repoPath)
	cmd := exec.Command("git", "remote", "show", "origin")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get default branch: %w", err)
	}

	// Parse the output to extract the default branch
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "HEAD branch:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				defaultBranch := strings.TrimSpace(parts[1])

				// Cache the result
				newRepoConfig := &RepoConfig{
					RepoPath:      repoPath,
					DefaultBranch: defaultBranch,
				}
				if saveErr := SaveRepoConfig(newRepoConfig); saveErr != nil {
					log.WarningLog.Printf("failed to cache default branch: %v", saveErr)
				}

				log.DebugLog.Printf("cached default branch: %s", defaultBranch)
				return defaultBranch, nil
			}
		}
	}

	return "", fmt.Errorf("could not determine default branch from git remote show origin")
}
