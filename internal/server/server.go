package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"forgor/internal/crypto"
	"forgor/internal/models"
	"forgor/internal/storage"
)

type Server struct {
	httpServer *http.Server
	store      *storage.Store
	shareChan  chan models.IncomingShare
	port       int
}

func New(store *storage.Store, shareChan chan models.IncomingShare, port int) *Server {
	return &Server{
		store:     store,
		shareChan: shareChan,
		port:      port,
	}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/whoami", s.handleWhoAmI)
	mux.HandleFunc("/share", s.handleShare)

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	listener, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	go func() {
		if err := s.httpServer.Serve(listener); err != http.ErrServerClosed {
			fmt.Printf("HTTP server error: %v\n", err)
		}
	}()

	return nil
}

func (s *Server) Stop() error {
	if s.httpServer == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) Port() int {
	return s.port
}

func (s *Server) handleWhoAmI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Fetch device fresh from store (fails if locked)
	device, err := s.store.GetDevice()
	if err != nil {
		http.Error(w, "Vault is locked", http.StatusServiceUnavailable)
		return
	}

	response := models.WhoAmIResponse{
		DeviceName:  device.Name,
		PubKey:      device.PubKey,
		Fingerprint: device.Fingerprint(),
		Version:     "1",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleShare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Fetch device fresh from store (fails if locked - this is the atomic check)
	device, err := s.store.GetDevice()
	if err != nil {
		http.Error(w, "Vault is locked", http.StatusServiceUnavailable)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1024*1024)) // im limiting this to 1 MB (note here because im not gonna remember 1024*1024 in the morning)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var shareMsg models.ShareMessage
	if err := json.Unmarshal(body, &shareMsg); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	friend, err := s.store.GetFriend(shareMsg.FromFingerprint)
	if err != nil {
		http.Error(w, "Sender not paired", http.StatusForbidden)
		return
	}

	plaintext, err := crypto.BoxOpen(shareMsg.Ciphertext, &friend.PubKey, &device.PrivKey)
	if err != nil {
		http.Error(w, "Decryption failed", http.StatusBadRequest)
		return
	}

	var entry models.Entry
	if err := json.Unmarshal(plaintext, &entry); err != nil {
		http.Error(w, "Invalid entry data", http.StatusBadRequest)
		return
	}

	incoming := models.IncomingShare{
		FromFingerprint: shareMsg.FromFingerprint,
		FromName:        friend.Name,
		Entry:           entry,
	}

	select {
	case s.shareChan <- incoming:
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"status":"pending"}`))
	default:
		http.Error(w, "Busy", http.StatusServiceUnavailable)
	}
}

func FetchWhoAmI(host string, port int) (*models.WhoAmIResponse, error) {
	url := fmt.Sprintf("http://%s:%d/whoami", host, port)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var whoami models.WhoAmIResponse
	if err := json.NewDecoder(resp.Body).Decode(&whoami); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &whoami, nil
}

func SendShare(host string, port int, entry models.Entry, senderDevice *models.Device, recipientPubKey *[32]byte) error {
	plaintext, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to serialize entry: %w", err)
	}

	ciphertext, err := crypto.BoxSeal(plaintext, recipientPubKey, &senderDevice.PrivKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt: %w", err)
	}

	shareMsg := models.ShareMessage{
		FromFingerprint: senderDevice.Fingerprint(),
		Ciphertext:      ciphertext,
	}

	body, err := json.Marshal(shareMsg)
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	url := fmt.Sprintf("http://%s:%d/share", host, port)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func FetchAndPair(address string) (*models.Peer, error) {
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		host = address
		portStr = "8765"
	}

	var port int
	fmt.Sscanf(portStr, "%d", &port)
	if port == 0 {
		port = 8765
	}

	whoami, err := FetchWhoAmI(host, port)
	if err != nil {
		return nil, err
	}

	computedFp := models.ComputeFingerprint(whoami.PubKey[:])
	if computedFp != whoami.Fingerprint {
		return nil, fmt.Errorf("fingerprint mismatch")
	}

	return &models.Peer{
		Name:        whoami.DeviceName,
		Fingerprint: whoami.Fingerprint,
		Host:        host,
		Port:        port,
		PubKey:      whoami.PubKey,
	}, nil
}
