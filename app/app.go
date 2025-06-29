package app

import (
	"agent-farmer/config"
	"agent-farmer/keys"
	"agent-farmer/log"
	"agent-farmer/session"
	"agent-farmer/session/tmux"
	"agent-farmer/ui"
	"agent-farmer/ui/overlay"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const GlobalInstanceLimit = 10

// Custom message types
type pushCompleteMsg struct{}
type rebaseCompleteMsg struct{}
type operationCompleteMsg struct{}

// Run is the main entrypoint into the application.
func Run(ctx context.Context, program string, autoYes bool) error {
	p := tea.NewProgram(
		newHome(ctx, program, autoYes),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(), // Mouse scroll
	)
	_, err := p.Run()
	return err
}

type state int

const (
	stateDefault state = iota
	// stateNew is the state when the user is creating a new instance.
	stateNew
	// statePrompt is the state when the user is entering a prompt.
	statePrompt
	// statePromptForName is the state when collecting a prompt to generate a name.
	statePromptForName
	// stateHelp is the state when a help screen is displayed.
	stateHelp
	// stateConfirm is the state when a confirmation modal is displayed.
	stateConfirm
	// stateLoading is the state when a loading indicator is displayed.
	stateLoading
)

type home struct {
	ctx context.Context

	// -- Storage and Configuration --

	program string
	autoYes bool

	// storage is the interface for saving/loading data to/from the app's state
	storage *session.Storage
	// appConfig stores persistent application configuration
	appConfig *config.Config
	// appState stores persistent application state like seen help screens
	appState config.AppState

	// -- State --

	// state is the current discrete state of the application
	state state
	// newInstanceFinalizer is called when the state is stateNew and then you press enter.
	// It registers the new instance in the list after the instance has been started.
	newInstanceFinalizer func()

	// promptAfterName tracks if we should enter prompt mode after naming
	promptAfterName bool

	// keySent is used to manage underlining menu items
	keySent bool

	// quitConfirmed is set to true when user confirms quit
	quitConfirmed bool

	// -- UI Components --

	// list displays the list of instances
	list *ui.List
	// menu displays the bottom menu
	menu *ui.Menu
	// tabbedWindow displays the tabbed window with preview and diff panes
	tabbedWindow *ui.TabbedWindow
	// errBox displays error messages
	errBox *ui.ErrBox
	// global spinner instance. we plumb this down to where it's needed
	spinner spinner.Model
	// textInputOverlay handles text input with state
	textInputOverlay *overlay.TextInputOverlay
	// textOverlay displays text information
	textOverlay *overlay.TextOverlay
	// confirmationOverlay displays confirmation modals
	confirmationOverlay *overlay.ConfirmationOverlay
	// loadingOverlay displays loading indicators
	loadingOverlay *overlay.LoadingOverlay
	// pendingAction stores the action to execute when confirmation is confirmed
	pendingAction tea.Cmd
	// pendingActionInfo stores more detailed information about pending actions
	pendingActionInfo *pendingActionInfo
}

func newHome(ctx context.Context, program string, autoYes bool) *home {
	// Load application config
	appConfig := config.LoadConfig()

	// Load application state
	appState := config.LoadState()

	// Initialize storage
	storage, err := session.NewStorage(appState)
	if err != nil {
		fmt.Printf("Failed to initialize storage: %v\n", err)
		os.Exit(1)
	}

	h := &home{
		ctx:          ctx,
		spinner:      spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		menu:         ui.NewMenu(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane()),
		errBox:       ui.NewErrBox(),
		storage:      storage,
		appConfig:    appConfig,
		program:      program,
		autoYes:      autoYes,
		state:        stateDefault,
		appState:     appState,
	}
	h.list = ui.NewList(&h.spinner, autoYes)

	// Load saved instances
	instances, err := storage.LoadInstances()
	if err != nil {
		fmt.Printf("Failed to load instances: %v\n", err)
		os.Exit(1)
	}

	// Add loaded instances to the list
	for _, instance := range instances {
		// Call the finalizer immediately.
		h.list.AddInstance(instance)()
		if autoYes {
			instance.AutoYes = true
		}
	}

	return h
}

// updateHandleWindowSizeEvent sets the sizes of the components.
// The components will try to render inside their bounds.
func (m *home) updateHandleWindowSizeEvent(msg tea.WindowSizeMsg) {
	// List takes 30% of width, preview takes 70%
	listWidth := int(float32(msg.Width) * 0.3)
	tabsWidth := msg.Width - listWidth

	// Menu takes 10% of height, list and window take 90%
	contentHeight := int(float32(msg.Height) * 0.9)
	menuHeight := msg.Height - contentHeight - 1     // minus 1 for error box
	m.errBox.SetSize(int(float32(msg.Width)*0.9), 1) // error box takes 1 row

	m.tabbedWindow.SetSize(tabsWidth, contentHeight)
	m.list.SetSize(listWidth, contentHeight)

	if m.textInputOverlay != nil {
		m.textInputOverlay.SetSize(int(float32(msg.Width)*0.6), int(float32(msg.Height)*0.4))
	}
	if m.textOverlay != nil {
		m.textOverlay.SetWidth(int(float32(msg.Width) * 0.6))
	}

	previewWidth, previewHeight := m.tabbedWindow.GetPreviewSize()
	if err := m.list.SetSessionPreviewSize(previewWidth, previewHeight); err != nil {
		log.ErrorLog.Print(err)
	}
	m.menu.SetSize(msg.Width, menuHeight)
}

func (m *home) Init() tea.Cmd {
	// Upon starting, we want to start the spinner. Whenever we get a spinner.TickMsg, we
	// update the spinner, which sends a new spinner.TickMsg. I think this lasts forever lol.
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			time.Sleep(100 * time.Millisecond)
			return previewTickMsg{}
		},
		tickUpdateMetadataCmd,
	)
}

