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

## Development Environment System (`devenv/`)

Claude Squad includes an intelligent development environment system that automatically provisions and starts project services when AI agents complete their tasks. This provides immediate access to test and review agent work in a live environment.

### Architecture Components

#### 1. Development Environment Manager (`devenv/devenv.go`)

The `DevEnvironmentManager` serves as the main orchestrator for development environments:

```go
type DevEnvironmentManager struct {
    repoPath   string
    repoConfig *config.RepoConfig
}
```

**Core Responsibilities:**
- Repository-specific configuration management
- Auto-detection of project structure and services
- Lifecycle management (start/stop/status)
- Integration with the session management system

#### 2. LLM-Powered Tiltfile Generator (`devenv/tiltfile_generator.go`)

An intelligent Tiltfile generation system that analyzes repository context to create optimized development configurations:

```go
type RepositoryContext struct {
    ReadmeContent      string
    DockerFiles        []string
    DockerComposeFiles []string
    PackageFiles       []string
    ServiceFiles       []string
    RepoPath           string
}
```

**Generation Process:**
1. **Repository Analysis**: Scans for package managers, Dockerfiles, README content
2. **LLM Processing**: Uses Anthropic Claude or OpenAI GPT to understand project structure
3. **Tiltfile Generation**: Creates context-aware Tilt configuration
4. **Fallback System**: Template-based generation when API keys unavailable

#### 3. Configuration Integration

Development environment settings are stored in the existing `RepoConfig` system:

```go
type DevEnvironmentConfig struct {
    Enabled           bool     `json:"enabled"`
    TiltfilePath      string   `json:"tiltfile_path"`
    DockerComposePath string   `json:"docker_compose_path"`
    Services          []string `json:"services,omitempty"`
    HostnamePattern   string   `json:"hostname_pattern"`
    Port              int      `json:"port"`
}
```

### Supported Technology Detection

The system automatically detects and configures environments for multiple technology stacks:

| Technology | Detection File | Generated Command | Default Port |
|------------|---------------|-------------------|--------------|
| Node.js | `package.json` | `npm run dev` | 3000 |
| Python/Django | `requirements.txt`, `pyproject.toml` | `python manage.py runserver` | 8000 |
| Go | `go.mod` | `go run main.go` | 8080 |
| Rust | `Cargo.toml` | `cargo run` | 8080 |
| PHP | `composer.json` | `php -S localhost:8000` | 8000 |
| Ruby/Rails | `Gemfile` | `rails server` | 3000 |
| Java/Maven | `pom.xml` | `mvn spring-boot:run` | 8080 |
| Java/Gradle | `build.gradle` | `./gradlew bootRun` | 8080 |

### Design Principles

#### 1. Host-First Application Execution
- **Applications**: Run on host using `local_resource()` for faster iteration
- **Dependencies**: Containerized using docker-compose for consistency
- **Benefits**: Hot reloading, direct debugging, host-native performance

#### 2. Task-Specific Isolation
- **Hostname Generation**: Each agent task gets unique hostname (e.g., `github-123.agent`)
- **Port Allocation**: Dynamic port assignment based on task context
- **Environment Variables**: `TASK_NAME`, `TASK_HOSTNAME`, `TASK_PORT` available to services

#### 3. Intelligent Configuration
- **LLM Analysis**: Uses AI to understand project structure from README and code
- **Preference Detection**: Respects containerization preferences indicated in README
- **Fallback Generation**: Robust template-based generation without API dependencies

### Integration with Session Lifecycle

#### Auto-Start Mechanism
The development environment integrates with the daemon polling system:

```go
// In daemon/daemon.go
if err := instance.AutoStartDevEnvironmentIfEnabled(); err != nil {
    log.WarningLog.Printf("failed to auto-start dev environment for %s: %v", 
                         instance.Title, err)
}
```

