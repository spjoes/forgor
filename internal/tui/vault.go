package tui

import (
	"fmt"
	"strings"
	"time"

	"forgor/internal/models"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type vaultMode int

const (
	modeList vaultMode = iota
	modeView
	modeEdit
	modeAdd
	modeDelete
)

type VaultScreen struct {
	entries       []models.Entry
	filtered      []models.Entry
	cursor        int
	searchInput   textinput.Model
	mode          vaultMode
	editEntry     models.Entry
	editFields    []textinput.Model
	editFocus     int
	showPassword  bool
	statusMsg     string
	statusIsError bool
	width         int
	height        int
}

func NewVaultScreen(entries []models.Entry) VaultScreen {
	search := textinput.New()
	search.Placeholder = "Search entries..."
	search.Width = 40

	v := VaultScreen{
		entries:     entries,
		searchInput: search,
		mode:        modeList,
	}
	v.filterEntries()
	return v
}

func (v *VaultScreen) SetEntries(entries []models.Entry) {
	v.entries = entries
	v.filterEntries()
}

func (v VaultScreen) Init() tea.Cmd {
	return nil
}

func (v VaultScreen) Update(msg tea.Msg) (VaultScreen, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height

	case StatusMsg:
		v.statusMsg = msg.Message
		v.statusIsError = msg.IsError
		return v, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
			return ClearStatusMsg{}
		})

	case ClearStatusMsg:
		v.statusMsg = ""

	case tea.KeyMsg:
		switch v.mode {
		case modeList:
			return v.updateList(msg)
		case modeView:
			return v.updateView(msg)
		case modeEdit, modeAdd:
			return v.updateEdit(msg)
		case modeDelete:
			return v.updateDelete(msg)
		}
	}

	if v.mode == modeList {
		v.searchInput, cmd = v.searchInput.Update(msg)
		v.filterEntries()
	}

	return v, cmd
}

func (v VaultScreen) updateList(msg tea.KeyMsg) (VaultScreen, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if v.cursor > 0 {
			v.cursor--
		}
	case "down", "j":
		if v.cursor < len(v.filtered)-1 {
			v.cursor++
		}
	case "enter":
		if len(v.filtered) > 0 {
			v.mode = modeView
			v.showPassword = false
		}
	case "a":
		v.mode = modeAdd
		v.editEntry = models.Entry{}
		v.initEditFields()
	case "/":
		v.searchInput.Focus()
	case "esc":
		v.searchInput.Blur()
		v.searchInput.SetValue("")
		v.filterEntries()
	}
	return v, nil
}

func (v VaultScreen) updateView(msg tea.KeyMsg) (VaultScreen, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		v.mode = modeList
	case "e":
		if len(v.filtered) > 0 {
			v.editEntry = v.filtered[v.cursor]
			v.mode = modeEdit
			v.initEditFields()
		}
	case "d":
		v.mode = modeDelete
	case "p":
		v.showPassword = !v.showPassword
	case "u":
		if len(v.filtered) > 0 {
			entry := v.filtered[v.cursor]
			return v, func() tea.Msg {
				return CopyToClipboardMsg{Text: entry.Username, Label: "Username"}
			}
		}
	case "c":
		if len(v.filtered) > 0 {
			entry := v.filtered[v.cursor]
			return v, func() tea.Msg {
				return CopyToClipboardMsg{Text: entry.Password, Label: "Password"}
			}
		}
	}
	return v, nil
}

func (v VaultScreen) updateEdit(msg tea.KeyMsg) (VaultScreen, tea.Cmd) {
	switch msg.String() {
	case "esc":
		v.mode = modeList
		return v, nil
	case "tab", "down":
		v.editFields[v.editFocus].Blur()
		v.editFocus = (v.editFocus + 1) % len(v.editFields)
		v.editFields[v.editFocus].Focus()
		return v, nil
	case "shift+tab", "up":
		v.editFields[v.editFocus].Blur()
		v.editFocus = (v.editFocus + len(v.editFields) - 1) % len(v.editFields)
		v.editFields[v.editFocus].Focus()
		return v, nil
	case "ctrl+s":
		return v.saveEntry()
	}

	var cmd tea.Cmd
	v.editFields[v.editFocus], cmd = v.editFields[v.editFocus].Update(msg)
	return v, cmd
}

