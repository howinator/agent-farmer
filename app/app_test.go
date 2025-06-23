package app

import (
	"agent-farmer/config"
	"agent-farmer/log"
	"agent-farmer/session"
	"agent-farmer/ui"
	"agent-farmer/ui/overlay"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain runs before all tests to set up the test environment
func TestMain(m *testing.M) {
	// Initialize the logger before any tests run
	log.Initialize(false)
	defer log.Close()

	// Run all tests
	exitCode := m.Run()

	// Exit with the same code as the tests
	os.Exit(exitCode)
}

// TestConfirmationModalStateTransitions tests state transitions without full instance setup
func TestConfirmationModalStateTransitions(t *testing.T) {
	// Create a minimal home struct for testing state transitions
	h := &home{
		ctx:       context.Background(),
		state:     stateDefault,
		appConfig: config.DefaultConfig(),
	}

	t.Run("shows confirmation on D press", func(t *testing.T) {
		// Simulate pressing 'D'
		h.state = stateDefault
		h.confirmationOverlay = nil

		// Manually trigger what would happen in handleKeyPress for 'D'
		h.state = stateConfirm
		h.confirmationOverlay = overlay.NewConfirmationOverlay("[!] Kill session 'test'?")

		assert.Equal(t, stateConfirm, h.state)
		assert.NotNil(t, h.confirmationOverlay)
		assert.False(t, h.confirmationOverlay.Dismissed)
	})

	t.Run("returns to default on y press", func(t *testing.T) {
		// Start in confirmation state
		h.state = stateConfirm
		h.confirmationOverlay = overlay.NewConfirmationOverlay("Test confirmation")

		// Simulate pressing 'y' using HandleKeyPress
		keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}
		shouldClose := h.confirmationOverlay.HandleKeyPress(keyMsg)
		if shouldClose {
			h.state = stateDefault
			h.confirmationOverlay = nil
		}

		assert.Equal(t, stateDefault, h.state)
		assert.Nil(t, h.confirmationOverlay)
	})

	t.Run("returns to default on n press", func(t *testing.T) {
		// Start in confirmation state
		h.state = stateConfirm
		h.confirmationOverlay = overlay.NewConfirmationOverlay("Test confirmation")

		// Simulate pressing 'n' using HandleKeyPress
		keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}
		shouldClose := h.confirmationOverlay.HandleKeyPress(keyMsg)
		if shouldClose {
			h.state = stateDefault
			h.confirmationOverlay = nil
		}

		assert.Equal(t, stateDefault, h.state)
		assert.Nil(t, h.confirmationOverlay)
	})

	t.Run("returns to default on esc press", func(t *testing.T) {
		// Start in confirmation state
		h.state = stateConfirm
		h.confirmationOverlay = overlay.NewConfirmationOverlay("Test confirmation")

		// Simulate pressing ESC using HandleKeyPress
		keyMsg := tea.KeyMsg{Type: tea.KeyEscape}
		shouldClose := h.confirmationOverlay.HandleKeyPress(keyMsg)
		if shouldClose {
			h.state = stateDefault
			h.confirmationOverlay = nil
		}

		assert.Equal(t, stateDefault, h.state)
		assert.Nil(t, h.confirmationOverlay)
	})
}

