package multiselection

import (
	"bytes"
	"fmt"
	"os"
	"text/template"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/real-rock/goprompt"
	"golang.org/x/term"
)

// Model implements the bubbletea.Model for a selection prompt.
type Model[T any] struct {
	*Selection[T]

	// Err holds errors that may occur during the execution of
	// the selection prompt.
	Err error

	// MaxWidth limits the width of the view using the Selection's WrapMode.
	MaxWidth int

	selected map[int]*Choice[T]

	filterInput textinput.Model
	// currently displayed choices, after filtering and pagination
	currentChoices []*Choice[T]
	// number of available choices after filtering
	availableChoices int
	// index of current selection in currentChoices slice
	currentIdx        int
	scrollOffset      int
	width             int
	height            int
	tmpl              *template.Template
	resultTmpl        *template.Template
	requestedPageSize int

	quitting bool
}

// ensure that the Model interface is implemented.
var _ tea.Model = &Model[any]{}

// NewModel returns a new selection prompt model for the
// provided choices.
func NewModel[T any](selection *Selection[T]) *Model[T] {
	return &Model[T]{
		Selection: selection,
		selected:  make(map[int]*Choice[T]),
	}
}

// Init initializes the selection prompt model.
func (m *Model[T]) Init() tea.Cmd {
	m.reindexChoices()

	if len(m.choices) == 0 {
		m.Err = fmt.Errorf("no choices provided")

		return tea.Quit
	}

	if m.Template == "" {
		m.Err = fmt.Errorf("empty template")

		return tea.Quit
	}

	m.tmpl, m.Err = m.initTemplate()
	if m.Err != nil {
		return tea.Quit
	}

	m.resultTmpl, m.Err = m.initResultTemplate()
	if m.Err != nil {
		return tea.Quit
	}

	m.filterInput = m.initFilterInput()

	m.currentChoices, m.availableChoices = m.filteredAndPagedChoices()

	m.requestedPageSize = m.PageSize

	// try to get an initial terminal size in order to avoid initial overdrawing
	// which can cause ugly glitches on some terminals
	outputFile, ok := m.Output.(*os.File)
	if ok {
		width, height, err := term.GetSize(int(outputFile.Fd()))
		if err == nil {
			m.resize(width, height)
		}
	}

	return textinput.Blink
}

func (m *Model[T]) initTemplate() (*template.Template, error) {
	tmpl := template.New("view")
	tmpl.Funcs(termenv.TemplateFuncs(m.ColorProfile))
	tmpl.Funcs(m.ExtendedTemplateFuncs)
	tmpl.Funcs(goprompt.UtilFuncMap())
	tmpl.Funcs(template.FuncMap{
		"IsScrollDownHintPosition": func(idx int) bool {
			return m.canScrollDown() && (idx == len(m.currentChoices)-1)
		},
		"IsScrollUpHintPosition": func(idx int) bool {
			return m.canScrollUp() && idx == 0 && m.scrollOffset > 0
		},
		"Selected": func(c *Choice[T]) string {
			if m.SelectedChoiceStyle == nil {
				return c.String
			}

			return m.SelectedChoiceStyle(c)
		},
		"CurrentCursor": func(c *Choice[T]) string {
			if m.SelectedChoiceStyle == nil {
				return c.String
			}

			return m.CurrentCursorStyle(c)
		},
		"Unselected": func(c *Choice[T]) string {
			if m.UnselectedChoiceStyle == nil {
				return c.String
			}

			return m.UnselectedChoiceStyle(c)
		},
		"Contains": func(s map[int]*Choice[T], elem *Choice[T]) bool {
			_, ok := s[elem.idx]
			return ok
		},
	})

	return tmpl.Parse(m.Template)
}

func (m *Model[T]) initResultTemplate() (*template.Template, error) {
	if m.ResultTemplate == "" {
		return nil, nil //nolint:nilnil
	}

	tmpl := template.New("result")
	tmpl.Funcs(termenv.TemplateFuncs(m.ColorProfile))
	tmpl.Funcs(m.ExtendedTemplateFuncs)
	tmpl.Funcs(goprompt.UtilFuncMap())
	tmpl.Funcs(template.FuncMap{
		"Final": func(c *Choice[T]) string {
			if m.FinalChoiceStyle == nil {
				return c.String
			}

			return m.FinalChoiceStyle(c)
		},
	})

	return tmpl.Parse(m.ResultTemplate)
}

func (m *Model[T]) initFilterInput() textinput.Model {
	filterInput := textinput.New()
	filterInput.Prompt = ""
	filterInput.TextStyle = m.FilterInputTextStyle
	filterInput.PlaceholderStyle = m.FilterInputPlaceholderStyle
	filterInput.Cursor.Style = m.FilterInputCursorStyle
	filterInput.Placeholder = m.FilterPlaceholder
	filterInput.Width = 80
	filterInput.Focus()

	return filterInput
}