func (v VaultScreen) updateDelete(msg tea.KeyMsg) (VaultScreen, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if len(v.filtered) > 0 {
			entryToDelete := v.filtered[v.cursor]
			newEntries := make([]models.Entry, 0, len(v.entries)-1)
			for _, e := range v.entries {
				if e.ID != entryToDelete.ID {
					newEntries = append(newEntries, e)
				}
			}
			v.entries = newEntries
			v.filterEntries()
			if v.cursor >= len(v.filtered) && v.cursor > 0 {
				v.cursor--
			}
			v.mode = modeList
			return v, func() tea.Msg {
				return SaveEntriesMsg{Entries: v.entries}
			}
		}
	case "n", "N", "esc":
		v.mode = modeView
	}
	return v, nil
}

func (v *VaultScreen) initEditFields() {
	fields := make([]textinput.Model, 5)

	website := textinput.New()
	website.Placeholder = "Website"
	website.SetValue(v.editEntry.Website)
	website.Width = 40
	website.Focus()
	fields[0] = website

	username := textinput.New()
	username.Placeholder = "Username"
	username.SetValue(v.editEntry.Username)
	username.Width = 40
	fields[1] = username

	password := textinput.New()
	password.Placeholder = "Password"
	password.SetValue(v.editEntry.Password)
	password.Width = 40
	fields[2] = password

	notes := textinput.New()
	notes.Placeholder = "Notes"
	notes.SetValue(v.editEntry.Notes)
	notes.Width = 40
	fields[3] = notes

	tags := textinput.New()
	tags.Placeholder = "Tags (comma separated)"
	tags.SetValue(strings.Join(v.editEntry.Tags, ", "))
	tags.Width = 40
	fields[4] = tags

	v.editFields = fields
	v.editFocus = 0
}