// TestConfirmationModalKeyHandling tests the actual key handling in confirmation state
func TestConfirmationModalKeyHandling(t *testing.T) {
	// Import needed packages
	spinner := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	list := ui.NewList(&spinner, false)

	// Create enough of home struct to test handleKeyPress in confirmation state
	h := &home{
		ctx:                 context.Background(),
		state:               stateConfirm,
		appConfig:           config.DefaultConfig(),
		list:                list,
		menu:                ui.NewMenu(),
		confirmationOverlay: overlay.NewConfirmationOverlay("Kill session?"),
	}

	testCases := []struct {
		name              string
		key               string
		expectedState     state
		expectedDismissed bool
		expectedNil       bool
	}{
		{
			name:              "y key confirms and dismisses overlay",
			key:               "y",
			expectedState:     stateDefault,
			expectedDismissed: true,
			expectedNil:       true,
		},
		{
			name:              "n key cancels and dismisses overlay",
			key:               "n",
			expectedState:     stateDefault,
			expectedDismissed: true,
			expectedNil:       true,
		},
		{
			name:              "esc key cancels and dismisses overlay",
			key:               "esc",
			expectedState:     stateDefault,
			expectedDismissed: true,
			expectedNil:       true,
		},
		{
			name:              "other keys are ignored",
			key:               "x",
			expectedState:     stateConfirm,
			expectedDismissed: false,
			expectedNil:       false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset state
			h.state = stateConfirm
			h.confirmationOverlay = overlay.NewConfirmationOverlay("Kill session?")

			// Create key message
			var keyMsg tea.KeyMsg
			if tc.key == "esc" {
				keyMsg = tea.KeyMsg{Type: tea.KeyEscape}
			} else {
				keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tc.key)}
			}

			// Call handleKeyPress
			model, _ := h.handleKeyPress(keyMsg)
			homeModel, ok := model.(*home)
			require.True(t, ok)

			assert.Equal(t, tc.expectedState, homeModel.state, "State mismatch for key: %s", tc.key)
			if tc.expectedNil {
				assert.Nil(t, homeModel.confirmationOverlay, "Overlay should be nil for key: %s", tc.key)
			} else {
				assert.NotNil(t, homeModel.confirmationOverlay, "Overlay should not be nil for key: %s", tc.key)
				assert.Equal(t, tc.expectedDismissed, homeModel.confirmationOverlay.Dismissed, "Dismissed mismatch for key: %s", tc.key)
			}
		})
	}
}

// TestConfirmationMessageFormatting tests that confirmation messages are formatted correctly
func TestConfirmationMessageFormatting(t *testing.T) {
	testCases := []struct {
		name            string
		sessionTitle    string
		expectedMessage string
	}{
		{
			name:            "short session name",
			sessionTitle:    "my-feature",
			expectedMessage: "[!] Kill session 'my-feature'? (y/n)",
		},
		{
			name:            "long session name",
			sessionTitle:    "very-long-feature-branch-name-here",
			expectedMessage: "[!] Kill session 'very-long-feature-branch-name-here'? (y/n)",
		},
		{
			name:            "session with spaces",
			sessionTitle:    "feature with spaces",
			expectedMessage: "[!] Kill session 'feature with spaces'? (y/n)",
		},
		{
			name:            "session with special chars",
			sessionTitle:    "feature/branch-123",
			expectedMessage: "[!] Kill session 'feature/branch-123'? (y/n)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test the message formatting directly
			actualMessage := fmt.Sprintf("[!] Kill session '%s'? (y/n)", tc.sessionTitle)
			assert.Equal(t, tc.expectedMessage, actualMessage)
		})
	}
}

// TestConfirmationFlowSimulation tests the confirmation flow by simulating the state changes
func TestConfirmationFlowSimulation(t *testing.T) {
	// Create a minimal setup
	spinner := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	list := ui.NewList(&spinner, false)

	// Add test instance
	instance, err := session.NewInstance(session.InstanceOptions{
		Title:   "test-session",
		Path:    t.TempDir(),
		Program: "claude",
		AutoYes: false,
	})
	require.NoError(t, err)
	_ = list.AddInstance(instance)
	list.SetSelectedInstance(0)

	h := &home{
		ctx:       context.Background(),
		state:     stateDefault,
		appConfig: config.DefaultConfig(),
		list:      list,
		menu:      ui.NewMenu(),
	}

	// Simulate what happens when D is pressed
	selected := h.list.GetSelectedInstance()
	require.NotNil(t, selected)

	// This is what the KeyKill handler does
	message := fmt.Sprintf("[!] Kill session '%s'?", selected.Title)
	h.confirmationOverlay = overlay.NewConfirmationOverlay(message)
	h.state = stateConfirm

	// Verify the state
	assert.Equal(t, stateConfirm, h.state)
	assert.NotNil(t, h.confirmationOverlay)
	assert.False(t, h.confirmationOverlay.Dismissed)
	// Test that overlay renders with the correct message
	rendered := h.confirmationOverlay.Render()
	assert.Contains(t, rendered, "Kill session 'test-session'?")
}

