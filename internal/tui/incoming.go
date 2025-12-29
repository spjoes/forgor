package tui

import (
	"fmt"
	"strings"

	"forgor/internal/models"

	tea "github.com/charmbracelet/bubbletea"
)

type IncomingShareScreen struct {
	share   *models.IncomingShare
	visible bool
}

func NewIncomingShareScreen() IncomingShareScreen {
	return IncomingShareScreen{}
}

func (s IncomingShareScreen) Init() tea.Cmd {
	return nil
}

func (s IncomingShareScreen) Update(msg tea.Msg) (IncomingShareScreen, tea.Cmd) {
	switch msg := msg.(type) {
	case IncomingShareMsg:
		s.share = &msg.Share
		s.visible = true
		return s, nil

	case tea.KeyMsg:
		if !s.visible {
			return s, nil
		}

		switch msg.String() {
		case "y", "Y":
			if s.share != nil {
				share := *s.share
				s.visible = false
				s.share = nil
				return s, func() tea.Msg {
					return AcceptShareMsg{Share: share}
				}
			}
		case "n", "N", "esc":
			s.visible = false
			s.share = nil
		}
	}

	return s, nil
}

func (s IncomingShareScreen) View() string {
	if !s.visible || s.share == nil {
		return ""
	}

	var b strings.Builder

	b.WriteString(titleStyle.Render("You got a share request!"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("%s wants to share their %s login with you\n\n", s.share.FromName, s.share.Entry.Website))
	b.WriteString(fmt.Sprintf("Username: %s\n", s.share.Entry.Username))
	b.WriteString("\n")
	b.WriteString("Would you like to accept this login into your vault?\n")
	b.WriteString("This action is E2E encrypted. Your data remains private and secure.\n\n")
	b.WriteString(helpStyle.Render("y accept • n decline"))

	return boxStyle.Render(b.String())
}

func (s IncomingShareScreen) IsVisible() bool {
	return s.visible
}

type AcceptShareMsg struct {
	Share models.IncomingShare
}
