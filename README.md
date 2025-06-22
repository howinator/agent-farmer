# Agent Farmer [![CI](https://github.com/howinator/agent-farmer/actions/workflows/build.yml/badge.svg)](https://github.com/howinator/agent-farmer/actions/workflows/build.yml) [![GitHub Release](https://img.shields.io/github/v/release/howinator/agent-farmer)](https://github.com/howinator/agent-farmer/releases/latest)

[Agent Farmer](https://smtg-ai.github.io/agent-farmer/) is a terminal app that manages multiple [Claude Code](https://github.com/anthropiaf/claude-code), [Codex](https://github.com/openai/codex) (and other local agents including [Aider](https://github.com/Aider-AI/aider)) in separate workspaces, allowing you to work on multiple tasks simultaneously.


![Agent Farmer Screenshot](assets/screenshot.png)

### Highlights
- Complete tasks in the background (including yolo / auto-accept mode!)
- Manage instances and tasks in one terminal window
- Review changes before applying them, checkout changes before pushing them
- Each task gets its own isolated git workspace, so no conflicts
- **🚀 NEW: Intelligent development environment** - automatically spins up your project when agents finish


### Installation

Both Homebrew and manual installation will install Agent Farmer as `af` on your system.

#### Homebrew

```bash
brew install agent-farmer
ln -s "$(brew --prefix)/bin/agent-farmer" "$(brew --prefix)/bin/af"
```

#### Manual

Agent Farmer can also be installed by running the following command:

```bash
curl -fsSL https://raw.githubusercontent.com/howinator/agent-farmer/main/install.sh | bash
```

This puts the `af` binary in `~/.local/bin`.

To use a custom name for the binary:

```bash
curl -fsSL https://raw.githubusercontent.com/howinator/agent-farmer/main/install.sh | bash -s -- --name <your-binary-name>
```

### Prerequisites

**Core Requirements:**
- [tmux](https://github.com/tmux/tmux/wiki/Installing)
- [gh](https://cli.github.com/)

**For Development Environment (optional):**
- [Tilt](https://tilt.dev/) - For orchestrating development environments
- [Docker](https://docker.com/) - For running containerized services

### Usage

```
Usage:
  af [flags]
  af [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  debug       Print debug information like config paths
  devenv      Manage development environment settings
  help        Help about any command
  reset       Reset all stored instances
  version     Print the version number of agent-farmer

Flags:
  -y, --autoyes          [experimental] If enabled, all instances will automatically accept prompts for claude code & aider
  -h, --help             help for agent-farmer
  -p, --program string   Program to run in new instances (e.g. 'aider --model ollama_chat/gemma3:1b')
```

Run the application with:

```bash
af
```

<br />

<b>Using Agent Farmer with other AI assistants:</b>
- For [Codex](https://github.com/openai/codex): Set your API key with `export OPENAI_API_KEY=<your_key>`
- Launch with specific assistants:
   - Codex: `af -p "codex"`
   - Aider: `af -p "aider ..."`
- Make this the default, by modifying the config file (locate with `af debug`)

<br />

#### Menu
The menu at the bottom of the screen shows available commands: 

##### Instance/Session Management
- `n` - Create a new session
- `N` - Create a new session with a prompt
- `D` - Kill (delete) the selected session
- `↑/j`, `↓/k` - Navigate between sessions

##### Actions
- `↵/o` - Attach to the selected session to reprompt
- `ctrl-q` - Detach from session
- `s` - Commit and push branch to github
- `c` - Checkout. Commits changes and pauses the session
- `r` - Resume a paused session
- `?` - Show help menu

##### Navigation
- `tab` - Switch between preview tab and diff tab
- `q` - Quit the application
- `shift-↓/↑` - scroll in diff view

### How It Works

1. **tmux** to create isolated terminal sessions for each agent
2. **git worktrees** to isolate codebases so each session works on its own branch
3. A simple TUI interface for easy navigation and management

## Development Environment

Agent Farmer includes an intelligent development environment system that automatically spins up your project's services when agents complete their tasks. This gives you immediate access to test and review the agent's work.

### Quick Start

```bash
# Enable development environment for your repository
af devenv enable

# Generate an intelligent Tiltfile based on your project
af devenv init

# Check status
af devenv status
```

### Features

- **🤖 Intelligent Setup**: Uses LLM analysis to understand your project structure
- **🐳 Docker Integration**: Automatically detects and uses docker-compose files
- **🌐 Task-Specific Hostnames**: Each agent task gets its own hostname (e.g., `github-123.agent`)
- **⚡ Auto-Start**: Development environment starts automatically when agents finish
- **📁 Organized**: Keeps configuration in `.agent-farmer/` directory

### Supported Languages & Frameworks

Agent Farmer automatically detects and configures development environments for:

| Language/Framework | Package File | Dev Command | Port |
|-------------------|--------------|-------------|------|
| **Node.js** | `package.json` | `npm run dev` | 3000 |
| **Python/Django** | `requirements.txt`, `pyproject.toml` | `python manage.py runserver` | 8000 |
| **Go** | `go.mod` | `go run main.go` | 8080 |
| **Rust** | `Cargo.toml` | `cargo run` | 8080 |
| **PHP** | `composer.json` | `php -S localhost:8000` | 8000 |
| **Ruby/Rails** | `Gemfile` | `rails server` | 3000 |
| **Java/Maven** | `pom.xml` | `mvn spring-boot:run` | 8080 |
| **Java/Gradle** | `build.gradle` | `./gradlew bootRun` | 8080 |

### Docker Services

The system automatically containerizes these services for dependencies:
- **PostgreSQL** (`postgres`)
- **MySQL** (`mysql`) 
- **Redis** (`redis`)
- **MongoDB** (`mongodb`)
- **Elasticsearch** (`elasticsearch`)

### Configuration

#### Basic Commands

```bash
# Enable development environment
af devenv enable

# Disable development environment  
af devenv disable

# Check current status
af devenv status

# Generate/regenerate Tiltfile
af devenv init [--force]
af devenv generate
```

#### LLM-Powered Generation

For the most intelligent Tiltfile generation, set up API access:

```bash
# Using Anthropic Claude (recommended)
export ANTHROPIC_API_KEY=your_key_here

# Or using OpenAI
export OPENAI_API_KEY=your_key_here
```

**With API keys**: Creates sophisticated Tiltfiles that understand your project structure, README preferences, and technology stack.

**Without API keys**: Falls back to template-based generation using detected package files.

#### Environment Variables

The development environment provides these variables to your Tiltfile:

- `TASK_NAME` - Name of the current agent task
- `TASK_HOSTNAME` - Generated hostname for this task  
- `TASK_PORT` - Base port for this task

#### Example Generated Tiltfile

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

#### Installing Development Environment Prerequisites

```bash
# Install Tilt
curl -fsSL https://raw.githubusercontent.com/tilt-dev/tilt/master/scripts/install.sh | bash

# Or via Homebrew
brew install tilt-dev/tap/tilt

# Install Docker
# Visit https://docker.com/get-started or use your system's package manager
```

### How It Works

1. **Repository Analysis**: Scans your project for Dockerfiles, docker-compose files, package managers, and README
2. **LLM Processing**: Uses Claude or GPT to understand your project structure and generate appropriate configuration
3. **Tiltfile Generation**: Creates a Tiltfile that runs dependencies in Docker and your app on the host
4. **Auto-Start**: When agents reach "Ready" state, automatically starts the development environment
5. **Task Isolation**: Each agent task gets its own hostname and port configuration

### Acknowledgements

Agent Farmer is based on the original [Claude Squad](https://github.com/smtg-ai/claude-squad) project. Thanks to the smtg-ai team for the foundational concept and implementation.

### License

[AGPL-3.0](LICENSE.md)

### Star History

[![Star History Chart](https://api.star-history.com/svg?repos=howinator/agent-farmer&type=Date)](https://www.star-history.com/#howinator/agent-farmer&Date)