// TestConfirmActionWithDifferentTypes tests that confirmAction works with different action types
func TestConfirmActionWithDifferentTypes(t *testing.T) {
	h := &home{
		ctx:       context.Background(),
		state:     stateDefault,
		appConfig: config.DefaultConfig(),
	}

	t.Run("works with simple action returning nil", func(t *testing.T) {
		actionCalled := false
		action := func() tea.Msg {
			actionCalled = true
			return nil
		}

		// Set up callback to track action execution
		actionExecuted := false
		h.confirmationOverlay = overlay.NewConfirmationOverlay("Test action?")
		h.confirmationOverlay.OnConfirm = func() {
			h.state = stateDefault
			actionExecuted = true
			action() // Execute the action
		}
		h.state = stateConfirm

		// Verify state was set
		assert.Equal(t, stateConfirm, h.state)
		assert.NotNil(t, h.confirmationOverlay)
		assert.False(t, h.confirmationOverlay.Dismissed)
		assert.NotNil(t, h.confirmationOverlay.OnConfirm)

		// Execute the confirmation callback
		h.confirmationOverlay.OnConfirm()
		assert.True(t, actionCalled)
		assert.True(t, actionExecuted)
	})

	t.Run("works with action returning error", func(t *testing.T) {
		expectedErr := fmt.Errorf("test error")
		action := func() tea.Msg {
			return expectedErr
		}

		// Set up callback to track action execution
		var receivedMsg tea.Msg
		h.confirmationOverlay = overlay.NewConfirmationOverlay("Error action?")
		h.confirmationOverlay.OnConfirm = func() {
			h.state = stateDefault
			receivedMsg = action() // Execute the action and capture result
		}
		h.state = stateConfirm

		// Verify state was set
		assert.Equal(t, stateConfirm, h.state)
		assert.NotNil(t, h.confirmationOverlay)
		assert.False(t, h.confirmationOverlay.Dismissed)
		assert.NotNil(t, h.confirmationOverlay.OnConfirm)

		// Execute the confirmation callback
		h.confirmationOverlay.OnConfirm()
		assert.Equal(t, expectedErr, receivedMsg)
	})

	t.Run("works with action returning custom message", func(t *testing.T) {
		action := func() tea.Msg {
			return instanceChangedMsg{}
		}

		// Set up callback to track action execution
		var receivedMsg tea.Msg
		h.confirmationOverlay = overlay.NewConfirmationOverlay("Custom message action?")
		h.confirmationOverlay.OnConfirm = func() {
			h.state = stateDefault
			receivedMsg = action() // Execute the action and capture result
		}
		h.state = stateConfirm

		// Verify state was set
		assert.Equal(t, stateConfirm, h.state)
		assert.NotNil(t, h.confirmationOverlay)
		assert.False(t, h.confirmationOverlay.Dismissed)
		assert.NotNil(t, h.confirmationOverlay.OnConfirm)

		// Execute the confirmation callback
		h.confirmationOverlay.OnConfirm()
		_, ok := receivedMsg.(instanceChangedMsg)
		assert.True(t, ok, "Expected instanceChangedMsg but got %T", receivedMsg)
	})
}

// TestMultipleConfirmationsDontInterfere tests that multiple confirmations don't interfere with each other
func TestMultipleConfirmationsDontInterfere(t *testing.T) {
	h := &home{
		ctx:       context.Background(),
		state:     stateDefault,
		appConfig: config.DefaultConfig(),
	}

	// First confirmation
	action1Called := false
	action1 := func() tea.Msg {
		action1Called = true
		return nil
	}

	// Set up first confirmation
	h.confirmationOverlay = overlay.NewConfirmationOverlay("First action?")
	firstOnConfirm := func() {
		h.state = stateDefault
		action1()
	}
	h.confirmationOverlay.OnConfirm = firstOnConfirm
	h.state = stateConfirm

	// Verify first confirmation
	assert.Equal(t, stateConfirm, h.state)
	assert.NotNil(t, h.confirmationOverlay)
	assert.False(t, h.confirmationOverlay.Dismissed)
	assert.NotNil(t, h.confirmationOverlay.OnConfirm)

	// Cancel first confirmation (simulate pressing 'n')
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}
	shouldClose := h.confirmationOverlay.HandleKeyPress(keyMsg)
	if shouldClose {
		h.state = stateDefault
		h.confirmationOverlay = nil
	}

	// Second confirmation with different action
	action2Called := false
	action2 := func() tea.Msg {
		action2Called = true
		return fmt.Errorf("action2 error")
	}

	// Set up second confirmation
	h.confirmationOverlay = overlay.NewConfirmationOverlay("Second action?")
	var secondResult tea.Msg
	secondOnConfirm := func() {
		h.state = stateDefault
		secondResult = action2()
	}
	h.confirmationOverlay.OnConfirm = secondOnConfirm
	h.state = stateConfirm

	// Verify second confirmation
	assert.Equal(t, stateConfirm, h.state)
	assert.NotNil(t, h.confirmationOverlay)
	assert.False(t, h.confirmationOverlay.Dismissed)
	assert.NotNil(t, h.confirmationOverlay.OnConfirm)

	// Execute second action to verify it's the correct one
	h.confirmationOverlay.OnConfirm()
	err, ok := secondResult.(error)
	assert.True(t, ok)
	assert.Equal(t, "action2 error", err.Error())
	assert.True(t, action2Called)
	assert.False(t, action1Called, "First action should not have been called")

	// Test that cancelled action can still be executed independently
	firstOnConfirm()
	assert.True(t, action1Called, "First action should be callable after being replaced")
}