func (m *home) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle confirmed quit
	if m.quitConfirmed {
		if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
			return m, m.handleError(err)
		}
		return m, tea.Quit
	}

	switch msg := msg.(type) {
	case pushCompleteMsg, rebaseCompleteMsg, operationCompleteMsg:
		// Handle operation completion - dismiss loading overlay and return to default state
		if m.loadingOverlay != nil {
			m.loadingOverlay.Dismiss()
			m.loadingOverlay = nil
		}
		m.state = stateDefault
		return m, nil
	case hideErrMsg:
		m.errBox.Clear()
	case previewTickMsg:
		cmd := m.instanceChanged()
		return m, tea.Batch(
			cmd,
			func() tea.Msg {
				time.Sleep(100 * time.Millisecond)
				return previewTickMsg{}
			},
		)
	case keyupMsg:
		m.menu.ClearKeydown()
		return m, nil
	case tickUpdateMetadataMessage:
		for _, instance := range m.list.GetInstances() {
			if !instance.Started() || instance.Paused() {
				continue
			}
			updated, prompt := instance.HasUpdated()
			if updated {
				instance.SetStatus(session.Running)
			} else {
				if prompt {
					instance.TapEnter()
				} else {
					instance.SetStatus(session.Ready)
				}
			}
			if err := instance.UpdateDiffStats(); err != nil {
				log.WarningLog.Printf("could not update diff stats: %v", err)
			}
		}
		return m, tickUpdateMetadataCmd
	case tea.MouseMsg:
		// Handle mouse wheel scrolling in the diff view
		if m.tabbedWindow.IsInDiffTab() {
			if msg.Action == tea.MouseActionPress {
				switch msg.Button {
				case tea.MouseButtonWheelUp:
					m.tabbedWindow.ScrollUp()
					return m, m.instanceChanged()
				case tea.MouseButtonWheelDown:
					m.tabbedWindow.ScrollDown()
					return m, m.instanceChanged()
				}
			}
		}
		return m, nil
	case tea.KeyMsg:
		return m.handleKeyPress(msg)
	case tea.WindowSizeMsg:
		m.updateHandleWindowSizeEvent(msg)
		return m, nil
	case error:
		// Handle errors from confirmation actions
		// Dismiss loading overlay if it's shown
		if m.loadingOverlay != nil {
			m.loadingOverlay.Dismiss()
			m.loadingOverlay = nil
		}
		m.state = stateDefault
		return m, m.handleError(msg)
	case instanceChangedMsg:
		// Handle instance changed after confirmation action
		return m, m.instanceChanged()
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)

		// Also update loading overlay spinner if we're in loading state
		if m.state == stateLoading && m.loadingOverlay != nil {
			loadingCmd := m.loadingOverlay.Update(msg)
			return m, tea.Batch(cmd, loadingCmd)
		}

		return m, cmd
	}
	return m, nil
}

