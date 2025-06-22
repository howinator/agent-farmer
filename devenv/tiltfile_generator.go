package devenv

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TiltfileGeneratorConfig holds configuration for Tiltfile generation
type TiltfileGeneratorConfig struct {
	AnthropicAPIKey string
	OpenAIAPIKey    string
	MaxRetries      int
}

// NewTiltfileGeneratorConfig creates a new config with default values
func NewTiltfileGeneratorConfig() *TiltfileGeneratorConfig {
	return &TiltfileGeneratorConfig{
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		OpenAIAPIKey:    os.Getenv("OPENAI_API_KEY"),
		MaxRetries:      2,
	}
}

// RepositoryContext holds context about the repository for Tiltfile generation
type RepositoryContext struct {
	ReadmeContent      string
	DockerFiles        []string
	DockerComposeFiles []string
	PackageFiles       []string
	ServiceFiles       []string
	RepoPath           string
}

// GenerateTiltfile generates a Tiltfile based on repository context
func GenerateTiltfile(repoPath string, config *TiltfileGeneratorConfig) (string, error) {
	if config == nil {
		config = NewTiltfileGeneratorConfig()
	}

	// Gather repository context
	ctx, err := gatherRepositoryContext(repoPath)
	if err != nil {
		return "", fmt.Errorf("failed to gather repository context: %w", err)
	}

	// Check if we have any API keys available
	if config.AnthropicAPIKey == "" && config.OpenAIAPIKey == "" {
		// Fallback to simple template-based generation
		return generateFallbackTiltfile(ctx), nil
	}

	// Try generating Tiltfile with retries
	for attempt := 0; attempt < config.MaxRetries; attempt++ {
		var tiltfile string
		var err error

		// Try Anthropic first if API key is available
		if config.AnthropicAPIKey != "" {
			tiltfile, err = generateWithAnthropic(ctx, config)
		} else if config.OpenAIAPIKey != "" {
			tiltfile, err = generateWithOpenAI(ctx, config)
		}

		if err != nil {
			// If API fails, fall back to template-based generation
			return generateFallbackTiltfile(ctx), nil
		}

		if len(strings.TrimSpace(tiltfile)) > 0 {
			return tiltfile, nil
		}
	}

	// If all API attempts fail, use fallback
	return generateFallbackTiltfile(ctx), nil
}

// gatherRepositoryContext scans the repository to understand its structure
func gatherRepositoryContext(repoPath string) (*RepositoryContext, error) {
	ctx := &RepositoryContext{
		RepoPath:           repoPath,
		DockerFiles:        []string{},
		DockerComposeFiles: []string{},
		PackageFiles:       []string{},
		ServiceFiles:       []string{},
	}

	// Read README
	readmePaths := []string{"README.md", "README.rst", "README.txt", "readme.md", "Readme.md"}
	for _, readmePath := range readmePaths {
		fullPath := filepath.Join(repoPath, readmePath)
		if content, err := os.ReadFile(fullPath); err == nil {
			ctx.ReadmeContent = string(content)
			break
		}
	}

	// Walk through repository to find relevant files
	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue on errors
		}

		// Skip hidden directories and node_modules, vendor, etc.
		if info.IsDir() && (strings.HasPrefix(info.Name(), ".") ||
			info.Name() == "node_modules" ||
			info.Name() == "vendor" ||
			info.Name() == "__pycache__") {
			return filepath.SkipDir
		}

		relPath, _ := filepath.Rel(repoPath, path)
		fileName := info.Name()

		switch {
		case strings.HasPrefix(fileName, "Dockerfile") || fileName == "Dockerfile":
			ctx.DockerFiles = append(ctx.DockerFiles, relPath)
		case strings.Contains(fileName, "docker-compose") && (strings.HasSuffix(fileName, ".yml") || strings.HasSuffix(fileName, ".yaml")):
			ctx.DockerComposeFiles = append(ctx.DockerComposeFiles, relPath)
		case fileName == "package.json" || fileName == "pyproject.toml" || fileName == "requirements.txt" ||
			fileName == "go.mod" || fileName == "Cargo.toml" || fileName == "composer.json" ||
			fileName == "Gemfile" || fileName == "pom.xml" || fileName == "build.gradle":
			ctx.PackageFiles = append(ctx.PackageFiles, relPath)
		case fileName == "Makefile" || strings.HasSuffix(fileName, ".sh") || strings.HasSuffix(fileName, ".py") ||
			strings.HasSuffix(fileName, ".js") || strings.HasSuffix(fileName, ".ts") ||
			strings.HasSuffix(fileName, ".go") || strings.HasSuffix(fileName, ".rs"):
			// Only include scripts in the root or immediate subdirectories to avoid noise
			depth := len(strings.Split(relPath, string(filepath.Separator)))
			if depth <= 2 {
				ctx.ServiceFiles = append(ctx.ServiceFiles, relPath)
			}
		}

		return nil
	})

	return ctx, err
}