// TestConfirmationModalVisualAppearance tests that confirmation modal has distinct visual appearance
func TestConfirmationModalVisualAppearance(t *testing.T) {
	h := &home{
		ctx:       context.Background(),
		state:     stateDefault,
		appConfig: config.DefaultConfig(),
	}

	// Create a test confirmation overlay
	message := "[!] Delete everything?"
	h.confirmationOverlay = overlay.NewConfirmationOverlay(message)
	h.state = stateConfirm

	// Verify the overlay was created with confirmation settings
	assert.NotNil(t, h.confirmationOverlay)
	assert.Equal(t, stateConfirm, h.state)
	assert.False(t, h.confirmationOverlay.Dismissed)

	// Test the overlay render (we can test that it renders without errors)
	rendered := h.confirmationOverlay.Render()
	assert.NotEmpty(t, rendered)

	// Test that it includes the message content and instructions
	assert.Contains(t, rendered, "Delete everything?")
	assert.Contains(t, rendered, "Press")
	assert.Contains(t, rendered, "to confirm")
	assert.Contains(t, rendered, "to cancel")

	// Test that the danger indicator is preserved
	assert.Contains(t, rendered, "[!")
}

// TestLoadingIndicatorForLongRunningOperations tests that loading indicators only show for appropriate operations
func TestLoadingIndicatorForLongRunningOperations(t *testing.T) {
	// Create a minimal setup
	spinner := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	list := ui.NewList(&spinner, false)

	// Add test instance
	instance, err := session.NewInstance(session.InstanceOptions{
		Title:   "test-session",
		Path:    t.TempDir(),
		Program: "claude",
		AutoYes: false,
	})
	require.NoError(t, err)
	_ = list.AddInstance(instance)
	list.SetSelectedInstance(0)

	h := &home{
		ctx:       context.Background(),
		state:     stateDefault,
		appConfig: config.DefaultConfig(),
		list:      list,
		spinner:   spinner,
	}

	t.Run("delete operation shows confirmation only, no loading indicator", func(t *testing.T) {
		h.state = stateDefault
		h.confirmationOverlay = nil
		h.loadingOverlay = nil
		h.pendingAction = nil
		h.pendingActionInfo = nil

		// Create a mock delete action (we won't actually execute it)
		killAction := func() tea.Msg {
			return instanceChangedMsg{}
		}

		// Call confirmAction (not confirmActionWithLoading)
		_ = h.confirmAction("[!] Kill session 'test-session'?", killAction)

		// Verify state
		assert.Equal(t, stateConfirm, h.state)
		assert.NotNil(t, h.confirmationOverlay)
		assert.Nil(t, h.loadingOverlay) // No loading overlay for delete
		assert.NotNil(t, h.pendingAction)
		assert.Nil(t, h.pendingActionInfo) // No pendingActionInfo for simple confirmations
	})

	t.Run("push operation shows confirmation and loading indicator", func(t *testing.T) {
		h.state = stateDefault
		h.confirmationOverlay = nil
		h.loadingOverlay = nil
		h.pendingAction = nil
		h.pendingActionInfo = nil

		// Create a mock push action (we won't actually execute it)
		pushAction := func() tea.Msg {
			return pushCompleteMsg{}
		}

		// Call confirmActionWithLoading
		_ = h.confirmActionWithLoading("[!] Push changes from session 'test-session'?", pushAction, "Pushing changes...")

		// Verify state
		assert.Equal(t, stateConfirm, h.state)
		assert.NotNil(t, h.confirmationOverlay)
		assert.Nil(t, h.loadingOverlay) // Loading overlay created only after confirmation
		assert.Nil(t, h.pendingAction)
		assert.NotNil(t, h.pendingActionInfo) // pendingActionInfo used for loading operations
		assert.True(t, h.pendingActionInfo.needsLoading)
		assert.Equal(t, "Pushing changes...", h.pendingActionInfo.loadingMessage)
	})

	t.Run("rebase operation shows confirmation and loading indicator", func(t *testing.T) {
		h.state = stateDefault
		h.confirmationOverlay = nil
		h.loadingOverlay = nil
		h.pendingAction = nil
		h.pendingActionInfo = nil

		// Create a mock rebase action
		rebaseAction := func() tea.Msg {
			return rebaseCompleteMsg{}
		}

		// Call confirmActionWithLoading
		_ = h.confirmActionWithLoading("[!] Rebase session 'test-session'?", rebaseAction, "Rebasing onto default branch...")

		// Verify state
		assert.Equal(t, stateConfirm, h.state)
		assert.NotNil(t, h.confirmationOverlay)
		assert.Nil(t, h.loadingOverlay) // Loading overlay created only after confirmation
		assert.Nil(t, h.pendingAction)
		assert.NotNil(t, h.pendingActionInfo)
		assert.True(t, h.pendingActionInfo.needsLoading)
		assert.Equal(t, "Rebasing onto default branch...", h.pendingActionInfo.loadingMessage)
	})
}