func (m *home) handleQuitConfirmation() (tea.Model, tea.Cmd) {
	m.state = stateConfirm

	// Create and show the confirmation overlay
	m.confirmationOverlay = overlay.NewConfirmationOverlay("Are you sure you want to quit?")
	m.confirmationOverlay.SetWidth(50)

	// Set callbacks for confirmation and cancellation
	m.confirmationOverlay.OnConfirm = func() {
		m.state = stateDefault
		m.quitConfirmed = true
	}

	m.confirmationOverlay.OnCancel = func() {
		m.state = stateDefault
	}

	return m, nil
}

func (m *home) handleMenuHighlighting(msg tea.KeyMsg) (cmd tea.Cmd, returnEarly bool) {
	// Handle menu highlighting when you press a button. We intercept it here and immediately return to
	// update the ui while re-sending the keypress. Then, on the next call to this, we actually handle the keypress.
	if m.keySent {
		m.keySent = false
		return nil, false
	}
	if m.state == statePrompt || m.state == statePromptForName || m.state == stateHelp || m.state == stateConfirm {
		return nil, false
	}
	// If it's in the global keymap, we should try to highlight it.
	name, ok := keys.GlobalKeyStringsMap[msg.String()]
	if !ok {
		return nil, false
	}

	if m.list.GetSelectedInstance() != nil && m.list.GetSelectedInstance().Paused() && name == keys.KeyEnter {
		return nil, false
	}
	if name == keys.KeyShiftDown || name == keys.KeyShiftUp {
		return nil, false
	}

	// Skip the menu highlighting if the key is not in the map or we are using the shift up and down keys.
	// TODO: cleanup: when you press enter on stateNew, we use keys.KeySubmitName. We should unify the keymap.
	if name == keys.KeyEnter && m.state == stateNew {
		name = keys.KeySubmitName
	}
	m.keySent = true
	return tea.Batch(
		func() tea.Msg { return msg },
		m.keydownCallback(name)), true
}

