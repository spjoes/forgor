package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"forgor/internal/discovery"
	"forgor/internal/models"
	"forgor/internal/server"
	"forgor/internal/storage"
	"forgor/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

var (
	portFlag   = flag.Int("port", 8765, "HTTP server port for sharing")
	dbPathFlag = flag.String("db", "", "Custom database path (for testing multiple instances)")
)

func main() {
	flag.Parse()

	var dbPath string
	var err error
	if *dbPathFlag != "" {
		dbPath = *dbPathFlag
	} else {
		dbPath, err = getDBPath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	store, err := storage.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	peerChan := make(chan models.Peer, 10)
	shareChan := make(chan models.IncomingShare, 10)

	app := tui.NewApp(store, peerChan, shareChan, *portFlag)

	p := tea.NewProgram(app, tea.WithAltScreen())

	var disc *discovery.Discovery
	var srv *server.Server

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for range ticker.C {
			if store.IsUnlocked() {
				device, err := store.GetDevice()
				if err != nil {
					continue
				}

				srv = server.New(store, shareChan, *portFlag)
				if err := srv.Start(); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: Failed to start server: %v\n", err)
				}

				disc = discovery.New(device.Name, device.Fingerprint(), *portFlag, peerChan)
				if err := disc.Start(); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: Failed to start discovery: %v\n", err)
				}

				break
			}
		}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if disc != nil {
		disc.Stop()
	}
	if srv != nil {
		srv.Stop()
	}
}

func getDBPath() (string, error) {
	var dataDir string

	switch runtime.GOOS {
	case "windows":
		dataDir = os.Getenv("APPDATA")
		if dataDir == "" {
			dataDir = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dataDir = filepath.Join(home, "Library", "Application Support")
	default:
		dataDir = os.Getenv("XDG_DATA_HOME")
		if dataDir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			dataDir = filepath.Join(home, ".local", "share")
		}
	}

	forgorDir := filepath.Join(dataDir, "forgor")
	return filepath.Join(forgorDir, "vault.db"), nil
}
