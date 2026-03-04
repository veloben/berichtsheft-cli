package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
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

	panelStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
)

var dayNames = []string{"Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag"}
var statuses = []string{"anwesend", "schulzeit", "urlaub", "sonstiges"}

type dayEntry struct {
	day         int
	name        string
	date        time.Time
	data        api.DayData
	dirty       bool
	saving      bool
	lastSavedAt string
}

type loadMsg struct {
	entries []dayEntry
	err     error
}

type saveMsg struct {
	idx int
	err error
}

type saveAllMsg struct {
	saved  []int
	failed map[int]string
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

	helpVisible bool

	editMode bool
	editor   textarea.Model

	width  int
	height int
}

func Run(client *api.Client, year, week int) error {
	m := model{
		client:      client,
		year:        year,
		week:        week,
		status:      "Lade Woche...",
		helpVisible: true,
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m model) Init() tea.Cmd {
	return m.loadWeekCmd()
}

func (m model) loadWeekCmd() tea.Cmd {
	y, w := m.year, m.week
	return func() tea.Msg {
		entries := make([]dayEntry, 0, 5)
		monday := mondayOfISOWeek(y, w)

		for i := 1; i <= 5; i++ {
			date := monday.AddDate(0, 0, i-1)
			day, code, err := m.client.GetDay(y, w, i)
			if err != nil {
				return loadMsg{err: err}
			}
			if code == 404 || day == nil {
				empty := api.DayData{}
				api.NormalizeDayData(&empty)
				entries = append(entries, dayEntry{day: i, name: dayNames[i-1], date: date, data: empty})
				continue
			}
			entries = append(entries, dayEntry{day: i, name: dayNames[i-1], date: date, data: *day})
		}

		return loadMsg{entries: entries}
	}
}

func (m model) saveDayCmd(idx int) tea.Cmd {
	y, w := m.year, m.week
	entry := m.entries[idx]
	api.NormalizeDayData(&entry.data)
	return func() tea.Msg {
		err := m.client.SaveDay(y, w, entry.day, entry.data)
		return saveMsg{idx: idx, err: err}
	}
}

func (m model) saveAllCmd() tea.Cmd {
	y, w := m.year, m.week

	type payload struct {
		idx  int
		day  int
		data api.DayData
	}

	batch := make([]payload, 0)
	for i := range m.entries {
		if !m.entries[i].dirty {
			continue
		}
		d := m.entries[i].data
		api.NormalizeDayData(&d)
		batch = append(batch, payload{idx: i, day: m.entries[i].day, data: d})
	}

	return func() tea.Msg {
		res := saveAllMsg{saved: []int{}, failed: map[int]string{}}
		for _, item := range batch {
			if err := m.client.SaveDay(y, w, item.day, item.data); err != nil {
				res.failed[item.idx] = err.Error()
				continue
			}
			res.saved = append(res.saved, item.idx)
		}
		return res
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.editMode {
		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(msg)

		switch k := msg.(type) {
		case tea.KeyMsg:
			switch k.String() {
			case "esc":
				m.editMode = false
				m.status = "Bearbeitung abgebrochen"
				m.err = ""
				return m, nil
			case "ctrl+s":
				if len(m.entries) == 0 {
					m.editMode = false
					return m, nil
				}
				m.entries[m.cursor].data.MdData = strings.TrimSpace(m.editor.Value())
				m.entries[m.cursor].dirty = true
				m.editMode = false
				m.status = "Text aktualisiert (noch nicht gespeichert)"
				m.err = ""
				return m, nil
			}
		}

		return m, cmd
	}

	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height

	case tea.KeyMsg:
		switch v.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "?":
			m.helpVisible = !m.helpVisible
			return m, nil
		case "r":
			m.loading = true
			m.status = "Lade Woche neu..."
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
		case "left", "h", "[":
			m.prevWeek()
			m.loading = true
			m.status = "Vorherige Woche laden..."
			m.err = ""
			return m, m.loadWeekCmd()
		case "right", "l", "]":
			m.nextWeek()
			m.loading = true
			m.status = "Nächste Woche laden..."
			m.err = ""
			return m, m.loadWeekCmd()
		case "s":
			if len(m.entries) == 0 {
				return m, nil
			}
			e := &m.entries[m.cursor]
			e.data.Metadata.Status = cycleStatus(e.data.Metadata.Status)
			e.dirty = true
			m.status = fmt.Sprintf("%s: Status -> %s", e.name, e.data.Metadata.Status)
			m.err = ""
		case "o":
			if len(m.entries) == 0 {
				return m, nil
			}
			e := &m.entries[m.cursor]
			if e.data.Metadata.Location == "schule" {
				e.data.Metadata.Location = "betrieb"
			} else {
				e.data.Metadata.Location = "schule"
			}
			e.dirty = true
			m.status = fmt.Sprintf("%s: Ort -> %s", e.name, e.data.Metadata.Location)
			m.err = ""
		case "+", "=":
			if len(m.entries) == 0 {
				return m, nil
			}
			e := &m.entries[m.cursor]
			e.data.Metadata.TimeSpent = api.ClampTime(e.data.Metadata.TimeSpent + 1)
			e.dirty = true
			m.status = fmt.Sprintf("%s: Zeit -> %dh", e.name, e.data.Metadata.TimeSpent)
			m.err = ""
		case "-", "_":
			if len(m.entries) == 0 {
				return m, nil
			}
			e := &m.entries[m.cursor]
			e.data.Metadata.TimeSpent = api.ClampTime(e.data.Metadata.TimeSpent - 1)
			e.dirty = true
			m.status = fmt.Sprintf("%s: Zeit -> %dh", e.name, e.data.Metadata.TimeSpent)
			m.err = ""
		case "e", "enter":
			if len(m.entries) == 0 {
				return m, nil
			}
			ta := textarea.New()
			ta.Placeholder = "Was hast du gemacht?"
			ta.SetValue(m.entries[m.cursor].data.MdData)
			ta.Focus()
			ta.CharLimit = 3000
			ta.SetWidth(maxInt(40, m.width-10))
			ta.SetHeight(maxInt(8, minInt(16, m.height/3)))
			m.editor = ta
			m.editMode = true
			m.status = "Text bearbeiten (Ctrl+S speichern, Esc abbrechen)"
			m.err = ""
		case "w":
			if len(m.entries) == 0 {
				return m, nil
			}
			if !m.entries[m.cursor].dirty {
				m.status = "Keine Änderungen zum Speichern"
				m.err = ""
				return m, nil
			}
			m.entries[m.cursor].saving = true
			m.status = fmt.Sprintf("Speichere %s...", m.entries[m.cursor].name)
			m.err = ""
			return m, m.saveDayCmd(m.cursor)
		case "a":
			dirtyCount := 0
			for i := range m.entries {
				if m.entries[i].dirty {
					dirtyCount++
					m.entries[i].saving = true
				}
			}
			if dirtyCount == 0 {
				m.status = "Keine Änderungen zum Speichern"
				m.err = ""
				return m, nil
			}
			m.status = fmt.Sprintf("Speichere %d geänderte Tage...", dirtyCount)
			m.err = ""
			return m, m.saveAllCmd()
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
		m.status = fmt.Sprintf("Woche geladen (%d Tage)", len(m.entries))

	case saveMsg:
		if v.idx >= 0 && v.idx < len(m.entries) {
			m.entries[v.idx].saving = false
		}
		if v.err != nil {
			m.err = v.err.Error()
			m.status = ""
			return m, nil
		}
		if v.idx >= 0 && v.idx < len(m.entries) {
			m.entries[v.idx].dirty = false
			m.entries[v.idx].lastSavedAt = time.Now().Format("15:04")
			m.status = fmt.Sprintf("Gespeichert: %s (%s)", m.entries[v.idx].name, m.entries[v.idx].lastSavedAt)
			m.err = ""
		}

	case saveAllMsg:
		for i := range m.entries {
			m.entries[i].saving = false
		}

		now := time.Now().Format("15:04")
		for _, idx := range v.saved {
			if idx >= 0 && idx < len(m.entries) {
				m.entries[idx].dirty = false
				m.entries[idx].lastSavedAt = now
			}
		}

		if len(v.failed) > 0 {
			indexes := make([]int, 0, len(v.failed))
			for idx := range v.failed {
				indexes = append(indexes, idx)
			}
			sort.Ints(indexes)

			parts := make([]string, 0, len(indexes))
			for _, idx := range indexes {
				parts = append(parts, fmt.Sprintf("%s: %s", m.entries[idx].name, v.failed[idx]))
			}
			m.err = strings.Join(parts, " | ")
			m.status = fmt.Sprintf("%d gespeichert, %d fehlgeschlagen", len(v.saved), len(v.failed))
			return m, nil
		}

		m.err = ""
		m.status = fmt.Sprintf("Alle Änderungen gespeichert (%d Tage)", len(v.saved))
	}

	return m, nil
}

func (m model) View() string {
	b := &strings.Builder{}

	start, end := weekRange(m.year, m.week)
	head := fmt.Sprintf("Berichtsheft TUI  |  %d-W%02d  (%s – %s)",
		m.year,
		m.week,
		start.Format("02.01."),
		end.Format("02.01."),
	)
	fmt.Fprintln(b, headerStyle.Render(head))

	dirtyCount := 0
	for _, e := range m.entries {
		if e.dirty {
			dirtyCount++
		}
	}
	fmt.Fprintln(b, mutedStyle.Render(fmt.Sprintf("Geändert: %d  |  [w]=Speichern  [a]=Alle speichern  [r]=Reload  [q]=Quit", dirtyCount)))
	fmt.Fprintln(b)

	if m.loading {
		fmt.Fprintln(b, mutedStyle.Render("Lade Daten..."))
	}

	if len(m.entries) == 0 {
		fmt.Fprintln(b, mutedStyle.Render("Keine Daten"))
	} else {
		list := m.renderDayList()
		detail := m.renderDetailPane()

		if m.width >= 110 {
			left := panelStyle.Width(maxInt(40, m.width/2-2)).Render(list)
			right := panelStyle.Width(maxInt(50, m.width/2-2)).Render(detail)
			fmt.Fprintln(b, lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right))
		} else {
			fmt.Fprintln(b, panelStyle.Render(list))
			fmt.Fprintln(b)
			fmt.Fprintln(b, panelStyle.Render(detail))
		}
	}

	if m.editMode {
		fmt.Fprintln(b)
		fmt.Fprintln(b, panelStyle.Render(headerStyle.Render("Text bearbeiten (Ctrl+S speichern, Esc abbrechen)")+"\n\n"+m.editor.View()))
	}

	if m.err != "" {
		fmt.Fprintln(b)
		fmt.Fprintln(b, errStyle.Render("ERROR: "+m.err))
	} else if m.status != "" {
		fmt.Fprintln(b)
		fmt.Fprintln(b, okStyle.Render(m.status))
	}

	if m.helpVisible {
		fmt.Fprintln(b)
		fmt.Fprintln(b, mutedStyle.Render("Shortcuts: j/k oder ↑/↓ Tag wählen | h/l oder ←/→ Woche wechseln | s Status | o Ort | +/- Stunden | e/Enter Text | ? Hilfe"))
	}

	return b.String()
}

