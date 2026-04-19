package printer

import (
	"fmt"
	"sync"
)

// Transport is the interface that serial and network transports implement.
type Transport interface {
	Write(data []byte) error
	Close()
	Name() string
	Type() string
}

// Manager holds the active transport and orchestrates printing.
type Manager struct {
	mu        sync.Mutex
	transport Transport
	paperMM   int
}

func NewManager(paperMM int) *Manager {
	if paperMM == 0 {
		paperMM = 80
	}
	return &Manager{paperMM: paperMM}
}

func (m *Manager) SetPaperWidth(mm int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.paperMM = mm
}

// ConfigureSerial replaces the active transport with a serial one.
func (m *Manager) ConfigureSerial(port string, baud int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.transport != nil {
		m.transport.Close()
		m.transport = nil
	}
	t := NewSerialTransport(port, baud)
	if err := t.Connect(); err != nil {
		return err
	}
	m.transport = t
	return nil
}

// ConfigureNetwork replaces the active transport with a network one.
func (m *Manager) ConfigureNetwork(ip string, port int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.transport != nil {
		m.transport.Close()
		m.transport = nil
	}
	if !PingNetwork(ip, port) {
		return fmt.Errorf("no se puede conectar a %s:%d — verifica que la impresora esté encendida y en la red", ip, port)
	}
	m.transport = NewNetworkTransport(ip, port)
	return nil
}

// ConfigureUSB replaces the active transport with a USB one.
func (m *Manager) ConfigureUSB(port string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.transport != nil {
		m.transport.Close()
		m.transport = nil
	}
	t := NewUSBTransport(port)
	if err := t.Connect(); err != nil {
		return err
	}
	m.transport = t
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

// TestPrint sends a test page.
func (m *Manager) TestPrint() error {
	m.mu.Lock()
	pw := m.paperMM
	m.mu.Unlock()
	bytes := BuildTestPrint(pw)
	return m.PrintBytes(bytes)
}

// Status returns current printer information.
type Status struct {
	Connected     bool   `json:"connected"`
	TransportType string `json:"transportType"`
	PrinterName   string `json:"printerName"`
	PaperWidthMM  int    `json:"paperWidthMM"`
	Version       string `json:"version"`
}

const BridgeVersion = "1.0.0"

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
		Connected:     true,
		TransportType: m.transport.Type(),
		PrinterName:   m.transport.Name(),
		PaperWidthMM:  m.paperMM,
		Version:       BridgeVersion,
	}
}
