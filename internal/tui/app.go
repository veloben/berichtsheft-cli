package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/veloben/berichtsheft-cli/internal/api"
)

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	mutedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Bold(true)
)

var dayNames = []string{"Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag"}
var statuses = []string{"anwesend", "schulzeit", "urlaub", "sonstiges"}

type dayEntry struct {
	day  int
	name string
	data api.DayData
}

type loadMsg struct {
	entries []dayEntry
	err     error
}

type saveMsg struct {
	day int
	err error
}

type model struct {
	client *api.Client
	year   int
	week   int

	entries []dayEntry
	cursor  int
	loading bool
	status  string
	err     string

	editMode bool
	input    textinput.Model
}

func Run(client *api.Client, year, week int) error {
	m := model{
		client: client,
		year:   year,
		week:   week,
		status: "loading...",
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m model) Init() tea.Cmd {
	return m.loadWeekCmd()
}

func (m model) loadWeekCmd() tea.Cmd {
	m.loading = true
	y, w := m.year, m.week
	return func() tea.Msg {
		entries := make([]dayEntry, 0, 5)
		for i := 1; i <= 5; i++ {
			day, code, err := m.client.GetDay(y, w, i)
			if err != nil {
				return loadMsg{err: err}
			}
			if code == 404 || day == nil {
				empty := api.DayData{}
				api.NormalizeDayData(&empty)
				entries = append(entries, dayEntry{day: i, name: dayNames[i-1], data: empty})
				continue
			}
			entries = append(entries, dayEntry{day: i, name: dayNames[i-1], data: *day})
		}
		return loadMsg{entries: entries}
	}
}

func (m model) saveDayCmd(idx int) tea.Cmd {
	y, w := m.year, m.week
	entry := m.entries[idx]
	return func() tea.Msg {
		err := m.client.SaveDay(y, w, entry.day, entry.data)
		return saveMsg{day: entry.day, err: err}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.editMode {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		switch k := msg.(type) {
		case tea.KeyMsg:
			switch k.String() {
			case "esc":
				m.editMode = false
				m.status = "edit cancelled"
				m.err = ""
				return m, nil
			case "enter":
				if len(m.entries) > 0 {
					m.entries[m.cursor].data.MdData = strings.TrimSpace(m.input.Value())
				}
				m.editMode = false
				m.status = "saving text..."
				m.err = ""
				return m, m.saveDayCmd(m.cursor)
			}
		}
		return m, cmd
	}

	switch v := msg.(type) {
	case tea.KeyMsg:
		switch v.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "r":
			m.status = "reloading..."
			m.err = ""
			return m, m.loadWeekCmd()
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.entries)-1 {
				m.cursor++
			}
		case "[":
			m.prevWeek()
			m.status = "loading previous week..."
			m.err = ""
			return m, m.loadWeekCmd()
		case "]":
			m.nextWeek()
			m.status = "loading next week..."
			m.err = ""
			return m, m.loadWeekCmd()
		case "s":
			if len(m.entries) == 0 {
				return m, nil
			}
			e := &m.entries[m.cursor]
			e.data.Metadata.Status = cycleStatus(e.data.Metadata.Status)
			m.status = fmt.Sprintf("saving %s status=%s", e.name, e.data.Metadata.Status)
			m.err = ""
			return m, m.saveDayCmd(m.cursor)
		case "l":
			if len(m.entries) == 0 {
				return m, nil
			}
			e := &m.entries[m.cursor]
			if e.data.Metadata.Location == "schule" {
				e.data.Metadata.Location = "betrieb"
			} else {
				e.data.Metadata.Location = "schule"
			}
			m.status = fmt.Sprintf("saving %s location=%s", e.name, e.data.Metadata.Location)
			m.err = ""
			return m, m.saveDayCmd(m.cursor)
		case "+", "=":
			if len(m.entries) == 0 {
				return m, nil
			}
			e := &m.entries[m.cursor]
			e.data.Metadata.TimeSpent = api.ClampTime(e.data.Metadata.TimeSpent + 1)
			m.status = fmt.Sprintf("saving %s time=%dh", e.name, e.data.Metadata.TimeSpent)
			m.err = ""
			return m, m.saveDayCmd(m.cursor)
		case "-", "_":
			if len(m.entries) == 0 {
				return m, nil
			}
			e := &m.entries[m.cursor]
			e.data.Metadata.TimeSpent = api.ClampTime(e.data.Metadata.TimeSpent - 1)
			m.status = fmt.Sprintf("saving %s time=%dh", e.name, e.data.Metadata.TimeSpent)
			m.err = ""
			return m, m.saveDayCmd(m.cursor)
		case "e":
			if len(m.entries) == 0 {
				return m, nil
			}
			ti := textinput.New()
			ti.Placeholder = "Day text"
			ti.CharLimit = 1000
			ti.Width = 90
			ti.SetValue(m.entries[m.cursor].data.MdData)
			ti.Focus()
			m.input = ti
			m.editMode = true
			m.err = ""
			m.status = "edit mode: Enter save, Esc cancel"
		}

	case loadMsg:
		m.loading = false
		if v.err != nil {
			m.err = v.err.Error()
			m.status = ""
			return m, nil
		}
		m.entries = v.entries
		if m.cursor >= len(m.entries) {
			m.cursor = 0
		}
		m.err = ""
		m.status = fmt.Sprintf("loaded %d days", len(m.entries))
	case saveMsg:
		if v.err != nil {
			m.err = v.err.Error()
			m.status = ""
			return m, nil
		}
		m.err = ""
		m.status = fmt.Sprintf("saved day %d at %s", v.day, time.Now().Format("15:04:05"))
	}

	return m, nil
}