func (m model) renderDayList() string {
	b := &strings.Builder{}
	fmt.Fprintln(b, headerStyle.Render("Tage"))
	fmt.Fprintln(b)

	for i, e := range m.entries {
		cursor := " "
		if i == m.cursor {
			cursor = cursorStyle.Render("➜")
		}

		state := ""
		if e.saving {
			state = mutedStyle.Render(" ⏳")
		} else if e.dirty {
			state = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render(" *")
		} else if e.lastSavedAt != "" {
			state = mutedStyle.Render(" ✓" + e.lastSavedAt)
		}

		fmt.Fprintf(
			b,
			"%s %d %-11s %-6s  %-10s %-7s %2dh%s\n",
			cursor,
			e.day,
			e.name,
			e.date.Format("02.01"),
			e.data.Metadata.Status,
			e.data.Metadata.Location,
			e.data.Metadata.TimeSpent,
			state,
		)
	}

	return b.String()
}

func (m model) renderDetailPane() string {
	if len(m.entries) == 0 {
		return "Keine Auswahl"
	}
	e := m.entries[m.cursor]

	b := &strings.Builder{}
	fmt.Fprintln(b, headerStyle.Render("Details"))
	fmt.Fprintln(b)
	fmt.Fprintf(b, "Tag: %s (%s)\n", e.name, e.date.Format("Monday, 02.01.2006"))
	fmt.Fprintf(b, "Status: %s\n", e.data.Metadata.Status)
	fmt.Fprintf(b, "Ort: %s\n", e.data.Metadata.Location)
	fmt.Fprintf(b, "Stunden: %dh\n", e.data.Metadata.TimeSpent)
	fmt.Fprintf(b, "Kommentare: %d\n", len(e.data.Metadata.Comments))
	fmt.Fprintf(b, "Qualifikationen: %d\n", len(e.data.Metadata.Qualifications))
	fmt.Fprintln(b)
	fmt.Fprintln(b, headerStyle.Render("Text"))

	text := strings.TrimSpace(e.data.MdData)
	if text == "" {
		text = "(leer)"
	}
	fmt.Fprintln(b, truncateMultiline(text, 12, 100))

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

func mondayOfISOWeek(year, week int) time.Time {
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
	monday := jan4.AddDate(0, 0, -(int(jan4.Weekday())+6)%7)
	return monday.AddDate(0, 0, (week-1)*7)
}

func weekRange(year, week int) (time.Time, time.Time) {
	start := mondayOfISOWeek(year, week)
	end := start.AddDate(0, 0, 4)
	return start, end
}

func truncateMultiline(s string, maxLines, maxCols int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines[maxLines-1] += " …"
	}
	for i := range lines {
		if len(lines[i]) > maxCols {
			lines[i] = lines[i][:maxCols] + "…"
		}
	}
	return strings.Join(lines, "\n")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
