package devenv

import (
	"agent-farmer/config"
	"fmt"
	"os"
	"path/filepath"
)

// DevEnvironmentManager handles development environment operations
type DevEnvironmentManager struct {
	repoPath   string
	repoConfig *config.RepoConfig
}

// DevEnvironmentConfig represents development environment configuration
type DevEnvironmentConfig struct {
	Enabled           bool     `json:"enabled"`
	TiltfilePath      string   `json:"tiltfile_path"`
	DockerComposePath string   `json:"docker_compose_path"`
	Services          []string `json:"services,omitempty"`
	HostnamePattern   string   `json:"hostname_pattern"`
	Port              int      `json:"port"`
}

// NewDevEnvironmentManager creates a new development environment manager
func NewDevEnvironmentManager(repoPath string) (*DevEnvironmentManager, error) {
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	return &DevEnvironmentManager{
		repoPath: absPath,
	}, nil
}

// TiltfileExists checks if a Tiltfile exists in the expected location
func (dm *DevEnvironmentManager) TiltfileExists() bool {
	tiltfilePath := filepath.Join(dm.repoPath, ".agent-farmer", "Tiltfile")
	_, err := os.Stat(tiltfilePath)
	return err == nil
}

// GetTiltfilePath returns the path where the Tiltfile should be located
func (dm *DevEnvironmentManager) GetTiltfilePath() string {
	return filepath.Join(dm.repoPath, ".agent-farmer", "Tiltfile")
}

// GetRepoConfigPath returns the path where the repo config should be located
func (dm *DevEnvironmentManager) GetRepoConfigPath() string {
	return filepath.Join(dm.repoPath, ".agent-farmer", "repo-config.json")
}

// IsEnabled checks if development environment is enabled for this repository
func (dm *DevEnvironmentManager) IsEnabled() bool {
	// For now, we'll consider it enabled if a Tiltfile exists
	// In the future, this could check a more specific config setting
	return dm.TiltfileExists()
}