func (m *home) handleKeyPress(msg tea.KeyMsg) (mod tea.Model, cmd tea.Cmd) {
	// Ignore all key presses during loading state
	if m.state == stateLoading {
		return m, nil
	}

	cmd, returnEarly := m.handleMenuHighlighting(msg)
	if returnEarly {
		return m, cmd
	}

	if m.state == stateHelp {
		return m.handleHelpState(msg)
	}

	if m.state == stateNew {
		// Handle quit commands first. Don't handle q because the user might want to type that.
		if msg.String() == "ctrl+c" {
			m.state = stateDefault
			m.promptAfterName = false
			m.list.Kill()
			return m, tea.Sequence(
				tea.WindowSize(),
				func() tea.Msg {
					m.menu.SetState(ui.StateDefault)
					return nil
				},
			)
		}

		instance := m.list.GetInstances()[m.list.NumInstances()-1]
		switch msg.Type {
		// Start the instance (enable previews etc) and go back to the main menu state.
		case tea.KeyEnter:
			if len(instance.Title) == 0 {
				return m, m.handleError(fmt.Errorf("title cannot be empty"))
			}

			if err := instance.Start(true); err != nil {
				m.list.Kill()
				m.state = stateDefault
				return m, m.handleError(err)
			}
			// Save after adding new instance
			if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
				return m, m.handleError(err)
			}
			// Instance added successfully, call the finalizer.
			m.newInstanceFinalizer()
			if m.autoYes {
				instance.AutoYes = true
			}

			m.newInstanceFinalizer()
			m.state = stateDefault
			if m.promptAfterName {
				m.state = statePrompt
				m.menu.SetState(ui.StatePrompt)
				// Initialize the text input overlay
				m.textInputOverlay = overlay.NewTextInputOverlay("Enter prompt", "")
				m.promptAfterName = false
			} else {
				m.menu.SetState(ui.StateDefault)
				m.showHelpScreen(helpTypeInstanceStart, nil)
			}

			return m, tea.Batch(tea.WindowSize(), m.instanceChanged())
		case tea.KeyRunes:
			if len(instance.Title) >= 32 {
				return m, m.handleError(fmt.Errorf("title cannot be longer than 32 characters"))
			}
			if err := instance.SetTitle(instance.Title + string(msg.Runes)); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeyBackspace:
			if len(instance.Title) == 0 {
				return m, nil
			}
			if err := instance.SetTitle(instance.Title[:len(instance.Title)-1]); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeySpace:
			if err := instance.SetTitle(instance.Title + " "); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeyEsc:
			m.list.Kill()
			m.state = stateDefault
			m.instanceChanged()

			return m, tea.Sequence(
				tea.WindowSize(),
				func() tea.Msg {
					m.menu.SetState(ui.StateDefault)
					return nil
				},
			)
		default:
		}
		return m, nil
	} else if m.state == statePrompt {
		// Use the new TextInputOverlay component to handle all key events
		shouldClose := m.textInputOverlay.HandleKeyPress(msg)

		// Check if the form was submitted or canceled
		if shouldClose {
			if m.textInputOverlay.IsSubmitted() {
				// Form was submitted, process the input
				selected := m.list.GetSelectedInstance()
				if selected == nil {
					return m, nil
				}
				if err := selected.SendPrompt(m.textInputOverlay.GetValue()); err != nil {
					return m, m.handleError(err)
				}
			}

			// Close the overlay and reset state
			m.textInputOverlay = nil
			m.state = stateDefault
			return m, tea.Sequence(
				tea.WindowSize(),
				func() tea.Msg {
					m.menu.SetState(ui.StateDefault)
					m.showHelpScreen(helpTypeInstanceStart, nil)
					return nil
				},
			)
		}

		return m, nil
	} else if m.state == statePromptForName {
		// Handle prompt collection for name generation
		shouldClose := m.textInputOverlay.HandleKeyPress(msg)

		// Check if the form was submitted or canceled
		if shouldClose {
			if m.textInputOverlay.IsSubmitted() {
				// Form was submitted, generate name and create instance
				prompt := m.textInputOverlay.GetValue()
				if prompt == "" {
					return m, m.handleError(fmt.Errorf("prompt cannot be empty"))
				}

				// Generate name using the prompt
				generatedName, err := session.GenerateSessionName(prompt, nil)
				if err != nil {
					return m, m.handleError(fmt.Errorf("failed to generate session name: %w", err))
				}

				// Create the instance with the generated name
				instance, err := session.NewInstance(session.InstanceOptions{
					Title:   generatedName,
					Path:    ".",
					Program: m.program,
				})
				if err != nil {
					return m, m.handleError(err)
				}

				// Start the instance
				if err := instance.Start(true); err != nil {
					return m, m.handleError(err)
				}

				// Add to list and save
				m.newInstanceFinalizer = m.list.AddInstance(instance)
				m.list.SetSelectedInstance(m.list.NumInstances() - 1)
				if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
					return m, m.handleError(err)
				}

				// Call the finalizer and set autoyes if needed
				m.newInstanceFinalizer()
				if m.autoYes {
					instance.AutoYes = true
				}

				// Close the overlay and reset state first
				m.textInputOverlay = nil
				m.state = stateDefault
				m.menu.SetState(ui.StateDefault)

				// Send the prompt after a brief delay to allow Claude to initialize
				return m, tea.Sequence(
					tea.WindowSize(),
					m.instanceChanged(),
					func() tea.Msg {
						time.Sleep(1000 * time.Millisecond) // Give Claude time to start
						if err := instance.SendPrompt(prompt); err != nil {
							log.ErrorLog.Printf("Failed to send prompt: %v", err)
						}
						return nil
					},
				)
			}

			// Close the overlay and reset state
			m.textInputOverlay = nil
			m.state = stateDefault
			return m, tea.Sequence(
				tea.WindowSize(),
				func() tea.Msg {
					m.menu.SetState(ui.StateDefault)
					return nil
				},
				m.instanceChanged(),
			)
		}

		return m, nil
	}

	// Handle confirmation state
	if m.state == stateConfirm {
		shouldClose := m.confirmationOverlay.HandleKeyPress(msg)
		if shouldClose {
			var actionToExecute tea.Cmd = nil

			// If we have a pending action and it was confirmed (not cancelled)
			if (m.pendingAction != nil || m.pendingActionInfo != nil) && msg.String() == "y" {
				if m.pendingActionInfo != nil {
					actionToExecute = m.pendingActionInfo.action
					// Show loading indicator if needed
					if m.pendingActionInfo.needsLoading {
						m.state = stateLoading
						m.loadingOverlay = overlay.NewLoadingOverlay(m.pendingActionInfo.loadingMessage)
						m.loadingOverlay.SetWidth(50)
						// Start the spinner animation and execute the action
						return m, tea.Batch(m.loadingOverlay.Init(), actionToExecute)
					} else {
						m.state = stateDefault
					}
				} else if m.pendingAction != nil {
					actionToExecute = m.pendingAction
					m.state = stateDefault
				}
			} else {
				m.state = stateDefault
			}

			// Clean up confirmation overlay
			m.confirmationOverlay = nil
			m.pendingAction = nil
			m.pendingActionInfo = nil

			// Execute the action if confirmed
			if actionToExecute != nil {
				return m, actionToExecute
			}

			return m, nil
		}
		return m, nil
	}

	// Handle quit commands first
	if msg.String() == "ctrl+c" || msg.String() == "q" {
		return m.handleQuitConfirmation()
	}

	name, ok := keys.GlobalKeyStringsMap[msg.String()]
	if !ok {
		return m, nil
	}

	switch name {
	case keys.KeyHelp:
		return m.showHelpScreen(helpTypeGeneral, nil)
	case keys.KeyPrompt:
		if m.list.NumInstances() >= GlobalInstanceLimit {
			return m, m.handleError(
				fmt.Errorf("you can't create more than %d instances", GlobalInstanceLimit))
		}
		instance, err := session.NewInstance(session.InstanceOptions{
			Title:   "",
			Path:    ".",
			Program: m.program,
		})
		if err != nil {
			return m, m.handleError(err)
		}

		m.newInstanceFinalizer = m.list.AddInstance(instance)
		m.list.SetSelectedInstance(m.list.NumInstances() - 1)
		m.state = stateNew
		m.menu.SetState(ui.StateNewInstance)
		m.promptAfterName = true

		return m, nil
	case keys.KeyNew:
		if m.list.NumInstances() >= GlobalInstanceLimit {
			return m, m.handleError(
				fmt.Errorf("you can't create more than %d instances", GlobalInstanceLimit))
		}

		// Go to prompt collection state for name generation
		m.state = statePromptForName
		m.menu.SetState(ui.StatePrompt)
		// Initialize the text input overlay for prompt collection
		m.textInputOverlay = overlay.NewTextInputOverlay("Enter prompt for new session", "")

		return m, tea.WindowSize()
	case keys.KeyUp:
		m.list.Up()
		return m, m.instanceChanged()
	case keys.KeyDown:
		m.list.Down()
		return m, m.instanceChanged()
	case keys.KeyShiftUp:
		if m.tabbedWindow.IsInDiffTab() {
			m.tabbedWindow.ScrollUp()
		}
		return m, m.instanceChanged()
	case keys.KeyShiftDown:
		if m.tabbedWindow.IsInDiffTab() {
			m.tabbedWindow.ScrollDown()
		}
		return m, m.instanceChanged()
	case keys.KeyTab:
		m.tabbedWindow.Toggle()
		m.menu.SetInDiffTab(m.tabbedWindow.IsInDiffTab())
		return m, m.instanceChanged()
	case keys.KeyKill:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Create the kill action as a tea.Cmd
		killAction := func() tea.Msg {
			// Get worktree and check if branch is checked out
			worktree, err := selected.GetGitWorktree()
			if err != nil {
				return err
			}

			checkedOut, err := worktree.IsBranchCheckedOut()
			if err != nil {
				return err
			}

			if checkedOut {
				return fmt.Errorf("instance %s is currently checked out", selected.Title)
			}

			// Delete from storage first
			if err := m.storage.DeleteInstance(selected.Title); err != nil {
				return err
			}

			// Then kill the instance
			m.list.Kill()
			return instanceChangedMsg{}
		}

		// Show confirmation modal
		message := fmt.Sprintf("[!] Kill session '%s'?", selected.Title)
		return m, m.confirmAction(message, killAction)
	case keys.KeySubmit:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Create the push action as a tea.Cmd
		pushAction := func() tea.Msg {
			// Default commit message with timestamp
			commitMsg := fmt.Sprintf("[agentfarmer] update from '%s' on %s", selected.Title, time.Now().Format(time.RFC822))
			worktree, err := selected.GetGitWorktree()
			if err != nil {
				return err
			}
			if err = worktree.PushChanges(commitMsg, true); err != nil {
				return err
			}
			return pushCompleteMsg{}
		}

		// Show confirmation modal
		message := fmt.Sprintf("[!] Push changes from session '%s'?", selected.Title)
		return m, m.confirmActionWithLoading(message, pushAction, "Pushing changes...")
	case keys.KeyCheckout:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Show help screen before pausing
		m.showHelpScreen(helpTypeInstanceCheckout, func() {
			if err := selected.Pause(); err != nil {
				m.handleError(err)
			}
			m.instanceChanged()
		})
		return m, nil
	case keys.KeyResume:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		if err := selected.Resume(); err != nil {
			return m, m.handleError(err)
		}
		return m, tea.WindowSize()
	case keys.KeyEnter:
		if m.list.NumInstances() == 0 {
			return m, nil
		}
		selected := m.list.GetSelectedInstance()
		if selected == nil || selected.Paused() || !selected.TmuxAlive() {
			return m, nil
		}
		// Show help screen before attaching
		m.showHelpScreen(helpTypeInstanceAttach, func() {
			ch, err := m.list.Attach()
			if err != nil {
				m.handleError(err)
				return
			}
			<-ch
			m.state = stateDefault
		})
		return m, nil
	case keys.KeyOpenWorktree:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Get the worktree path
		worktree, err := selected.GetGitWorktree()
		if err != nil {
			return m, m.handleError(fmt.Errorf("failed to get worktree: %v", err))
		}

		worktreePath := worktree.GetWorktreePath()
		sessionName := fmt.Sprintf("%s-tree", selected.Title)

		// Show help screen before attaching to worktree session (similar to 'o' key)
		m.showHelpScreen(helpTypeInstanceAttach, func() {
			// Create a tmux session for the worktree using proper TmuxSession infrastructure
			// Use shell as program to ensure it starts properly
			tmuxSession := tmux.NewTmuxSession(sessionName, "$SHELL")

			// Check if session already exists
			if tmuxSession.DoesSessionExist() {
				// Session exists, restore and attach to it
				if err := tmuxSession.Restore(); err != nil {
					m.handleError(fmt.Errorf("failed to restore worktree session: %v", err))
					return
				}
			} else {
				// Create new session in the worktree directory
				if err := tmuxSession.Start(worktreePath); err != nil {
					m.handleError(fmt.Errorf("failed to create worktree session: %v", err))
					return
				}
			}

			// Attach to the session using proper TmuxSession infrastructure
			ch, err := tmuxSession.Attach()
			if err != nil {
				m.handleError(fmt.Errorf("failed to attach to worktree session: %v", err))
				return
			}
			<-ch
			m.state = stateDefault
		})
		return m, nil
	case keys.KeyRebase:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Create the rebase action as a tea.Cmd
		rebaseAction := func() tea.Msg {
			log.DebugLog.Printf("starting rebase for session '%s'", selected.Title)
			worktree, err := selected.GetGitWorktree()
			if err != nil {
				log.ErrorLog.Printf("failed to get git worktree for rebase: %v", err)
				return err
			}
			if err = worktree.RebaseOntoDefault(); err != nil {
				log.ErrorLog.Printf("rebase failed for session '%s': %v", selected.Title, err)
				return err
			}
			log.InfoLog.Printf("rebase completed successfully for session '%s'", selected.Title)
			return rebaseCompleteMsg{}
		}

		// Show confirmation modal
		message := fmt.Sprintf("[!] Rebase session '%s' onto default branch?", selected.Title)
		return m, m.confirmActionWithLoading(message, rebaseAction, "Rebasing onto default branch...")
	default:
		return m, nil
	}
}

