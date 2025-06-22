package main

import (
	"agent-farmer/app"
	cmd2 "agent-farmer/cmd"
	"agent-farmer/config"
	"agent-farmer/daemon"
	"agent-farmer/devenv"
	"agent-farmer/log"
	"agent-farmer/session"
	"agent-farmer/session/git"
	"agent-farmer/session/tmux"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	version     = "1.0.5"
	programFlag string
	autoYesFlag bool
	daemonFlag  bool
	rootCmd     = &cobra.Command{
		Use:   "agent-farmer",
		Short: "Agent Farmer - Manage multiple AI agents like Claude Code, Aider, Codex, and Amp.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			log.Initialize(daemonFlag)
			defer log.Close()

			if daemonFlag {
				cfg := config.LoadConfig()
				err := daemon.RunDaemon(cfg)
				log.ErrorLog.Printf("failed to start daemon %v", err)
				return err
			}

			// Check if we're in a git repository
			currentDir, err := filepath.Abs(".")
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}

			if !git.IsGitRepo(currentDir) {
				return fmt.Errorf("error: agent-farmer must be run from within a git repository")
			}

			cfg := config.LoadConfig()

			// Program flag overrides config
			program := cfg.DefaultProgram
			if programFlag != "" {
				program = programFlag
			}
			// AutoYes flag overrides config
			autoYes := cfg.AutoYes
			if autoYesFlag {
				autoYes = true
			}
			if autoYes {
				defer func() {
					if err := daemon.LaunchDaemon(); err != nil {
						log.ErrorLog.Printf("failed to launch daemon: %v", err)
					}
				}()
			}
			// Kill any daemon that's running.
			if err := daemon.StopDaemon(); err != nil {
				log.ErrorLog.Printf("failed to stop daemon: %v", err)
			}

			return app.Run(ctx, program, autoYes)
		},
	}

	resetCmd = &cobra.Command{
		Use:   "reset",
		Short: "Reset all stored instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			log.Initialize(false)
			defer log.Close()

			force, _ := cmd.Flags().GetBool("force")

			state := config.LoadState()
			storage, err := session.NewStorage(state)
			if err != nil {
				return fmt.Errorf("failed to initialize storage: %w", err)
			}
			if err := storage.DeleteAllInstances(); err != nil {
				return fmt.Errorf("failed to reset storage: %w", err)
			}
			fmt.Println("Storage has been reset successfully")

			if err := tmux.CleanupSessions(cmd2.MakeExecutor()); err != nil {
				return fmt.Errorf("failed to cleanup tmux sessions: %w", err)
			}
			fmt.Println("Tmux sessions have been cleaned up")

			if err := git.CleanupWorktrees(); err != nil {
				return fmt.Errorf("failed to cleanup worktrees: %w", err)
			}
			fmt.Println("Worktrees have been cleaned up")

			// Kill any daemon that's running.
			if err := daemon.StopDaemon(); err != nil {
				return err
			}
			fmt.Println("daemon has been stopped")

			// If force flag is set, also delete all cached repo configs
			if force {
				if err := config.DeleteAllRepoConfigs(); err != nil {
					return fmt.Errorf("failed to delete cached repo configs: %w", err)
				}
				fmt.Println("Cached repository configs have been reset")
			}

			return nil
		},
	}

	debugCmd = &cobra.Command{
		Use:   "debug",
		Short: "Print debug information like config paths",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.LoadConfig()

			configDir, err := config.GetConfigDir()
			if err != nil {
				return fmt.Errorf("failed to get config directory: %w", err)
			}
			configJson, _ := json.MarshalIndent(cfg, "", "  ")

			fmt.Printf("Config: %s\n%s\n", filepath.Join(configDir, config.ConfigFileName), configJson)

			return nil
		},
	}

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version number of agent-farmer",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("agent-farmer version %s\n", version)
			fmt.Printf("https://github.com/howinator/agent-farmer/releases/tag/v%s\n", version)
		},
	}

	devEnvCmd = &cobra.Command{
		Use:   "devenv",
		Short: "Manage development environment settings",
		Long:  "Configure and manage the development environment that can be automatically started when agents complete tasks",
	}

	devEnvEnableCmd = &cobra.Command{
		Use:   "enable",
		Short: "Enable development environment for this repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			log.Initialize(false)
			defer log.Close()

			currentDir, err := filepath.Abs(".")
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}

			if !git.IsGitRepo(currentDir) {
				return fmt.Errorf("error: must be run from within a git repository")
			}

			devEnvManager, err := devenv.NewDevEnvironmentManager(currentDir)
			if err != nil {
				return fmt.Errorf("failed to initialize development environment: %w", err)
			}

			// Auto-detect configuration
			if err := devEnvManager.AutoDetectConfiguration(); err != nil {
				return fmt.Errorf("failed to auto-detect configuration: %w", err)
			}

			if err := devEnvManager.Enable(); err != nil {
				return fmt.Errorf("failed to enable development environment: %w", err)
			}

			fmt.Println("Development environment enabled for this repository")

			// Show current configuration
			config := devEnvManager.GetConfiguration()
			if config != nil {
				fmt.Printf("Tiltfile: %s\n", config.TiltfilePath)
				fmt.Printf("Docker Compose: %s\n", config.DockerComposePath)
				fmt.Printf("Hostname Pattern: %s\n", config.HostnamePattern)
				fmt.Printf("Port: %d\n", config.Port)
			}

			return nil
		},
	}

	devEnvDisableCmd = &cobra.Command{
		Use:   "disable",
		Short: "Disable development environment for this repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			log.Initialize(false)
			defer log.Close()

			currentDir, err := filepath.Abs(".")
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}

			if !git.IsGitRepo(currentDir) {
				return fmt.Errorf("error: must be run from within a git repository")
			}

			devEnvManager, err := devenv.NewDevEnvironmentManager(currentDir)
			if err != nil {
				return fmt.Errorf("failed to initialize development environment: %w", err)
			}

			if err := devEnvManager.Disable(); err != nil {
				return fmt.Errorf("failed to disable development environment: %w", err)
			}

			fmt.Println("Development environment disabled for this repository")
			return nil
		},
	}

	devEnvStatusCmd = &cobra.Command{
		Use:   "status",
		Short: "Show development environment status",
		RunE: func(cmd *cobra.Command, args []string) error {
			log.Initialize(false)
			defer log.Close()

			currentDir, err := filepath.Abs(".")
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}

			if !git.IsGitRepo(currentDir) {
				return fmt.Errorf("error: must be run from within a git repository")
			}

			devEnvManager, err := devenv.NewDevEnvironmentManager(currentDir)
			if err != nil {
				return fmt.Errorf("failed to initialize development environment: %w", err)
			}

			enabled := devEnvManager.IsEnabled()
			status, err := devEnvManager.GetStatus()
			if err != nil {
				return fmt.Errorf("failed to get status: %w", err)
			}

			fmt.Printf("Development Environment Status:\n")
			fmt.Printf("  Enabled: %v\n", enabled)
			fmt.Printf("  Status: %s\n", status)

			config := devEnvManager.GetConfiguration()
			if config != nil && enabled {
				fmt.Printf("  Configuration:\n")
				fmt.Printf("    Tiltfile: %s\n", config.TiltfilePath)
				fmt.Printf("    Docker Compose: %s\n", config.DockerComposePath)
				fmt.Printf("    Hostname Pattern: %s\n", config.HostnamePattern)
				fmt.Printf("    Port: %d\n", config.Port)
				if len(config.Services) > 0 {
					fmt.Printf("    Services: %v\n", config.Services)
				}
			}

			return nil
		},
	}

	devEnvInitCmd = &cobra.Command{
		Use:   "init",
		Short: "Initialize development environment with an intelligent Tiltfile",
		Long:  "Generate a Tiltfile based on repository analysis using LLM (requires ANTHROPIC_API_KEY or OPENAI_API_KEY)",
		RunE: func(cmd *cobra.Command, args []string) error {
			log.Initialize(false)
			defer log.Close()

			currentDir, err := filepath.Abs(".")
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}

			if !git.IsGitRepo(currentDir) {
				return fmt.Errorf("error: must be run from within a git repository")
			}

			devEnvManager, err := devenv.NewDevEnvironmentManager(currentDir)
			if err != nil {
				return fmt.Errorf("failed to initialize development environment: %w", err)
			}

			force, _ := cmd.Flags().GetBool("force")
			if force {
				// Remove existing Tiltfile if force flag is set
				tiltfilePath := filepath.Join(currentDir, ".agent-farmer", "Tiltfile")
				if err := os.Remove(tiltfilePath); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("failed to remove existing Tiltfile: %w", err)
				}
			}

			if err := devEnvManager.CreateDefaultTiltfile(); err != nil {
				return fmt.Errorf("failed to create Tiltfile: %w", err)
			}

			fmt.Println("Intelligent Tiltfile created successfully")
			fmt.Println("The Tiltfile was generated based on your repository structure and README")
			fmt.Println("You can customize it further for your specific needs")

			// Show API key status
			hasAnthropic := os.Getenv("ANTHROPIC_API_KEY") != ""
			hasOpenAI := os.Getenv("OPENAI_API_KEY") != ""
			if !hasAnthropic && !hasOpenAI {
				fmt.Println("\nNote: No API keys found - used fallback template generation")
				fmt.Println("Set ANTHROPIC_API_KEY or OPENAI_API_KEY for smarter Tiltfile generation")
			}

			return nil
		},
	}

	devEnvGenerateCmd = &cobra.Command{
		Use:   "generate",
		Short: "Regenerate Tiltfile with current repository analysis",
		Long:  "Force regeneration of the Tiltfile using LLM analysis of the current repository state",
		RunE: func(cmd *cobra.Command, args []string) error {
			log.Initialize(false)
			defer log.Close()

			currentDir, err := filepath.Abs(".")
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}

			if !git.IsGitRepo(currentDir) {
				return fmt.Errorf("error: must be run from within a git repository")
			}

			devEnvManager, err := devenv.NewDevEnvironmentManager(currentDir)
			if err != nil {
				return fmt.Errorf("failed to initialize development environment: %w", err)
			}

			// Remove existing Tiltfile
			tiltfilePath := filepath.Join(currentDir, ".agent-farmer", "Tiltfile")
			if err := os.Remove(tiltfilePath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to remove existing Tiltfile: %w", err)
			}

			if err := devEnvManager.CreateDefaultTiltfile(); err != nil {
				return fmt.Errorf("failed to generate Tiltfile: %w", err)
			}

			fmt.Println("Tiltfile regenerated successfully")
			fmt.Println("The new Tiltfile reflects the current repository structure")

			return nil
		},
	}
)