#### State-Based Triggering
- **Condition**: Development environment starts when session reaches `Ready` state
- **Indicator**: `Ready` state means agent is waiting for user input (task completion)
- **Isolation**: Each session's environment runs independently

#### Resource Management
- **Startup**: Environment starts automatically on agent completion
- **Cleanup**: Environment stops when session is killed or paused
- **Persistence**: Configuration survives session lifecycle

### CLI Interface

The development environment provides a comprehensive CLI interface:

```bash
af devenv enable    # Enable dev environment for repository
af devenv disable   # Disable dev environment  
af devenv status    # Show current configuration and status
af devenv init      # Generate intelligent Tiltfile using LLM
af devenv generate  # Regenerate Tiltfile with current analysis
```

### File Organization

Development environment files are organized in the `.agent-farmer/` directory:

```
repository/
├── .agent-farmer/
│   ├── Tiltfile           # Generated development configuration
│   └── repo-config.json   # Repository-specific settings
├── docker-compose.yml     # Docker services (if present)
└── package.json          # Application definition (example)
```

### LLM Prompt Engineering

The Tiltfile generator uses sophisticated prompts that include:

#### Context Sections
- **Repository Structure**: File listings, package managers, Docker configurations
- **Documentation**: README content for preference detection
- **Technology Stack**: Detected languages and frameworks

#### Generation Rules
- **Syntax Compliance**: Ensures valid Tilt Python syntax
- **Best Practices**: Follows Tilt development patterns
- **Environment Integration**: Proper use of task-specific variables

#### Example Generated Output
```python
# Load docker-compose for dependencies
docker_compose('../docker-compose.yml')

# Run the main application on the host
local_resource(
    'web-app',
    cmd='npm run dev',
    deps=['package.json', 'src/'],
    resource_deps=['postgres', 'redis'],
    port_forwards=[os.getenv('TASK_PORT', '3000') + ':3000']
)

print('Starting development environment for task: ' + os.getenv('TASK_NAME', 'unknown'))
print('Hostname: ' + os.getenv('TASK_HOSTNAME', 'localhost'))
```

### Performance Considerations

#### Lazy Initialization
- Repository analysis only occurs when explicitly requested
- LLM calls are cached implicitly through file generation
- No performance impact on core session management

#### Error Resilience
- LLM failures gracefully degrade to template generation
- Missing dependencies (Tilt, Docker) don't break core functionality
- Configuration errors are isolated per repository

#### Resource Efficiency
- Development environments only start when agents complete tasks
- Each environment runs in isolated containers/processes
- Automatic cleanup prevents resource accumulation

This architecture provides a seamless bridge between AI agent task completion and immediate development environment access, enabling rapid iteration and testing of agent-generated code.

## Key Features

- **Isolated Workspaces**: Each session runs in its own git worktree, preventing conflicts
- **Background Execution**: Sessions can run tasks in background with auto-accept mode
- **Session Persistence**: Sessions can be paused (preserving branch) and resumed later
- **Multi-Assistant Support**: Works with Claude Code, Aider, Codex and other local agents
- **Smart Rebasing**: Uses `git rebase --onto` pattern with cached default branch detection
- **Intelligent Development Environment**: LLM-powered automatic setup of project services when agents complete tasks

## Configuration

- Config stored in `~/.agent-farmer/` directory
- Application state persisted in JSON format
- Repository-specific cache files for performance optimization
- Branch naming configurable via `BranchPrefix` setting
- Default program and auto-yes mode configurable

## Prerequisites

### Core Requirements
The application requires:
- tmux (for session management)
- gh (GitHub CLI, for git operations)
- Must be run from within a git repository

### Development Environment (Optional)
For development environment functionality:
- [Tilt](https://tilt.dev/) (for orchestrating development environments)
- [Docker](https://docker.com/) (for running containerized services)
- API keys for intelligent Tiltfile generation:
  - `ANTHROPIC_API_KEY` (recommended) or `OPENAI_API_KEY`