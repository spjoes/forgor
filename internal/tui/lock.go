package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type LockScreen struct {
	passwordInput textinput.Model
	confirmInput  textinput.Model
	deviceInput   textinput.Model
	isNewVault    bool
	focusIndex    int
	err           string
	loading       bool
}

func NewLockScreen(isNewVault bool) LockScreen {
	password := textinput.New()
	password.Placeholder = "Master password"
	password.EchoMode = textinput.EchoPassword
	password.EchoCharacter = '•'
	password.Focus()
	password.Width = 40

	confirm := textinput.New()
	confirm.Placeholder = "Confirm password"
	confirm.EchoMode = textinput.EchoPassword
	confirm.EchoCharacter = '•'
	confirm.Width = 40

	device := textinput.New()
	device.Placeholder = "Device name (e.g., laptop, phone)"
	device.Width = 40

	return LockScreen{
		passwordInput: password,
		confirmInput:  confirm,
		deviceInput:   device,
		isNewVault:    isNewVault,
	}
}

func (l LockScreen) Init() tea.Cmd {
	return textinput.Blink
}

func (l LockScreen) Update(msg tea.Msg) (LockScreen, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		l.err = ""

		switch msg.String() {
		case "tab", "shift+tab", "down", "up":
			if l.isNewVault {
				if msg.String() == "tab" || msg.String() == "down" {
					l.focusIndex = (l.focusIndex + 1) % 3
				} else {
					l.focusIndex = (l.focusIndex + 2) % 3
				}
				l.updateFocus()
			}
			return l, nil

		case "enter":
			if l.loading {
				return l, nil
			}

			password := l.passwordInput.Value()

			if l.isNewVault {
				confirm := l.confirmInput.Value()
				deviceName := strings.TrimSpace(l.deviceInput.Value())

				if password == "" {
					l.err = "Password is required"
					return l, nil
				}
				if len(password) < 8 {
					l.err = "Password must be at least 8 characters"
					return l, nil
				}
				if password != confirm {
					l.err = "Passwords do not match"
					return l, nil
				}
				if deviceName == "" {
					l.err = "Device name is required"
					return l, nil
				}

				l.loading = true
				return l, func() tea.Msg {
					return InitRequestMsg{Password: password, DeviceName: deviceName}
				}
			} else {
				if password == "" {
					l.err = "Password is required"
					return l, nil
				}

				l.loading = true
				return l, func() tea.Msg {
					return UnlockRequestMsg{Password: password}
				}
			}
		}
	}

	var cmd tea.Cmd
	if l.isNewVault {
		switch l.focusIndex {
		case 0:
			l.passwordInput, cmd = l.passwordInput.Update(msg)
		case 1:
			l.confirmInput, cmd = l.confirmInput.Update(msg)
		case 2:
			l.deviceInput, cmd = l.deviceInput.Update(msg)
		}
	} else {
		l.passwordInput, cmd = l.passwordInput.Update(msg)
	}
	cmds = append(cmds, cmd)

	return l, tea.Batch(cmds...)
}

func (l *LockScreen) updateFocus() {
	l.passwordInput.Blur()
	l.confirmInput.Blur()
	l.deviceInput.Blur()

	switch l.focusIndex {
	case 0:
		l.passwordInput.Focus()
	case 1:
		l.confirmInput.Focus()
	case 2:
		l.deviceInput.Focus()
	}
}

func (l LockScreen) View() string {
	var b strings.Builder

	b.WriteString(logoStyle.Render(logo))
	b.WriteString("\n")

	if l.isNewVault {
		b.WriteString(titleStyle.Render("Create New Vault"))
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render("Set up your master password to get started"))
		b.WriteString("\n\n")

		b.WriteString("Master Password:\n")
		if l.focusIndex == 0 {
			b.WriteString(focusedInputStyle.Render(l.passwordInput.View()))
		} else {
			b.WriteString(inputStyle.Render(l.passwordInput.View()))
		}
		b.WriteString("\n\n")

		b.WriteString("Confirm Password:\n")
		if l.focusIndex == 1 {
			b.WriteString(focusedInputStyle.Render(l.confirmInput.View()))
		} else {
			b.WriteString(inputStyle.Render(l.confirmInput.View()))
		}
		b.WriteString("\n\n")

		b.WriteString("Device Name:\n")
		if l.focusIndex == 2 {
			b.WriteString(focusedInputStyle.Render(l.deviceInput.View()))
		} else {
			b.WriteString(inputStyle.Render(l.deviceInput.View()))
		}
		b.WriteString("\n")
	} else {
		b.WriteString(titleStyle.Render("Unlock Vault"))
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render("Enter your master password"))
		b.WriteString("\n\n")

		b.WriteString("Master Password:\n")
		b.WriteString(focusedInputStyle.Render(l.passwordInput.View()))
		b.WriteString("\n")
	}

	if l.err != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render("⚠ " + l.err))
	}

	if l.loading {
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render("Unlocking..."))
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Press Enter to submit • Ctrl+C to quit"))

	return boxStyle.Render(b.String())
}

type UnlockRequestMsg struct {
	Password string
}

type InitRequestMsg struct {
	Password   string
	DeviceName string
}

func (l *LockScreen) SetError(err string) {
	l.err = err
	l.loading = false
}

func (l *LockScreen) Reset() {
	l.passwordInput.SetValue("")
	l.confirmInput.SetValue("")
	l.err = ""
	l.loading = false
	l.focusIndex = 0
	l.updateFocus()
}