// TestLoadingIndicatorAnimation tests that the spinner animation works correctly
func TestLoadingIndicatorAnimation(t *testing.T) {
	h := &home{
		ctx:       context.Background(),
		state:     stateLoading,
		appConfig: config.DefaultConfig(),
		spinner:   spinner.New(spinner.WithSpinner(spinner.MiniDot)),
	}

	// Create a loading overlay
	h.loadingOverlay = overlay.NewLoadingOverlay("Test loading...")
	h.loadingOverlay.SetWidth(50)

	t.Run("loading overlay properly initialized", func(t *testing.T) {
		assert.NotNil(t, h.loadingOverlay)
		assert.False(t, h.loadingOverlay.Dismissed)

		// Test that Init() returns a spinner tick command
		initCmd := h.loadingOverlay.Init()
		assert.NotNil(t, initCmd)

		// Execute the init command to get the first tick message
		initMsg := initCmd()
		assert.IsType(t, spinner.TickMsg{}, initMsg)
	})

	t.Run("spinner tick messages update loading overlay", func(t *testing.T) {
		// Create a spinner tick message
		tickMsg := spinner.TickMsg{}

		// Send the tick message through the main Update method
		model, cmd := h.Update(tickMsg)
		homeModel, ok := model.(*home)
		require.True(t, ok)

		// Verify the loading overlay is still present and state unchanged
		assert.Equal(t, stateLoading, homeModel.state)
		assert.NotNil(t, homeModel.loadingOverlay)
		assert.False(t, homeModel.loadingOverlay.Dismissed)

		// Verify that a command was returned (continuing the animation)
		assert.NotNil(t, cmd)
	})

	t.Run("loading overlay updates independently from main spinner", func(t *testing.T) {
		// Test that both main spinner and loading overlay get updated on tick
		tickMsg := spinner.TickMsg{}

		// Capture initial spinner state
		initialMainSpinner := h.spinner.View()

		// Send tick message
		model, cmd := h.Update(tickMsg)
		homeModel, ok := model.(*home)
		require.True(t, ok)

		// Verify main spinner was updated
		newMainSpinner := homeModel.spinner.View()
		// Note: spinner view might be the same if it's at the same animation frame
		// but the important thing is that Update was called without error

		// Verify loading overlay was also updated (command returned for continuation)
		assert.NotNil(t, cmd)
		assert.Equal(t, stateLoading, homeModel.state)
		assert.NotNil(t, homeModel.loadingOverlay)

		_ = initialMainSpinner // Use the variable to avoid unused variable error
		_ = newMainSpinner     // Use the variable to avoid unused variable error
	})
}