func (m model) View() string {
	b := &strings.Builder{}
	fmt.Fprintln(b, headerStyle.Render(fmt.Sprintf("Berichtsheft TUI  |  %d-W%02d", m.year, m.week)))
	fmt.Fprintln(b, mutedStyle.Render("j/k: move  s:cycle status  l:toggle location  +/-:time  e:edit text  [/]:week  r:reload  q:quit"))
	fmt.Fprintln(b)

	for i, e := range m.entries {
		cursor := " "
		if i == m.cursor {
			cursor = cursorStyle.Render("➜")
		}
		textPreview := strings.TrimSpace(e.data.MdData)
		if len(textPreview) > 48 {
			textPreview = textPreview[:48] + "…"
		}
		if textPreview == "" {
			textPreview = mutedStyle.Render("(leer)")
		}
		fmt.Fprintf(b, "%s %d %-11s  %-10s  %-7s  %4dh  %s\n",
			cursor,
			e.day,
			e.name,
			e.data.Metadata.Status,
			e.data.Metadata.Location,
			e.data.Metadata.TimeSpent,
			textPreview,
		)
	}

	fmt.Fprintln(b)
	if m.editMode {
		fmt.Fprintln(b, headerStyle.Render("Edit text:"))
		fmt.Fprintln(b, m.input.View())
	}

	if m.err != "" {
		fmt.Fprintln(b, errStyle.Render("ERROR: "+m.err))
	} else if m.status != "" {
		fmt.Fprintln(b, okStyle.Render(m.status))
	}

	return b.String()
}

func cycleStatus(current string) string {
	cur := api.NormalizeStatus(current)
	idx := 0
	for i, s := range statuses {
		if s == cur {
			idx = i
			break
		}
	}
	return statuses[(idx+1)%len(statuses)]
}

func (m *model) nextWeek() {
	_, max := time.Date(m.year, time.December, 28, 0, 0, 0, 0, time.UTC).ISOWeek()
	if m.week >= max {
		m.year++
		m.week = 1
		return
	}
	m.week++
}

func (m *model) prevWeek() {
	if m.week <= 1 {
		m.year--
		_, max := time.Date(m.year, time.December, 28, 0, 0, 0, 0, time.UTC).ISOWeek()
		m.week = max
		return
	}
	m.week--
}
