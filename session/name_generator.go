package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// NameGeneratorConfig holds configuration for name generation
type NameGeneratorConfig struct {
	AnthropicAPIKey string
	OpenAIAPIKey    string
	MaxRetries      int
	MaxLength       int
}

// NewNameGeneratorConfig creates a new config with default values
func NewNameGeneratorConfig() *NameGeneratorConfig {
	return &NameGeneratorConfig{
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		OpenAIAPIKey:    os.Getenv("OPENAI_API_KEY"),
		MaxRetries:      3,
		MaxLength:       32,
	}
}

// AnthropicRequest represents the request structure for Anthropic API
type AnthropicRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	Messages  []Message `json:"messages"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AnthropicResponse represents the response structure from Anthropic API
type AnthropicResponse struct {
	Content []Content `json:"content"`
}

type Content struct {
	Text string `json:"text"`
	Type string `json:"type"`
}

// OpenAIRequest represents the request structure for OpenAI API
type OpenAIRequest struct {
	Model     string       `json:"model"`
	Messages  []OAIMessage `json:"messages"`
	MaxTokens int          `json:"max_tokens"`
}

type OAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenAIResponse represents the response structure from OpenAI API
type OpenAIResponse struct {
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Message OAIMessage `json:"message"`
}

// GenerateSessionName generates a session name based on the given prompt
func GenerateSessionName(prompt string, config *NameGeneratorConfig) (string, error) {
	if config == nil {
		config = NewNameGeneratorConfig()
	}

	// Check if we have any API keys available
	if config.AnthropicAPIKey == "" && config.OpenAIAPIKey == "" {
		// Fallback to simple rule-based name generation
		return generateFallbackName(prompt, config), nil
	}

	// Try generating name with retries for length constraint
	for attempt := 0; attempt < config.MaxRetries; attempt++ {
		var name string
		var err error

		// Try Anthropic first if API key is available
		if config.AnthropicAPIKey != "" {
			name, err = generateWithAnthropic(prompt, config)
		} else if config.OpenAIAPIKey != "" {
			name, err = generateWithOpenAI(prompt, config)
		}

		if err != nil {
			// If API fails, fall back to rule-based generation
			return generateFallbackName(prompt, config), nil
		}

		// Clean and validate the name
		cleanName := cleanSessionName(name)
		if len(cleanName) <= config.MaxLength && len(cleanName) > 0 {
			return cleanName, nil
		}

		// If name is too long, we'll retry with more specific instructions
	}

	// If all API attempts fail, use fallback
	return generateFallbackName(prompt, config), nil
}

// generateWithAnthropic calls the Anthropic API to generate a name
func generateWithAnthropic(prompt string, config *NameGeneratorConfig) (string, error) {
	systemPrompt := buildSystemPrompt(prompt)

	reqBody := AnthropicRequest{
		Model:     "claude-3-haiku-20240307",
		MaxTokens: 50,
		Messages: []Message{
			{
				Role:    "user",
				Content: systemPrompt,
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
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var anthropicResp AnthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthropicResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(anthropicResp.Content) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	return strings.TrimSpace(anthropicResp.Content[0].Text), nil
}

// generateWithOpenAI calls the OpenAI API to generate a name
func generateWithOpenAI(prompt string, config *NameGeneratorConfig) (string, error) {
	systemPrompt := buildSystemPrompt(prompt)

	reqBody := OpenAIRequest{
		Model: "gpt-3.5-turbo",
		Messages: []OAIMessage{
			{
				Role:    "user",
				Content: systemPrompt,
			},
		},
		MaxTokens: 50,
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
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var openaiResp OpenAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&openaiResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(openaiResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return strings.TrimSpace(openaiResp.Choices[0].Message.Content), nil
}

// buildSystemPrompt creates the system prompt for name generation
func buildSystemPrompt(userPrompt string) string {
	// Check if prompt contains ticket numbers
	ticketRegex := regexp.MustCompile(`(?i)(?:ticket|issue|bug|task|story)[\s#-]*(\w+[-\w]*\d+|\d+[-\w]*\w*|\d+)`)
	ticketMatches := ticketRegex.FindStringSubmatch(userPrompt)

	basePrompt := `Generate a concise session name for this coding task. The name must be under 32 characters and use hyphens between words (no spaces). Make it descriptive but brief.`

	if len(ticketMatches) > 1 {
		// Extract ticket number
		ticketNum := ticketMatches[1]
		basePrompt += fmt.Sprintf(` If there's a ticket number (%s), use the format: %s-keyword (e.g., %s-auth, %s-fix).`, ticketNum, ticketNum, ticketNum, ticketNum)
	} else {
		basePrompt += ` Use format: keyword (e.g., auth-fix, add-validation, refactor-api).`
	}

	basePrompt += `

Task: ` + userPrompt + `

Return only the session name, nothing else.`

	return basePrompt
}

