package devenv

import (
	"agent-farmer/log"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RepositoryContext holds information about the repository structure
type RepositoryContext struct {
	ReadmeContent      string
	DockerFiles        []string
	DockerComposeFiles []string
	PackageFiles       []string
	ServiceFiles       []string
	RepoPath           string
}

// GeneratePromptForTiltfile creates a prompt for an LLM agent to generate a Tiltfile
// This prompt references files by name rather than including their contents
func GeneratePromptForTiltfile(repoPath string) (string, error) {
	ctx, err := analyzeRepository(repoPath)
	if err != nil {
		return "", fmt.Errorf("failed to analyze repository: %w", err)
	}

	prompt := `You are tasked with creating a development environment configuration for this project using Tilt. 

Based on the repository structure, please create a Tiltfile that will:
1. Set up the development environment with necessary services
2. Run the main application with hot reloading when possible
3. Configure any dependencies (databases, caches, etc.) using docker-compose if available
4. Use appropriate ports and expose them properly
5. Set up environment variables for development

Repository analysis:
`

	if len(ctx.PackageFiles) > 0 {
		prompt += fmt.Sprintf("- Package files found: %s\n", strings.Join(ctx.PackageFiles, ", "))
	}

	if len(ctx.DockerComposeFiles) > 0 {
		prompt += fmt.Sprintf("- Docker Compose files: %s\n", strings.Join(ctx.DockerComposeFiles, ", "))
	}

	if len(ctx.DockerFiles) > 0 {
		prompt += fmt.Sprintf("- Dockerfiles: %s\n", strings.Join(ctx.DockerFiles, ", "))
	}

	if ctx.ReadmeContent != "" {
		prompt += "- README.md file exists (check for setup instructions)\n"
	}

	prompt += `
Please examine these files to understand the project structure and dependencies. Then create a Tiltfile that:

1. Uses local_resource() for running the main application on the host (better for development)
2. Uses docker_compose() for dependencies like databases, redis, etc.
3. Sets up proper port forwarding
4. Includes resource dependencies where needed
5. Uses environment variables like TASK_NAME, TASK_HOSTNAME, TASK_PORT for task-specific configuration

The Tiltfile should be saved as .agent-farmer/Tiltfile in the repository root.

Focus on creating a working development environment that a developer can use immediately after the AI agent completes their work.`

	return prompt, nil
}

// analyzeRepository scans the repository to understand its structure
func analyzeRepository(repoPath string) (*RepositoryContext, error) {
	ctx := &RepositoryContext{
		RepoPath:           repoPath,
		DockerFiles:        []string{},
		DockerComposeFiles: []string{},
		PackageFiles:       []string{},
		ServiceFiles:       []string{},
	}

	// Check for README
	readmePath := filepath.Join(repoPath, "README.md")
	if _, err := os.Stat(readmePath); err == nil {
		content, err := os.ReadFile(readmePath)
		if err == nil {
			ctx.ReadmeContent = string(content)
		}
	}

	// Walk the repository to find relevant files
	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories except .agent-farmer
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") && info.Name() != ".agent-farmer" {
			return filepath.SkipDir
		}

		// Skip node_modules and other large directories
		if info.IsDir() && (info.Name() == "node_modules" || info.Name() == "vendor" || info.Name() == "target") {
			return filepath.SkipDir
		}

		if info.IsDir() {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(repoPath, path)
		if err != nil {
			log.WarningLog.Printf("failed to get relative path for %s: %v", path, err)
			return nil
		}

		fileName := info.Name()

		// Detect package files
		switch fileName {
		case "package.json", "go.mod", "Cargo.toml", "requirements.txt", "pyproject.toml", "composer.json", "Gemfile", "pom.xml", "build.gradle":
			ctx.PackageFiles = append(ctx.PackageFiles, relPath)
		case "Dockerfile", "dockerfile":
			ctx.DockerFiles = append(ctx.DockerFiles, relPath)
		case "docker-compose.yml", "docker-compose.yaml":
			ctx.DockerComposeFiles = append(ctx.DockerComposeFiles, relPath)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk repository: %w", err)
	}

	return ctx, nil
}

// EnsureAgentFarmerDir creates the .agent-farmer directory if it doesn't exist
func EnsureAgentFarmerDir(repoPath string) error {
	agentFarmerDir := filepath.Join(repoPath, ".agent-farmer")
	return os.MkdirAll(agentFarmerDir, 0755)
}