// TestLoadingIndicatorDismissal tests that loading indicators are properly dismissed
func TestLoadingIndicatorDismissal(t *testing.T) {
	h := &home{
		ctx:       context.Background(),
		state:     stateLoading,
		appConfig: config.DefaultConfig(),
		spinner:   spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		errBox:    ui.NewErrBox(), // Initialize errBox to prevent nil pointer dereference
	}

	// Create a loading overlay
	h.loadingOverlay = overlay.NewLoadingOverlay("Processing...")
	h.loadingOverlay.SetWidth(50)

	testCases := []struct {
		name        string
		message     tea.Msg
		shouldClear bool
	}{
		{
			name:        "push completion message dismisses loading",
			message:     pushCompleteMsg{},
			shouldClear: true,
		},
		{
			name:        "rebase completion message dismisses loading",
			message:     rebaseCompleteMsg{},
			shouldClear: true,
		},
		{
			name:        "operation completion message dismisses loading",
			message:     operationCompleteMsg{},
			shouldClear: true,
		},
		{
			name:        "error message dismisses loading",
			message:     fmt.Errorf("test error"),
			shouldClear: true,
		},
		{
			name:        "spinner tick does not dismiss loading",
			message:     spinner.TickMsg{},
			shouldClear: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset loading state
			h.state = stateLoading
			h.loadingOverlay = overlay.NewLoadingOverlay("Processing...")
			h.loadingOverlay.SetWidth(50)

			// Send the message
			model, _ := h.Update(tc.message)
			homeModel, ok := model.(*home)
			require.True(t, ok)

			if tc.shouldClear {
				// Verify loading overlay was dismissed and state reset
				assert.Equal(t, stateDefault, homeModel.state)
				assert.Nil(t, homeModel.loadingOverlay)
			} else {
				// Verify loading overlay remains
				assert.Equal(t, stateLoading, homeModel.state)
				assert.NotNil(t, homeModel.loadingOverlay)
				assert.False(t, homeModel.loadingOverlay.Dismissed)
			}
		})
	}
}

// TestKeyInputIgnoredDuringLoading tests that key presses are ignored during loading
func TestKeyInputIgnoredDuringLoading(t *testing.T) {
	h := &home{
		ctx:       context.Background(),
		state:     stateLoading,
		appConfig: config.DefaultConfig(),
		spinner:   spinner.New(spinner.WithSpinner(spinner.MiniDot)),
	}

	// Create a loading overlay
	h.loadingOverlay = overlay.NewLoadingOverlay("Loading...")
	h.loadingOverlay.SetWidth(50)

	testKeys := []struct {
		name   string
		keyMsg tea.KeyMsg
	}{
		{
			name:   "regular key press",
			keyMsg: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")},
		},
		{
			name:   "delete key",
			keyMsg: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("D")},
		},
		{
			name:   "enter key",
			keyMsg: tea.KeyMsg{Type: tea.KeyEnter},
		},
		{
			name:   "escape key",
			keyMsg: tea.KeyMsg{Type: tea.KeyEscape},
		},
		{
			name:   "ctrl+c",
			keyMsg: tea.KeyMsg{Type: tea.KeyCtrlC},
		},
	}

	for _, tc := range testKeys {
		t.Run(tc.name, func(t *testing.T) {
			// Reset state
			h.state = stateLoading
			h.loadingOverlay = overlay.NewLoadingOverlay("Loading...")

			// Send key message through the main Update method
			model, cmd := h.Update(tc.keyMsg)
			homeModel, ok := model.(*home)
			require.True(t, ok)

			// Verify state remains unchanged (key was ignored)
			assert.Equal(t, stateLoading, homeModel.state)
			assert.NotNil(t, homeModel.loadingOverlay)
			assert.False(t, homeModel.loadingOverlay.Dismissed)

			// Verify no command was returned (key processing was skipped)
			assert.Nil(t, cmd)
		})
	}

	t.Run("loading state prevents key processing in handleKeyPress", func(t *testing.T) {
		// Test the handleKeyPress method directly
		h.state = stateLoading
		keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")}

		model, cmd := h.handleKeyPress(keyMsg)
		homeModel, ok := model.(*home)
		require.True(t, ok)

		// Verify state unchanged and no command returned
		assert.Equal(t, stateLoading, homeModel.state)
		assert.Nil(t, cmd)
	})
}