// cleanSessionName cleans and validates the generated session name
func cleanSessionName(name string) string {
	// Remove quotes and extra whitespace
	name = strings.Trim(name, `"' `)

	// Replace spaces with hyphens
	name = strings.ReplaceAll(name, " ", "-")

	// Remove any characters that aren't alphanumeric, hyphens, or underscores
	reg := regexp.MustCompile(`[^a-zA-Z0-9\-_]`)
	name = reg.ReplaceAllString(name, "")

	// Remove multiple consecutive hyphens
	hyphenReg := regexp.MustCompile(`-+`)
	name = hyphenReg.ReplaceAllString(name, "-")

	// Trim hyphens from start and end
	name = strings.Trim(name, "-_")

	return name
}

// generateFallbackName creates a name using simple rule-based logic when API is unavailable
func generateFallbackName(prompt string, config *NameGeneratorConfig) string {
	// Check if prompt contains ticket numbers
	ticketRegex := regexp.MustCompile(`(?i)(?:ticket|issue|bug|task|story)[\s#-]*(\w+[-\w]*\d+|\d+[-\w]*\w*|\d+)`)
	ticketMatches := ticketRegex.FindStringSubmatch(prompt)

	// Extract keywords from the prompt
	words := strings.Fields(strings.ToLower(prompt))
	var keywords []string

	// Common coding action words
	actionWords := map[string]bool{
		"fix": true, "add": true, "update": true, "remove": true, "delete": true,
		"create": true, "implement": true, "refactor": true, "optimize": true,
		"debug": true, "test": true, "validate": true, "auth": true, "login": true,
		"api": true, "bug": true, "feature": true, "enhance": true, "improve": true,
	}

	// Extract meaningful keywords
	for _, word := range words {
		cleanWord := regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(word, "")
		if len(cleanWord) > 2 && (actionWords[cleanWord] || len(cleanWord) > 4) {
			keywords = append(keywords, cleanWord)
			if len(keywords) >= 3 { // Limit to 3 keywords to keep name short
				break
			}
		}
	}

	var name string
	if len(ticketMatches) > 1 {
		// If there's a ticket number, use it as prefix
		ticketNum := ticketMatches[1]
		if len(keywords) > 0 {
			name = fmt.Sprintf("%s-%s", ticketNum, strings.Join(keywords, "-"))
		} else {
			name = fmt.Sprintf("%s-task", ticketNum)
		}
	} else if len(keywords) > 0 {
		// Use keywords
		name = strings.Join(keywords, "-")
	} else {
		// Last resort: use timestamp-based name
		name = fmt.Sprintf("session-%d", time.Now().Unix()%10000)
	}

	// Clean and ensure it fits within length constraints
	cleanName := cleanSessionName(name)
	if len(cleanName) > config.MaxLength {
		// Truncate to fit
		cleanName = cleanName[:config.MaxLength]
		// Remove trailing hyphens after truncation
		cleanName = strings.TrimRight(cleanName, "-")
	}

	return cleanName
}