func init() {
	rootCmd.Flags().StringVarP(&programFlag, "program", "p", "",
		"Program to run in new instances (e.g. 'aider --model ollama_chat/gemma3:1b')")
	rootCmd.Flags().BoolVarP(&autoYesFlag, "autoyes", "y", false,
		"[experimental] If enabled, all instances will automatically accept prompts")
	rootCmd.Flags().BoolVar(&daemonFlag, "daemon", false, "Run a program that loads all sessions"+
		" and runs autoyes mode on them.")

	// Hide the daemonFlag as it's only for internal use
	err := rootCmd.Flags().MarkHidden("daemon")
	if err != nil {
		panic(err)
	}

	// Add force flag to reset command
	resetCmd.Flags().BoolVarP(new(bool), "force", "f", false, "Also reset cached repository configurations")

	// Add flags to devenv commands
	devEnvInitCmd.Flags().BoolP("force", "f", false, "Overwrite existing Tiltfile if it exists")

	// Add subcommands to devenv command
	devEnvCmd.AddCommand(devEnvEnableCmd)
	devEnvCmd.AddCommand(devEnvDisableCmd)
	devEnvCmd.AddCommand(devEnvStatusCmd)
	devEnvCmd.AddCommand(devEnvInitCmd)
	devEnvCmd.AddCommand(devEnvGenerateCmd)

	rootCmd.AddCommand(debugCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(resetCmd)
	rootCmd.AddCommand(devEnvCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
	}
}