func (v VaultScreen) saveEntry() (VaultScreen, tea.Cmd) {
	website := strings.TrimSpace(v.editFields[0].Value())
	username := strings.TrimSpace(v.editFields[1].Value())
	password := v.editFields[2].Value()
	notes := v.editFields[3].Value()
	tagsStr := v.editFields[4].Value()

	if website == "" {
		v.statusMsg = "Website is required"
		v.statusIsError = true
		return v, nil
	}

	var tags []string
	if tagsStr != "" {
		for _, t := range strings.Split(tagsStr, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}

	if v.mode == modeAdd {
		entry := models.NewEntry(website, username, password, notes, tags)
		v.entries = append(v.entries, entry)
	} else {
		for i, e := range v.entries {
			if e.ID == v.editEntry.ID {
				v.entries[i].Website = website
				v.entries[i].Username = username
				v.entries[i].Password = password
				v.entries[i].Notes = notes
				v.entries[i].Tags = tags
				v.entries[i].UpdatedAt = time.Now()
				break
			}
		}
	}

	v.filterEntries()
	v.mode = modeList

	return v, func() tea.Msg {
		return SaveEntriesMsg{Entries: v.entries}
	}
}

func (v *VaultScreen) filterEntries() {
	query := strings.ToLower(v.searchInput.Value())
	if query == "" {
		v.filtered = v.entries
		return
	}

	v.filtered = make([]models.Entry, 0)
	for _, e := range v.entries {
		if strings.Contains(strings.ToLower(e.Website), query) ||
			strings.Contains(strings.ToLower(e.Username), query) ||
			strings.Contains(strings.ToLower(e.Notes), query) {
			v.filtered = append(v.filtered, e)
		}
		for _, t := range e.Tags {
			if strings.Contains(strings.ToLower(t), query) {
				v.filtered = append(v.filtered, e)
				break
			}
		}
	}

	if v.cursor >= len(v.filtered) {
		v.cursor = max(0, len(v.filtered)-1)
	}
}

func (v VaultScreen) View() string {
	var b strings.Builder

	switch v.mode {
	case modeList:
		b.WriteString(v.viewList())
	case modeView:
		b.WriteString(v.viewEntry())
	case modeEdit, modeAdd:
		b.WriteString(v.viewEdit())
	case modeDelete:
		b.WriteString(v.viewDelete())
	}

	if v.statusMsg != "" {
		b.WriteString("\n")
		if v.statusIsError {
			b.WriteString(errorStyle.Render("⚠ " + v.statusMsg))
		} else {
			b.WriteString(successStyle.Render("✓ " + v.statusMsg))
		}
	}

	return b.String()
}

func (v VaultScreen) viewList() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Vault"))
	b.WriteString("\n\n")
	b.WriteString(v.searchInput.View())
	b.WriteString("\n\n")

	if len(v.filtered) == 0 {
		if len(v.entries) == 0 {
			b.WriteString(mutedStyle.Render("No entries yet. Press 'a' to add one."))
		} else {
			b.WriteString(mutedStyle.Render("No entries match your search."))
		}
	} else {
		for i, entry := range v.filtered {
			cursor := "  "
			style := normalStyle
			if i == v.cursor {
				cursor = "▸ "
				style = selectedStyle
			}

			line := fmt.Sprintf("%s%s", cursor, style.Render(entry.Website))
			if entry.Username != "" {
				line += mutedStyle.Render(" (" + entry.Username + ")")
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓ navigate • enter view • a add • / search • q quit"))

	return b.String()
}

func (v VaultScreen) viewEntry() string {
	if len(v.filtered) == 0 {
		return ""
	}

	entry := v.filtered[v.cursor]
	var b strings.Builder

	b.WriteString(titleStyle.Render(entry.Website))
	b.WriteString("\n\n")

	labelStyle := lipgloss.NewStyle().Width(12).Foreground(mutedColor)

	b.WriteString(labelStyle.Render("Username:"))
	b.WriteString(entry.Username)
	b.WriteString("\n")

	b.WriteString(labelStyle.Render("Password:"))
	if v.showPassword {
		b.WriteString(entry.Password)
	} else {
		b.WriteString(strings.Repeat("•", min(len(entry.Password), 20)))
	}
	b.WriteString("\n")

	if entry.Notes != "" {
		b.WriteString(labelStyle.Render("Notes:"))
		b.WriteString(entry.Notes)
		b.WriteString("\n")
	}

	if len(entry.Tags) > 0 {
		b.WriteString(labelStyle.Render("Tags:"))
		b.WriteString(strings.Join(entry.Tags, ", "))
		b.WriteString("\n")
	}

	b.WriteString(labelStyle.Render("Updated:"))
	b.WriteString(entry.UpdatedAt.Format("2006-01-02 15:04"))
	b.WriteString("\n")

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("u copy username • c copy password • p toggle password • e edit • d delete • esc back"))

	return boxStyle.Render(b.String())
}

func (v VaultScreen) viewEdit() string {
	var b strings.Builder

	if v.mode == modeAdd {
		b.WriteString(titleStyle.Render("Add Entry"))
	} else {
		b.WriteString(titleStyle.Render("Edit Entry"))
	}
	b.WriteString("\n\n")

	labels := []string{"Website:", "Username:", "Password:", "Notes:", "Tags:"}
	for i, field := range v.editFields {
		b.WriteString(labels[i])
		b.WriteString("\n")
		if i == v.editFocus {
			b.WriteString(focusedInputStyle.Render(field.View()))
		} else {
			b.WriteString(inputStyle.Render(field.View()))
		}
		b.WriteString("\n\n")
	}

	b.WriteString(helpStyle.Render("tab next field • ctrl+s save • esc cancel"))

	return boxStyle.Render(b.String())
}

func (v VaultScreen) viewDelete() string {
	if len(v.filtered) == 0 {
		return ""
	}

	entry := v.filtered[v.cursor]
	var b strings.Builder

	b.WriteString(errorStyle.Render("Delete Entry?"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Are you sure you want to delete '%s'?\n", entry.Website))
	b.WriteString("This action cannot be undone.\n\n")
	b.WriteString(helpStyle.Render("y confirm • n cancel"))

	return boxStyle.Render(b.String())
}

func (v VaultScreen) GetSelectedEntry() *models.Entry {
	if len(v.filtered) == 0 || v.cursor >= len(v.filtered) {
		return nil
	}
	entry := v.filtered[v.cursor]
	return &entry
}

func (v VaultScreen) IsInputActive() bool {
	return v.mode == modeEdit || v.mode == modeAdd || v.searchInput.Focused()
}

func (v VaultScreen) GetEntries() []models.Entry {
	return v.entries
}

type SaveEntriesMsg struct {
	Entries []models.Entry
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