// buildTiltfileSystemPrompt creates the system prompt for Tiltfile generation
func buildTiltfileSystemPrompt(ctx *RepositoryContext) string {
	prompt := `You are an expert DevOps engineer helping to create a Tiltfile for a development environment. 

Your task is to generate a Tiltfile that:
1. Uses dockerized services for dependencies like databases (postgres, mysql, redis, etc.)
2. Prefers running application code on the host using local_resource() unless the README explicitly indicates containerization is preferred
3. Sets up proper port forwarding and service dependencies
4. Uses environment variables TASK_NAME, TASK_HOSTNAME, and TASK_PORT when appropriate

Here's the Tilt documentation context you need:

## Tilt Syntax Rules (IMPORTANT):
- Tilt uses Python-like syntax but is NOT Python
- Use single quotes for strings: 'example' not "example"
- No f-strings - use string concatenation with +
- Use os.getenv('VAR', 'default') for environment variables
- Port forwards are strings: ['3000:3000'] not [3000:3000]

## Key Functions:
- docker_compose('path') loads a docker-compose file
- local_resource('name', cmd='command', deps=['files'], port_forwards=['3000:3000'])
- resource_deps=['service'] creates dependencies between resources

## Example Patterns:

# Load docker-compose for dependencies
docker_compose('../docker-compose.yml')

# Run application on host (preferred for main app code)
local_resource(
    'web-server',
    cmd='npm run dev',
    deps=['package.json', 'src/'],
    resource_deps=['postgres'],
    port_forwards=[os.getenv('TASK_PORT', '3000') + ':3000']
)

# For Go applications
local_resource(
    'go-app',
    cmd='go run main.go',
    deps=['go.mod', '*.go'],
    port_forwards=[os.getenv('TASK_PORT', '8080') + ':8080']
)

# Use environment variables
print('Starting development environment for task: ' + os.getenv('TASK_NAME', 'unknown'))
print('Hostname: ' + os.getenv('TASK_HOSTNAME', 'localhost'))
print('Port: ' + os.getenv('TASK_PORT', '3000'))

Repository Context:
`

	// Add README content
	if ctx.ReadmeContent != "" {
		prompt += fmt.Sprintf("\n<readme>\n%s\n</readme>\n", ctx.ReadmeContent)
	}

	// Add Docker files
	if len(ctx.DockerFiles) > 0 {
		prompt += "\n<dockerfiles>\n"
		for _, file := range ctx.DockerFiles {
			prompt += fmt.Sprintf("- %s\n", file)
		}
		prompt += "</dockerfiles>\n"
	}

	// Add Docker Compose files
	if len(ctx.DockerComposeFiles) > 0 {
		prompt += "\n<docker_compose_files>\n"
		for _, file := range ctx.DockerComposeFiles {
			prompt += fmt.Sprintf("- %s\n", file)
		}
		prompt += "</docker_compose_files>\n"
	}

	// Add package files
	if len(ctx.PackageFiles) > 0 {
		prompt += "\n<package_files>\n"
		for _, file := range ctx.PackageFiles {
			prompt += fmt.Sprintf("- %s\n", file)
		}
		prompt += "</package_files>\n"
	}

	// Add service files (limited list)
	if len(ctx.ServiceFiles) > 0 {
		prompt += "\n<service_files>\n"
		for _, file := range ctx.ServiceFiles[:min(10, len(ctx.ServiceFiles))] { // Limit to avoid too much noise
			prompt += fmt.Sprintf("- %s\n", file)
		}
		if len(ctx.ServiceFiles) > 10 {
			prompt += fmt.Sprintf("... and %d more files\n", len(ctx.ServiceFiles)-10)
		}
		prompt += "</service_files>\n"
	}

	prompt += `
Generate ONLY the Tiltfile content. Do not include any explanations or markdown formatting.

IMPORTANT: This Tiltfile will be placed in .agent-farmer/ directory, so docker-compose files in the repo root need '../' prefix.

The Tiltfile should:
- Load docker-compose files for databases and external dependencies (use '../docker-compose.yml' for root files)
- Use local_resource for running the main application code (unless README says otherwise)
- Set up appropriate port forwarding
- Use resource dependencies where needed
- Include helpful comments
- Use the available environment variables (TASK_NAME, TASK_HOSTNAME, TASK_PORT) where appropriate
- For Go projects: use 'go run main.go' or 'go run .' as the main command
- For Node projects: use 'npm run dev' or 'npm start' 
- For Python projects: use 'python manage.py runserver' or similar

Start your response with the Tiltfile content directly.`

	return prompt
}

