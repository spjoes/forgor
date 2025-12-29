package discovery

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"forgor/internal/models"

	"github.com/hashicorp/mdns"
)

func init() {
	// I hate how mDNS spams the Windows logs with "client closed" errors so im adding this to just discard them.
	log.SetOutput(io.Discard)
}

const (
	ServiceType = "_pwshare._tcp"
	Domain      = "local."
)

type Discovery struct {
	server       *mdns.Server
	deviceName   string
	fingerprint  string
	port         int
	peerChan     chan models.Peer
	stopBrowse   context.CancelFunc
}

func New(deviceName, fingerprint string, port int, peerChan chan models.Peer) *Discovery {
	return &Discovery{
		deviceName:  deviceName,
		fingerprint: fingerprint,
		port:        port,
		peerChan:    peerChan,
	}
}

func (d *Discovery) Start() error {
	if err := d.startAnnounce(); err != nil {
		return fmt.Errorf("failed to start mDNS announce: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	d.stopBrowse = cancel
	go d.browse(ctx)

	return nil
}

func (d *Discovery) Stop() {
	if d.stopBrowse != nil {
		d.stopBrowse()
	}
	if d.server != nil {
		d.server.Shutdown()
	}
}

func (d *Discovery) startAnnounce() error {
	host, err := getOutboundIP()
	if err != nil {
		host = "127.0.0.1"
	}

	info := []string{
		"pkfp=" + d.fingerprint,
		"v=1",
	}

	service, err := mdns.NewMDNSService(
		d.deviceName,
		ServiceType,
		Domain,
		"",
		d.port,
		[]net.IP{net.ParseIP(host)},
		info,
	)
	if err != nil {
		return fmt.Errorf("failed to create mDNS service: %w", err)
	}

	server, err := mdns.NewServer(&mdns.Config{
		Zone:       service,
		Iface:      nil,
		LogEmptyResponses: false,
	})
	if err != nil {
		return fmt.Errorf("failed to create mDNS server: %w", err)
	}

	d.server = server
	return nil
}

func (d *Discovery) browse(ctx context.Context) {
	entriesCh := make(chan *mdns.ServiceEntry, 10)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case entry := <-entriesCh:
				if entry == nil {
					continue
				}
				peer := d.entryToPeer(entry)
				if peer != nil && peer.Fingerprint != d.fingerprint {
					d.peerChan <- *peer
				}
			}
		}
	}()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	d.query(entriesCh)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.query(entriesCh)
		}
	}
}

func (d *Discovery) query(entriesCh chan *mdns.ServiceEntry) {

	// As much as I want to use ipv6, windows has some issues with it and its just not supported by this library
	// maybe one day ill go and see if theres a sort of fix or work around but we're gonna keep it as ipv4 only for now
	// the future is not now...
	params := &mdns.QueryParam{
		Service:             ServiceType,
		Domain:              Domain,
		Timeout:             3 * time.Second,
		Entries:             entriesCh,
		WantUnicastResponse: false,
		DisableIPv6:         true,
	}

	if err := mdns.Query(params); err != nil {
		if !strings.Contains(err.Error(), "not supported") {
			log.Printf("mDNS query failed: %v", err)
		}
	}
}

func (d *Discovery) entryToPeer(entry *mdns.ServiceEntry) *models.Peer {
	if entry == nil {
		return nil
	}

	var fingerprint string
	for _, txt := range entry.InfoFields {
		if strings.HasPrefix(txt, "pkfp=") {
			fingerprint = strings.TrimPrefix(txt, "pkfp=")
			break
		}
	}

	if fingerprint == "" {
		return nil
	}

	var host string
	if entry.AddrV4 != nil {
		host = entry.AddrV4.String()
	} else if entry.AddrV6 != nil {
		host = entry.AddrV6.String()
	}

	if host == "" {
		return nil
	}

	return &models.Peer{
		Name:        entry.Name,
		Fingerprint: fingerprint,
		Host:        host,
		Port:        entry.Port,
	}
}

func getOutboundIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

func (d *Discovery) Refresh() {
	entriesCh := make(chan *mdns.ServiceEntry, 10)
	go func() {
		for entry := range entriesCh {
			if entry == nil {
				continue
			}
			peer := d.entryToPeer(entry)
			if peer != nil && peer.Fingerprint != d.fingerprint {
				d.peerChan <- *peer
			}
		}
	}()
	d.query(entriesCh)
}