// instanceChanged updates the preview pane, menu, and diff pane based on the selected instance. It returns an error
// Cmd if there was any error.
func (m *home) instanceChanged() tea.Cmd {
	// selected may be nil
	selected := m.list.GetSelectedInstance()

	m.tabbedWindow.UpdateDiff(selected)
	// Update menu with current instance
	m.menu.SetInstance(selected)

	// If there's no selected instance, we don't need to update the preview.
	if err := m.tabbedWindow.UpdatePreview(selected); err != nil {
		return m.handleError(err)
	}
	return nil
}

type keyupMsg struct{}

// keydownCallback clears the menu option highlighting after 500ms.
func (m *home) keydownCallback(name keys.KeyName) tea.Cmd {
	m.menu.Keydown(name)
	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
		case <-time.After(500 * time.Millisecond):
		}

		return keyupMsg{}
	}
}

// hideErrMsg implements tea.Msg and clears the error text from the screen.
type hideErrMsg struct{}

// previewTickMsg implements tea.Msg and triggers a preview update
type previewTickMsg struct{}

type tickUpdateMetadataMessage struct{}

type instanceChangedMsg struct{}

// tickUpdateMetadataCmd is the callback to update the metadata of the instances every 500ms. Note that we iterate
// overall the instances and capture their output. It's a pretty expensive operation. Let's do it 2x a second only.
var tickUpdateMetadataCmd = func() tea.Msg {
	time.Sleep(500 * time.Millisecond)
	return tickUpdateMetadataMessage{}
}