// generateWithAnthropic calls the Anthropic API to generate a Tiltfile
func generateWithAnthropic(ctx *RepositoryContext, config *TiltfileGeneratorConfig) (string, error) {
	systemPrompt := buildTiltfileSystemPrompt(ctx)

	reqBody := map[string]interface{}{
		"model":      "claude-3-haiku-20240307",
		"max_tokens": 2000,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": systemPrompt,
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", config.AnthropicAPIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var response struct {
		Content []struct {
			Text string `json:"text"`
			Type string `json:"type"`
		} `json:"content"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(response.Content) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	return response.Content[0].Text, nil
}

// generateWithOpenAI calls the OpenAI API to generate a Tiltfile
func generateWithOpenAI(ctx *RepositoryContext, config *TiltfileGeneratorConfig) (string, error) {
	systemPrompt := buildTiltfileSystemPrompt(ctx)

	reqBody := map[string]interface{}{
		"model":      "gpt-3.5-turbo",
		"max_tokens": 2000,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": systemPrompt,
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.OpenAIAPIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return response.Choices[0].Message.Content, nil
}

// generateFallbackTiltfile creates a basic Tiltfile when LLM generation fails
func generateFallbackTiltfile(ctx *RepositoryContext) string {
	var tiltfile strings.Builder

	tiltfile.WriteString("# Tiltfile for agent-farmer development environment\n")
	tiltfile.WriteString("# This file was auto-generated by agent-farmer\n\n")

	// Load docker-compose files
	if len(ctx.DockerComposeFiles) > 0 {
		tiltfile.WriteString("# Load docker-compose files for dependencies\n")
		for _, file := range ctx.DockerComposeFiles {
			// Adjust path relative to .agent-farmer directory
			relPath := file
			if !strings.Contains(file, "/") {
				relPath = "../" + file
			}
			tiltfile.WriteString(fmt.Sprintf("docker_compose('%s')\n", relPath))
		}
		tiltfile.WriteString("\n")
	}

	// Add basic local resource based on detected package files
	tiltfile.WriteString("# Application services\n")
	hasNodeJS := false
	hasPython := false
	hasGo := false

	for _, file := range ctx.PackageFiles {
		switch {
		case strings.Contains(file, "package.json"):
			hasNodeJS = true
		case strings.Contains(file, "requirements.txt") || strings.Contains(file, "pyproject.toml"):
			hasPython = true
		case strings.Contains(file, "go.mod"):
			hasGo = true
		}
	}

	if hasNodeJS {
		tiltfile.WriteString(`local_resource(
    'web-app',
    cmd='npm install && npm run dev',
    serve_cmd='npm run dev',
    deps=['package.json', 'src/', 'public/'],
    port_forwards=[os.getenv('TASK_PORT', '3000') + ':3000']
)

`)
	} else if hasPython {
		tiltfile.WriteString(`local_resource(
    'python-app',
    cmd='pip install -r requirements.txt && python manage.py runserver',
    serve_cmd='python manage.py runserver',
    deps=['requirements.txt', '*.py'],
    port_forwards=[os.getenv('TASK_PORT', '8000') + ':8000']
)

`)
	} else if hasGo {
		tiltfile.WriteString(`local_resource(
    'go-app',
    cmd='go run .',
    serve_cmd='go run .',
    deps=['*.go', 'go.mod'],
    port_forwards=[os.getenv('TASK_PORT', '8080') + ':8080']
)

`)
	} else {
		tiltfile.WriteString("# Add your application services here\n")
		tiltfile.WriteString("# Example:\n")
		tiltfile.WriteString("# local_resource(\n")
		tiltfile.WriteString("#     'my-app',\n")
		tiltfile.WriteString("#     cmd='make run',\n")
		tiltfile.WriteString("#     deps=['src/'],\n")
		tiltfile.WriteString("#     port_forwards=['3000:3000']\n")
		tiltfile.WriteString("# )\n\n")
	}

	// Add environment variable usage
	tiltfile.WriteString("# Environment variables available:\n")
	tiltfile.WriteString("# TASK_NAME - the name of the current agent task\n")
	tiltfile.WriteString("# TASK_HOSTNAME - the generated hostname for this task\n")
	tiltfile.WriteString("# TASK_PORT - the base port for this task\n\n")

	tiltfile.WriteString("print('Starting development environment for task: ' + os.getenv('TASK_NAME', 'unknown'))\n")
	tiltfile.WriteString("print('Hostname: ' + os.getenv('TASK_HOSTNAME', 'unknown'))\n")
	tiltfile.WriteString("print('Port: ' + os.getenv('TASK_PORT', '3000'))\n")

	return tiltfile.String()
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