// ValueAsChoice returns the selected value wrapped in a Choice struct.
func (m *Model[T]) ValueAsChoice() ([]*Choice[T], error) {
	if m.Err != nil {
		return nil, m.Err
	}

	if len(m.currentChoices) == 0 {
		return nil, fmt.Errorf("no choices")
	}

	if m.currentIdx < 0 || m.currentIdx >= len(m.currentChoices) {
		return nil, fmt.Errorf("choice index out of bounds")
	}

	selectedSlice := []*Choice[T]{}

	for idx := range m.selected {
		selectedSlice = append(selectedSlice, m.choices[idx])
	}

	return selectedSlice, nil
}

// Value returns the choice that is currently selected or the final
// choice after the prompt has concluded.
func (m *Model[T]) Value() ([]T, error) {
	choices, err := m.ValueAsChoice()
	if err != nil {
		var zeroValue T

		return []T{zeroValue}, err
	}

	values := []T{}
	for _, c := range choices {
		values = append(values, c.Value)
	}

	return values, nil
}

// Update updates the model based on the received message.
func (m *Model[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.Err != nil {
		return m, tea.Quit
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case keyMatches(msg, m.KeyMap.Abort):
			m.Err = goprompt.ErrAborted
			m.quitting = true

			return m, tea.Quit
		case keyMatches(msg, m.KeyMap.Select):
			if len(m.currentChoices) == 0 {
				return m, nil
			}

			m.quitting = true

			return m, tea.Quit
		case keyMatches(msg, m.KeyMap.Tab):
			c := m.currentChoices[m.currentIdx]
			if _, ok := m.selected[c.Index()]; ok {
				delete(m.selected, c.Index())
			} else {
				m.selected[c.Index()] = c
			}
			m.cursorDown()
		case keyMatches(msg, m.KeyMap.ClearFilter):
			m.filterInput.Reset()
			m.currentChoices, m.availableChoices = m.filteredAndPagedChoices()
		case keyMatches(msg, m.KeyMap.Down):
			m.cursorDown()
		case keyMatches(msg, m.KeyMap.Up):
			m.cursorUp()
		case keyMatches(msg, m.KeyMap.ScrollDown):
			m.scrollDown()
		case keyMatches(msg, m.KeyMap.ScrollUp):
			m.scrollUp()
		case keyMatches(msg, m.KeyMap.Exit):
			m.quitting = true
			return m, tea.Quit
		default:
			return m.updateFilter(msg)
		}

		return m, nil
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)

		return m, tea.ClearScrollArea
	case error:
		m.Err = msg

		return m, tea.Quit
	}

	var cmd tea.Cmd

	return m, cmd
}

func (m *Model[T]) resize(width int, height int) {
	m.width = zeroAwareMin(width, m.MaxWidth)

	if m.height != height {
		m.height = height
		m.forceUpdatePageSizeForHeight()
	}
}

func (m *Model[T]) forceUpdatePageSizeForHeight() {
	maxAcceptablePageSize := len(m.choices)
	if m.requestedPageSize != 0 {
		maxAcceptablePageSize = min(len(m.choices), m.requestedPageSize)
	}

	// try preferred page size first
	m.PageSize = maxAcceptablePageSize
	m.currentIdx = 0
	m.scrollOffset = 0
	m.currentChoices, m.availableChoices = m.filteredAndPagedChoices()

	if lipgloss.Height(m.View()) < m.height {
		return
	}

	// if it does not fit, brute force a fitting page size
	for m.PageSize = 1; m.PageSize <= maxAcceptablePageSize; m.PageSize++ {
		m.currentChoices, m.availableChoices = m.filteredAndPagedChoices()

		if lipgloss.Height(m.View()) >= m.height {
			m.PageSize--
			m.currentChoices, m.availableChoices = m.filteredAndPagedChoices()

			return
		}
	}

	m.PageSize--
	m.currentChoices, m.availableChoices = m.filteredAndPagedChoices()
}

func (m *Model[T]) updateFilter(msg tea.Msg) (*Model[T], tea.Cmd) {
	if m.Filter == nil {
		return m, nil
	}

	previousFilter := m.filterInput.Value()

	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)

	if m.filterInput.Value() != previousFilter {
		m.currentIdx = 0
		m.scrollOffset = 0
		m.currentChoices, m.availableChoices = m.filteredAndPagedChoices()
	}

	return m, cmd
}

// View renders the selection prompt.
func (m *Model[T]) View() string {
	viewBuffer := &bytes.Buffer{}

	if m.quitting {
		return ""
	}

	// avoid panics if Quit is sent during Init
	if m.tmpl == nil {
		return ""
	}

	err := m.tmpl.Execute(viewBuffer, map[string]interface{}{
		"Prompt":        m.Prompt,
		"IsFiltered":    m.Filter != nil,
		"FilterPrompt":  m.FilterPrompt,
		"FilterInput":   m.filterInput.View(),
		"Choices":       m.currentChoices,
		"NChoices":      len(m.currentChoices),
		"CurrentIndex":  m.currentIdx,
		"SelectedItems": m.selected,
		"PageSize":      m.PageSize,
		"IsPaged":       m.PageSize > 0 && len(m.currentChoices) > m.PageSize,
		"AllChoices":    m.choices,
		"NAllChoices":   len(m.choices),
		"TerminalWidth": m.width,
	})
	if err != nil {
		m.Err = err

		return "Template Error: " + err.Error()
	}

	return m.wrap(viewBuffer.String())
}

