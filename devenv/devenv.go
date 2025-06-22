package devenv

import (
	"agent-farmer/config"
	"agent-farmer/log"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// DevEnvironmentManager manages development environments for agent sessions
type DevEnvironmentManager struct {
	repoPath   string
	repoConfig *config.RepoConfig
}

// NewDevEnvironmentManager creates a new development environment manager
func NewDevEnvironmentManager(repoPath string) (*DevEnvironmentManager, error) {
	absRepoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for repo: %w", err)
	}

	repoConfig, err := config.LoadRepoConfig(absRepoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load repo config: %w", err)
	}

	// Initialize with default dev environment config if none exists
	if repoConfig == nil {
		repoConfig = &config.RepoConfig{
			RepoPath: absRepoPath,
		}
	}

	if repoConfig.DevEnvironment == nil {
		repoConfig.DevEnvironment = &config.DevEnvironmentConfig{
			Enabled:           false,
			TiltfilePath:      ".agent-farmer/Tiltfile",
			DockerComposePath: "docker-compose.yml",
			HostnamePattern:   "{task}.agent",
			Port:              3000,
		}
	}

	return &DevEnvironmentManager{
		repoPath:   absRepoPath,
		repoConfig: repoConfig,
	}, nil
}

// IsEnabled returns whether the development environment is enabled for this repository
func (d *DevEnvironmentManager) IsEnabled() bool {
	return d.repoConfig.DevEnvironment != nil && d.repoConfig.DevEnvironment.Enabled
}

// Enable enables the development environment for this repository
func (d *DevEnvironmentManager) Enable() error {
	if d.repoConfig.DevEnvironment == nil {
		d.repoConfig.DevEnvironment = &config.DevEnvironmentConfig{}
	}
	d.repoConfig.DevEnvironment.Enabled = true
	return config.SaveRepoConfig(d.repoConfig)
}

// Disable disables the development environment for this repository
func (d *DevEnvironmentManager) Disable() error {
	if d.repoConfig.DevEnvironment == nil {
		return nil
	}
	d.repoConfig.DevEnvironment.Enabled = false
	return config.SaveRepoConfig(d.repoConfig)
}

// AutoDetectConfiguration attempts to automatically detect development environment configuration
func (d *DevEnvironmentManager) AutoDetectConfiguration() error {
	if d.repoConfig.DevEnvironment == nil {
		d.repoConfig.DevEnvironment = &config.DevEnvironmentConfig{
			HostnamePattern: "{task}.agent",
			Port:            3000,
		}
	}

	// Look for Tiltfile, prioritizing .agent-farmer directory
	tiltfilePaths := []string{".agent-farmer/Tiltfile", "Tiltfile", "tilt/Tiltfile", ".tilt/Tiltfile"}
	tiltfileFound := false
	for _, path := range tiltfilePaths {
		fullPath := filepath.Join(d.repoPath, path)
		if _, err := os.Stat(fullPath); err == nil {
			d.repoConfig.DevEnvironment.TiltfilePath = path
			log.DebugLog.Printf("found Tiltfile at: %s", path)
			tiltfileFound = true
			break
		}
	}
	
	// If no Tiltfile found, set default to .agent-farmer location
	if !tiltfileFound {
		d.repoConfig.DevEnvironment.TiltfilePath = ".agent-farmer/Tiltfile"
		log.DebugLog.Printf("no Tiltfile found, will use default: .agent-farmer/Tiltfile")
	}

	// Look for docker-compose files
	composePaths := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
		"docker/docker-compose.yml",
		"docker/docker-compose.yaml",
	}
	for _, path := range composePaths {
		fullPath := filepath.Join(d.repoPath, path)
		if _, err := os.Stat(fullPath); err == nil {
			d.repoConfig.DevEnvironment.DockerComposePath = path
			log.DebugLog.Printf("found docker-compose file at: %s", path)
			break
		}
	}

	return config.SaveRepoConfig(d.repoConfig)
}

// StartEnvironment starts the development environment for a specific task
func (d *DevEnvironmentManager) StartEnvironment(taskName string) error {
	if !d.IsEnabled() {
		return fmt.Errorf("development environment is not enabled for this repository")
	}

	log.DebugLog.Printf("starting development environment for task: %s", taskName)

	// Check if Tilt is available
	if _, err := exec.LookPath("tilt"); err != nil {
		return fmt.Errorf("tilt command not found - please install Tilt (https://docs.tilt.dev/install.html)")
	}

	// Generate hostname for this task
	hostname := d.generateHostname(taskName)
	log.DebugLog.Printf("generated hostname: %s", hostname)

	// Check if Tiltfile exists
	tiltfilePath := filepath.Join(d.repoPath, d.repoConfig.DevEnvironment.TiltfilePath)
	if _, err := os.Stat(tiltfilePath); os.IsNotExist(err) {
		return fmt.Errorf("Tiltfile not found at: %s", tiltfilePath)
	}

	// Start Tilt in the background
	args := []string{"up"}
	if len(d.repoConfig.DevEnvironment.Services) > 0 {
		// If specific services are configured, only start those
		args = append(args, d.repoConfig.DevEnvironment.Services...)
	}

	cmd := exec.Command("tilt", args...)
	cmd.Dir = d.repoPath
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("TASK_NAME=%s", taskName),
		fmt.Sprintf("TASK_HOSTNAME=%s", hostname),
		fmt.Sprintf("TASK_PORT=%d", d.repoConfig.DevEnvironment.Port),
	)

	// Start Tilt in detached mode
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Tilt: %w", err)
	}

	log.DebugLog.Printf("started Tilt with PID: %d", cmd.Process.Pid)

	// Give Tilt a moment to start up
	time.Sleep(2 * time.Second)

	// Check if Tilt is running
	statusCmd := exec.Command("tilt", "status")
	statusCmd.Dir = d.repoPath
	if err := statusCmd.Run(); err != nil {
		log.WarningLog.Printf("Tilt may not have started successfully: %v", err)
	}

	log.DebugLog.Printf("development environment started for task %s at %s", taskName, hostname)
	return nil
}