// handleError handles all errors which get bubbled up to the app. sets the error message. We return a callback tea.Cmd that returns a hideErrMsg message
// which clears the error message after 3 seconds.
func (m *home) handleError(err error) tea.Cmd {
	log.ErrorLog.Printf("%v", err)
	m.errBox.SetError(err)
	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
		case <-time.After(3 * time.Second):
		}

		return hideErrMsg{}
	}
}

// confirmAction shows a confirmation modal and stores the action to execute on confirm
func (m *home) confirmAction(message string, action tea.Cmd) tea.Cmd {
	m.state = stateConfirm
	m.pendingAction = action
	m.pendingActionInfo = nil // Clear any previous pendingActionInfo

	// Create and show the confirmation overlay using ConfirmationOverlay
	m.confirmationOverlay = overlay.NewConfirmationOverlay(message)
	// Set a fixed width for consistent appearance
	m.confirmationOverlay.SetWidth(50)

	// Set callbacks for confirmation and cancellation
	m.confirmationOverlay.OnConfirm = func() {
		m.state = stateDefault
		// Action will be executed in the handleKeyPress method
	}

	m.confirmationOverlay.OnCancel = func() {
		m.state = stateDefault
		m.pendingAction = nil
	}

	return nil
}

// pendingActionInfo stores information about a pending action including whether it needs loading
type pendingActionInfo struct {
	action         tea.Cmd
	needsLoading   bool
	loadingMessage string
}

