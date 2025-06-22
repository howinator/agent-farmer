package overlay

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// LoadingOverlay represents a loading indicator overlay
type LoadingOverlay struct {
	// Whether the overlay has been dismissed
	Dismissed bool
	// Message to display in the overlay
	message string
	// Width of the overlay
	width int
	// Spinner component for animation
	spinner spinner.Model
	// Custom styling options
	borderColor lipgloss.Color
}

// NewLoadingOverlay creates a new loading overlay with the given message
func NewLoadingOverlay(message string) *LoadingOverlay {
	s := spinner.New()
	s.Spinner = spinner.Points // More visually appealing animated spinner
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#06d6a0"))

	return &LoadingOverlay{
		Dismissed:   false,
		message:     message,
		width:       50, // Default width
		spinner:     s,
		borderColor: lipgloss.Color("#06d6a0"), // Green color for loading
	}
}

// Update updates the spinner animation
func (l *LoadingOverlay) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	l.spinner, cmd = l.spinner.Update(msg)
	return cmd
}

// Dismiss dismisses the loading overlay
func (l *LoadingOverlay) Dismiss() {
	l.Dismissed = true
}

// Render renders the loading overlay
func (l *LoadingOverlay) Render(opts ...WhitespaceOption) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(l.borderColor).
		Padding(1, 2).
		Width(l.width)

	// Render spinner with message
	content := l.spinner.View() + " " + l.message

	// Apply the border style and return
	return style.Render(content)
}

// SetWidth sets the width of the loading overlay
func (l *LoadingOverlay) SetWidth(width int) {
	l.width = width
}

// SetBorderColor sets the border color of the loading overlay
func (l *LoadingOverlay) SetBorderColor(color lipgloss.Color) {
	l.borderColor = color
}

// SetMessage sets the message of the loading overlay
func (l *LoadingOverlay) SetMessage(message string) {
	l.message = message
}