func (m *Model[T]) resultView() (string, error) {
	viewBuffer := &bytes.Buffer{}

	if m.ResultTemplate == "" {
		return "", nil
	}

	if m.resultTmpl == nil {
		return "", fmt.Errorf("rendering confirmation without loaded template")
	}

	choice, err := m.ValueAsChoice()
	if err != nil {
		return "", err
	}

	err = m.resultTmpl.Execute(viewBuffer, map[string]interface{}{
		"FinalChoice":   choice,
		"Prompt":        m.Prompt,
		"AllChoices":    m.choices,
		"NAllChoices":   len(m.choices),
		"TerminalWidth": m.width,
	})
	if err != nil {
		return "", fmt.Errorf("execute confirmation template: %w", err)
	}

	return viewBuffer.String(), nil
}

func (m *Model[T]) wrap(text string) string {
	if m.WrapMode == nil {
		return text
	}

	return m.WrapMode(text, m.width)
}

func SearchPrefix[T any](filter string, arr []*Choice[T]) []*Choice[T] {
	if filter == "" {
		return arr
	}
	lower := LowerBound(filter, arr)
	return arr[lower:]
}

// Find the lower bound of the prefix in the sorted slice.
func LowerBound[T any](filter string, arr []*Choice[T]) int {
	low, high := 0, len(arr)
	for low < high {
		mid := low + (high-low)/2
		if arr[mid].String < filter {
			low = mid + 1
		} else {
			high = mid
		}
	}
	return low
}

func (m *Model[T]) filteredAndPagedChoices() ([]*Choice[T], int) {
	// choices := []*Choice[T]{}

	var available, ignored int
	// var available int

	choices := SearchPrefix(m.filterInput.Value(), m.choices)
	viewedChoices := []*Choice[T]{}

	for _, choice := range choices {
		if m.Filter != nil && !m.Filter(m.filterInput.Value(), choice) {
			continue
		}

		available++

		if m.PageSize > 0 && (len(viewedChoices) >= m.PageSize || ignored < m.scrollOffset) {
			ignored++

			continue
		}

		viewedChoices = append(viewedChoices, choice)
	}

	return viewedChoices, available
}

func (m *Model[T]) canScrollDown() bool {
	if m.PageSize <= 0 || m.availableChoices <= m.PageSize {
		return false
	}

	if m.scrollOffset+m.PageSize >= len(m.choices) {
		return false
	}

	return true
}

func (m *Model[T]) canScrollUp() bool {
	return m.scrollOffset > 0
}

func (m *Model[T]) cursorDown() {
	if m.currentIdx == len(m.currentChoices)-1 {
		if m.canScrollDown() {
			m.scrollDown()
		} else if m.LoopCursor {
			m.scrollToTop()

			return
		}
	}

	m.currentIdx = min(len(m.currentChoices)-1, m.currentIdx+1)
}

func (m *Model[T]) cursorUp() {
	if m.currentIdx == 0 {
		if m.canScrollUp() {
			m.scrollUp()
		} else if m.LoopCursor {
			m.scrollToBottom()

			return
		}
	}

	m.currentIdx = max(0, m.currentIdx-1)
}

func (m *Model[T]) scrollDown() {
	if m.PageSize <= 0 || m.scrollOffset+m.PageSize >= m.availableChoices {
		return
	}

	m.currentIdx = max(0, m.currentIdx-1)
	m.scrollOffset++
	m.currentChoices, m.availableChoices = m.filteredAndPagedChoices()
}

func (m *Model[T]) scrollToBottom() {
	m.currentIdx = len(m.currentChoices) - 1

	if m.PageSize <= 0 || m.availableChoices < m.PageSize {
		return
	}

	m.scrollOffset = m.availableChoices - m.PageSize
	m.currentChoices, m.availableChoices = m.filteredAndPagedChoices()
}

func (m *Model[T]) scrollUp() {
	if m.PageSize <= 0 || m.scrollOffset <= 0 {
		return
	}

	m.currentIdx = min(len(m.currentChoices)-1, m.currentIdx+1)
	m.scrollOffset--
	m.currentChoices, m.availableChoices = m.filteredAndPagedChoices()
}

func (m *Model[T]) scrollToTop() {
	m.currentIdx = 0

	if m.PageSize <= 0 || m.availableChoices < m.PageSize {
		return
	}

	m.scrollOffset = 0
	m.currentChoices, m.availableChoices = m.filteredAndPagedChoices()
}

func (m *Model[T]) reindexChoices() {
	for i, choice := range m.choices {
		choice.idx = i
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}

	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}

	return b
}

func zeroAwareMin(a int, b int) int {
	switch {
	case a == 0:
		return b
	case b == 0:
		return a
	default:
		return min(a, b)
	}
}