// confirmActionWithLoading shows a confirmation modal and stores the action to execute on confirm
// When confirmed, it shows a loading indicator with the specified message
func (m *home) confirmActionWithLoading(message string, action tea.Cmd, loadingMessage string) tea.Cmd {
	m.state = stateConfirm
	m.pendingAction = nil // Clear any previous pendingAction
	m.pendingActionInfo = &pendingActionInfo{
		action:         action,
		needsLoading:   true,
		loadingMessage: loadingMessage,
	}

	// Create and show the confirmation overlay using ConfirmationOverlay
	m.confirmationOverlay = overlay.NewConfirmationOverlay(message)
	// Set a fixed width for consistent appearance
	m.confirmationOverlay.SetWidth(50)

	// Set callbacks for confirmation and cancellation
	m.confirmationOverlay.OnConfirm = func() {
		// State will be set by the confirmation handler
	}

	m.confirmationOverlay.OnCancel = func() {
		m.state = stateDefault
		m.pendingActionInfo = nil
	}

	return nil
}

func (m *home) View() string {
	listWithPadding := lipgloss.NewStyle().PaddingTop(1).Render(m.list.String())
	previewWithPadding := lipgloss.NewStyle().PaddingTop(1).Render(m.tabbedWindow.String())
	listAndPreview := lipgloss.JoinHorizontal(lipgloss.Top, listWithPadding, previewWithPadding)

	mainView := lipgloss.JoinVertical(
		lipgloss.Center,
		listAndPreview,
		m.menu.String(),
		m.errBox.String(),
	)

	if m.state == statePrompt || m.state == statePromptForName {
		if m.textInputOverlay == nil {
			log.ErrorLog.Printf("text input overlay is nil")
		}
		return overlay.PlaceOverlay(0, 0, m.textInputOverlay.Render(), mainView, true, true)
	} else if m.state == stateHelp {
		if m.textOverlay == nil {
			log.ErrorLog.Printf("text overlay is nil")
		}
		return overlay.PlaceOverlay(0, 0, m.textOverlay.Render(), mainView, true, true)
	} else if m.state == stateConfirm {
		if m.confirmationOverlay == nil {
			log.ErrorLog.Printf("confirmation overlay is nil")
		}
		return overlay.PlaceOverlay(0, 0, m.confirmationOverlay.Render(), mainView, true, true)
	} else if m.state == stateLoading {
		if m.loadingOverlay == nil {
			log.ErrorLog.Printf("loading overlay is nil")
		}
		return overlay.PlaceOverlay(0, 0, m.loadingOverlay.Render(), mainView, true, true)
	}

	return mainView
}
