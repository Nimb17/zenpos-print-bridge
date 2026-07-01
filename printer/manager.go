package printer

import (
	"fmt"
	"sync"
	"time"
)

// Transport is the interface that serial, network and USB transports implement.
type Transport interface {
	Write(data []byte) error
	Close()
	Name() string
	Type() string
	// Ping reports whether the printer is actually reachable right now.
	// A nil return means reachable; a non-nil error describes why not.
	Ping() error
}

// Manager holds the active transport and orchestrates printing.
type Manager struct {
	mu        sync.Mutex
	transport Transport
	paperMM   int
	reachable bool
}

func NewManager(paperMM int) *Manager {
	if paperMM == 0 {
		paperMM = 80
	}
	m := &Manager{paperMM: paperMM}
	go m.healthLoop()
	return m
}

// healthLoop periodically refreshes reachability so Status() stays fast: it
// only ever returns the cached value and never blocks on printer I/O.
func (m *Manager) healthLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		m.refreshReachable()
	}
}

// refreshReachable pings the active transport and caches the result. The ping
// runs without holding the lock so a slow network probe never blocks concurrent
// Status()/print requests.
func (m *Manager) refreshReachable() {
	m.mu.Lock()
	t := m.transport
	m.mu.Unlock()
	reachable := t != nil && t.Ping() == nil
	m.mu.Lock()
	m.reachable = reachable
	m.mu.Unlock()
}

// swapTransport installs a new transport (closing any previous one) and
// immediately refreshes reachability so the first Status() after configure
// reflects the real state instead of waiting for the next poll tick.
func (m *Manager) swapTransport(t Transport) {
	m.mu.Lock()
	if m.transport != nil {
		m.transport.Close()
	}
	m.transport = t
	m.mu.Unlock()
	m.refreshReachable()
}

func (m *Manager) SetPaperWidth(mm int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.paperMM = mm
}

// ConfigureSerial replaces the active transport with a serial one.
func (m *Manager) ConfigureSerial(port string, baud int) error {
	t := NewSerialTransport(port, baud)
	if err := t.Connect(); err != nil {
		return err
	}
	m.swapTransport(t)
	return nil
}

// ConfigureNetwork replaces the active transport with a network one.
func (m *Manager) ConfigureNetwork(ip string, port int) error {
	if !PingNetwork(ip, port) {
		return fmt.Errorf("no se puede conectar a %s:%d — verifica que la impresora esté encendida y en la red", ip, port)
	}
	m.swapTransport(NewNetworkTransport(ip, port))
	return nil
}

// ConfigureUSB replaces the active transport with a USB one.
func (m *Manager) ConfigureUSB(port string) error {
	t := NewUSBTransport(port)
	if err := t.Connect(); err != nil {
		return err
	}
	m.swapTransport(t)
	return nil
}

// Disconnect clears the active transport.
func (m *Manager) Disconnect() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.transport != nil {
		m.transport.Close()
		m.transport = nil
	}
	m.reachable = false
}

// PrintBytes sends raw bytes to the active transport.
func (m *Manager) PrintBytes(data []byte) error {
	m.mu.Lock()
	t := m.transport
	m.mu.Unlock()
	if t == nil {
		return fmt.Errorf("no hay impresora configurada")
	}
	return t.Write(data)
}

// PrintReceipt builds and sends a receipt.
func (m *Manager) PrintReceipt(r ReceiptData) error {
	m.mu.Lock()
	pw := m.paperMM
	m.mu.Unlock()
	if r.PaperWidth > 0 {
		pw = r.PaperWidth
	}
	r.PaperWidth = pw
	bytes := BuildReceipt(r)
	return m.PrintBytes(bytes)
}

// TestPrint sends a test page. It first verifies the printer is reachable so it
// won't report success when the printer is offline (e.g. powered off), which is
// the whole point of a test print.
func (m *Manager) TestPrint() error {
	m.mu.Lock()
	t := m.transport
	pw := m.paperMM
	m.mu.Unlock()
	if t == nil {
		return fmt.Errorf("no hay impresora configurada")
	}
	if err := t.Ping(); err != nil {
		return fmt.Errorf("la impresora no está disponible: %w", err)
	}
	return t.Write(BuildTestPrint(pw))
}

// Status returns current printer information.
type Status struct {
	Connected     bool   `json:"connected"`
	TransportType string `json:"transportType"`
	PrinterName   string `json:"printerName"`
	PaperWidthMM  int    `json:"paperWidthMM"`
	Version       string `json:"version"`
}

const BridgeVersion = "1.1.1"

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.transport == nil {
		return Status{
			Connected:    false,
			PaperWidthMM: m.paperMM,
			Version:      BridgeVersion,
		}
	}
	return Status{
		Connected:     m.reachable,
		TransportType: m.transport.Type(),
		PrinterName:   m.transport.Name(),
		PaperWidthMM:  m.paperMM,
		Version:       BridgeVersion,
	}
}
