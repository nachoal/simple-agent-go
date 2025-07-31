package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nachoal/simple-agent-go/llm"
)

// ModelItem represents a model in the list
type ModelItem struct {
	Provider    string
	Model       llm.Model
	DisplayName string
}

func (i ModelItem) Title() string       { return i.DisplayName }
func (i ModelItem) Description() string { return i.Model.Description }
func (i ModelItem) FilterValue() string { return i.DisplayName }

// ModelSelector is a component for selecting models
type ModelSelector struct {
	list          list.Model
	providers     map[string]llm.Client
	selected      ModelItem
	loading       bool
	err           error
	width         int
	height        int
	onSelect      func(provider, model string) tea.Cmd
}

// NewModelSelector creates a new model selector
func NewModelSelector(providers map[string]llm.Client, onSelect func(provider, model string) tea.Cmd) *ModelSelector {
	// Create list with custom styles
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color("170")).
		BorderLeftForeground(lipgloss.Color("170"))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color("170")).
		BorderLeftForeground(lipgloss.Color("170"))

	l := list.New([]list.Item{}, delegate, 80, 20) // Default size
	l.Title = "Select a Model"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)
	l.Styles.Title = lipgloss.NewStyle().
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("230")).
		Padding(0, 1)

	return &ModelSelector{
		list:      l,
		providers: providers,
		loading:   true,
		onSelect:  onSelect,
		width:     80,  // Default width
		height:    20,  // Default height
	}
}

func (m *ModelSelector) Init() tea.Cmd {
	return m.loadModels()
}

func (m *ModelSelector) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "enter":
			if i, ok := m.list.SelectedItem().(ModelItem); ok {
				m.selected = i
				if m.onSelect != nil {
					return m, m.onSelect(i.Provider, i.Model.ID)
				}
			}
		}

	case modelsLoadedMsg:
		items := make([]list.Item, 0)
		
		// Group models by provider
		providerModels := make(map[string][]llm.Model)
		for provider, models := range msg.models {
			if len(models) > 0 {
				providerModels[provider] = models
			}
		}

		// Sort providers for consistent display
		providers := make([]string, 0, len(providerModels))
		for p := range providerModels {
			providers = append(providers, p)
		}
		sort.Strings(providers)

		// Add models grouped by provider
		for _, provider := range providers {
			models := providerModels[provider]
			// Sort models by ID for consistency
			sort.Slice(models, func(i, j int) bool {
				return models[i].ID < models[j].ID
			})

			for _, model := range models {
				displayName := fmt.Sprintf("[%s] %s", provider, model.ID)
				items = append(items, ModelItem{
					Provider:    provider,
					Model:       model,
					DisplayName: displayName,
				})
			}
		}

		// If no models were found, show an error
		if len(items) == 0 {
			m.err = fmt.Errorf("no models found - check your API keys")
			m.loading = false
			return m, nil
		}

		m.list.SetItems(items)
		m.loading = false
		return m, nil

	case errMsg:
		m.err = msg.err
		m.loading = false
		return m, nil
	}

	// Update the list
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *ModelSelector) View() string {
	if m.loading {
		return lipgloss.NewStyle().
			Width(m.width).
			Height(m.height).
			Align(lipgloss.Center, lipgloss.Center).
			Render("Loading models...")
	}

	if m.err != nil {
		return lipgloss.NewStyle().
			Width(m.width).
			Height(m.height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(lipgloss.Color("9")).
			Render(fmt.Sprintf("Error loading models: %v", m.err))
	}

	return m.list.View()
}

// loadModels fetches models from all providers concurrently
func (m *ModelSelector) loadModels() tea.Cmd {
	return func() tea.Msg {
		// Check if we have any providers
		if len(m.providers) == 0 {
			return errMsg{err: fmt.Errorf("no providers available")}
		}

		ctx := context.Background()
		results := make(map[string][]llm.Model)
		errors := make([]string, 0)

		// Fetch models from each provider concurrently
		type result struct {
			provider string
			models   []llm.Model
			err      error
		}

		ch := make(chan result, len(m.providers))

		for name, client := range m.providers {
			go func(providerName string, c llm.Client) {
				models, err := c.ListModels(ctx)
				ch <- result{provider: providerName, models: models, err: err}
			}(name, client)
		}

		// Collect results
		for i := 0; i < len(m.providers); i++ {
			res := <-ch
			if res.err != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", res.provider, res.err))
			} else if len(res.models) > 0 {
				results[res.provider] = res.models
			}
		}

		// If all providers failed, return error
		if len(results) == 0 && len(errors) > 0 {
			return errMsg{err: fmt.Errorf("failed to load models: %s", strings.Join(errors, "; "))}
		}

		return modelsLoadedMsg{models: results}
	}
}

// Messages for model selector
type modelsLoadedMsg struct {
	models map[string][]llm.Model
}

type errMsg struct {
	err error
}