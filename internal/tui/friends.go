package tui

import (
	"fmt"
	"strings"

	"forgor/internal/models"

	tea "github.com/charmbracelet/bubbletea"
)

type friendsMode int

const (
	friendsModeList friendsMode = iota
	friendsModeShare
	friendsModeDelete
)

type FriendsScreen struct {
	friends       []models.Friend
	cursor        int
	mode          friendsMode
	selectedEntry *models.Entry
	statusMsg     string
	isError       bool
}

func NewFriendsScreen() FriendsScreen {
	return FriendsScreen{
		friends: []models.Friend{},
		mode:    friendsModeList,
	}
}

func (f FriendsScreen) Init() tea.Cmd {
	return nil
}

func (f FriendsScreen) Update(msg tea.Msg) (FriendsScreen, tea.Cmd) {
	switch msg := msg.(type) {
	case RefreshFriendsMsg:
		f.friends = msg.Friends
		if f.cursor >= len(f.friends) && f.cursor > 0 {
			f.cursor = len(f.friends) - 1
		}
		return f, nil

	case ShareSentMsg:
		f.statusMsg = "Entry shared successfully!"
		f.isError = false
		f.mode = friendsModeList
		return f, nil

	case ShareFailMsg:
		f.statusMsg = "Share failed: " + msg.Err.Error()
		f.isError = true
		f.mode = friendsModeList
		return f, nil

	case StatusMsg:
		f.statusMsg = msg.Message
		f.isError = msg.IsError
		return f, nil

	case tea.KeyMsg:
		switch f.mode {
		case friendsModeList:
			return f.updateList(msg)
		case friendsModeShare:
			return f.updateShare(msg)
		case friendsModeDelete:
			return f.updateDelete(msg)
		}
	}

	return f, nil
}

func (f FriendsScreen) updateList(msg tea.KeyMsg) (FriendsScreen, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if f.cursor > 0 {
			f.cursor--
		}
	case "down", "j":
		if f.cursor < len(f.friends)-1 {
			f.cursor++
		}
	case "s":
		if len(f.friends) > 0 && f.selectedEntry != nil {
			f.mode = friendsModeShare
		} else if f.selectedEntry == nil {
			f.statusMsg = "Select an entry in Vault first (press 's' on vault tab)"
			f.isError = true
		}
	case "d":
		if len(f.friends) > 0 {
			f.mode = friendsModeDelete
		}
	}
	return f, nil
}

func (f FriendsScreen) updateShare(msg tea.KeyMsg) (FriendsScreen, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if len(f.friends) > 0 && f.selectedEntry != nil {
			friend := f.friends[f.cursor]
			entry := *f.selectedEntry
			return f, func() tea.Msg {
				return SendShareMsg{Friend: friend, Entry: entry}
			}
		}
	case "n", "N", "esc":
		f.mode = friendsModeList
	}
	return f, nil
}

func (f FriendsScreen) updateDelete(msg tea.KeyMsg) (FriendsScreen, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if len(f.friends) > 0 {
			friend := f.friends[f.cursor]
			return f, func() tea.Msg {
				return DeleteFriendMsg{Fingerprint: friend.Fingerprint}
			}
		}
	case "n", "N", "esc":
		f.mode = friendsModeList
	}
	return f, nil
}

func (f FriendsScreen) View() string {
	var b strings.Builder

	switch f.mode {
	case friendsModeList:
		b.WriteString(f.viewList())
	case friendsModeShare:
		b.WriteString(f.viewShare())
	case friendsModeDelete:
		b.WriteString(f.viewDelete())
	}

	if f.statusMsg != "" {
		b.WriteString("\n")
		if f.isError {
			b.WriteString(errorStyle.Render("⚠ " + f.statusMsg))
		} else {
			b.WriteString(successStyle.Render("✓ " + f.statusMsg))
		}
	}

	return b.String()
}

func (f FriendsScreen) viewList() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Paired Devices"))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("Share passwords securely with trusted devices"))
	b.WriteString("\n\n")

	if f.selectedEntry != nil {
		b.WriteString(successStyle.Render(fmt.Sprintf("Selected: %s", f.selectedEntry.Website)))
		b.WriteString("\n\n")
	}

	if len(f.friends) == 0 {
		b.WriteString(mutedStyle.Render("No paired devices yet."))
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render("Go to 'Nearby' tab to discover and pair devices."))
	} else {
		for i, friend := range f.friends {
			cursor := "  "
			style := normalStyle
			if i == f.cursor {
				cursor = "▸ "
				style = selectedStyle
			}

			line := fmt.Sprintf("%s%s", cursor, style.Render(friend.Name))
			line += mutedStyle.Render(fmt.Sprintf(" [%s]", friend.Fingerprint[:8]))
			line += mutedStyle.Render(fmt.Sprintf(" • Added %s", friend.AddedAt.Format("Jan 02")))
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	if f.selectedEntry != nil {
		b.WriteString(helpStyle.Render("↑/↓ navigate • s share selected entry • d remove friend"))
	} else {
		b.WriteString(helpStyle.Render("↑/↓ navigate • d remove friend • select entry in Vault first to share"))
	}

	return b.String()
}

func (f FriendsScreen) viewShare() string {
	if len(f.friends) == 0 || f.selectedEntry == nil {
		return ""
	}

	friend := f.friends[f.cursor]
	var b strings.Builder

	b.WriteString(titleStyle.Render("Share Entry"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Share '%s' with %s?\n\n", f.selectedEntry.Website, friend.Name))
	b.WriteString("This action is E2E encrypted.\n")
	b.WriteString("Your data remains private and secure.\n")
	b.WriteString("Only the recipient can decrypt it.\n\n")
	b.WriteString(helpStyle.Render("y confirm • n cancel"))

	return boxStyle.Render(b.String())
}

func (f FriendsScreen) viewDelete() string {
	if len(f.friends) == 0 {
		return ""
	}

	friend := f.friends[f.cursor]
	var b strings.Builder

	b.WriteString(errorStyle.Render("Remove Friend?"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Remove '%s' from your paired devices?\n", friend.Name))
	b.WriteString("You won't be able to share with them until you pair again.\n\n")
	b.WriteString(helpStyle.Render("y confirm • n cancel"))

	return boxStyle.Render(b.String())
}

func (f *FriendsScreen) SetFriends(friends []models.Friend) {
	f.friends = friends
}

func (f *FriendsScreen) SetSelectedEntry(entry *models.Entry) {
	f.selectedEntry = entry
}

type SendShareMsg struct {
	Friend models.Friend
	Entry  models.Entry
}

type DeleteFriendMsg struct {
	Fingerprint string
}