// StopEnvironment stops the development environment
func (d *DevEnvironmentManager) StopEnvironment() error {
	if !d.IsEnabled() {
		return nil // Nothing to stop
	}

	log.DebugLog.Printf("stopping development environment")

	// Stop Tilt
	cmd := exec.Command("tilt", "down")
	cmd.Dir = d.repoPath

	if err := cmd.Run(); err != nil {
		log.WarningLog.Printf("failed to stop Tilt cleanly: %v", err)
		// Continue with cleanup even if Tilt didn't stop cleanly
	}

	log.DebugLog.Printf("development environment stopped")
	return nil
}

// GetStatus returns the status of the development environment
func (d *DevEnvironmentManager) GetStatus() (string, error) {
	if !d.IsEnabled() {
		return "disabled", nil
	}

	// Check if Tilt is running
	cmd := exec.Command("tilt", "status", "--output", "json")
	cmd.Dir = d.repoPath

	output, err := cmd.Output()
	if err != nil {
		return "stopped", nil
	}

	// Simple check - if we got output, Tilt is likely running
	if len(output) > 0 {
		return "running", nil
	}

	return "stopped", nil
}

// generateHostname generates a hostname for the given task name
func (d *DevEnvironmentManager) generateHostname(taskName string) string {
	hostname := d.repoConfig.DevEnvironment.HostnamePattern

	// Clean the task name to be hostname-friendly
	cleanTaskName := d.cleanTaskName(taskName)

	// Replace placeholders
	hostname = strings.ReplaceAll(hostname, "{task}", cleanTaskName)

	return hostname
}

// cleanTaskName cleans a task name to be suitable for use in hostnames
func (d *DevEnvironmentManager) cleanTaskName(taskName string) string {
	// Convert to lowercase
	cleaned := strings.ToLower(taskName)

	// Replace spaces and underscores with hyphens
	cleaned = strings.ReplaceAll(cleaned, " ", "-")
	cleaned = strings.ReplaceAll(cleaned, "_", "-")

	// Remove any characters that aren't alphanumeric or hyphens
	reg := regexp.MustCompile(`[^a-z0-9-]`)
	cleaned = reg.ReplaceAllString(cleaned, "")

	// Remove multiple consecutive hyphens
	reg = regexp.MustCompile(`-+`)
	cleaned = reg.ReplaceAllString(cleaned, "-")

	// Trim hyphens from start and end
	cleaned = strings.Trim(cleaned, "-")

	// Ensure it's not empty
	if cleaned == "" {
		cleaned = "task"
	}

	// Ensure it's not too long (hostnames have limits)
	if len(cleaned) > 32 {
		cleaned = cleaned[:32]
		cleaned = strings.TrimRight(cleaned, "-")
	}

	return cleaned
}

// CreateDefaultTiltfile creates a basic Tiltfile if one doesn't exist
func (d *DevEnvironmentManager) CreateDefaultTiltfile() error {
	tiltfilePath := filepath.Join(d.repoPath, d.repoConfig.DevEnvironment.TiltfilePath)

	// Check if Tiltfile already exists
	if _, err := os.Stat(tiltfilePath); err == nil {
		return fmt.Errorf("Tiltfile already exists at: %s", tiltfilePath)
	}

	// Create a basic Tiltfile template with relative path to docker-compose
	dockerComposePath := d.repoConfig.DevEnvironment.DockerComposePath
	// If the docker-compose file is in the repo root, we need to go up one level from .agent-farmer
	if !strings.Contains(dockerComposePath, "/") {
		dockerComposePath = "../" + dockerComposePath
	}
	
	tiltfileContent := `# Tiltfile for agent-farmer development environment
# This file was auto-generated by agent-farmer

# Load the docker-compose file
docker_compose('` + dockerComposePath + `')

# Set up port forwarding
# You can customize this based on your services
# Example:
# k8s_resource('your-service', port_forwards='3000:3000')

# Environment variables are available:
# TASK_NAME - the name of the current agent task
# TASK_HOSTNAME - the generated hostname for this task
# TASK_PORT - the base port for this task

print("Starting development environment for task: " + os.getenv('TASK_NAME', 'unknown'))
print("Hostname: " + os.getenv('TASK_HOSTNAME', 'unknown'))
print("Port: " + os.getenv('TASK_PORT', '3000'))
`

	// Ensure the directory exists
	dir := filepath.Dir(tiltfilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory for Tiltfile: %w", err)
	}

	// Write the Tiltfile
	if err := os.WriteFile(tiltfilePath, []byte(tiltfileContent), 0644); err != nil {
		return fmt.Errorf("failed to create Tiltfile: %w", err)
	}

	log.DebugLog.Printf("created default Tiltfile at: %s", tiltfilePath)
	return nil
}

// GetConfiguration returns the current configuration
func (d *DevEnvironmentManager) GetConfiguration() *config.DevEnvironmentConfig {
	if d.repoConfig.DevEnvironment == nil {
		return nil
	}
	return d.repoConfig.DevEnvironment
}

// UpdateConfiguration updates the development environment configuration
func (d *DevEnvironmentManager) UpdateConfiguration(devConfig *config.DevEnvironmentConfig) error {
	d.repoConfig.DevEnvironment = devConfig
	return config.SaveRepoConfig(d.repoConfig)
}
