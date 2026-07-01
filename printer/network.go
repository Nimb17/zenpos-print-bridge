package printer

import (
	"fmt"
	"net"
	"time"
)

// NetworkTransport sends ESC/POS bytes directly via TCP to a printer's IP:port (usually port 9100).
type NetworkTransport struct {
	ip   string
	port int
}

func NewNetworkTransport(ip string, port int) *NetworkTransport {
	if port == 0 {
		port = 9100
	}
	return &NetworkTransport{ip: ip, port: port}
}

func (n *NetworkTransport) Write(data []byte) error {
	addr := net.JoinHostPort(n.ip, fmt.Sprintf("%d", n.port))
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("conectar a %s: %w", addr, err)
	}
	defer conn.Close()

	if err := conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return fmt.Errorf("configurar deadline: %w", err)
	}

	const chunkSize = 1024
	for i := 0; i < len(data); i += chunkSize {
		end := i + chunkSize
		if end > len(data) {
			end = len(data)
		}
		_, err := conn.Write(data[i:end])
		if err != nil {
			return fmt.Errorf("escribir en %s: %w", addr, err)
		}
	}
	return nil
}

// Ping reports whether the printer answers at its IP:port right now.
func (n *NetworkTransport) Ping() error {
	if PingNetwork(n.ip, n.port) {
		return nil
	}
	return fmt.Errorf("sin respuesta de %s:%d", n.ip, n.port)
}

func (n *NetworkTransport) Close() {}

func (n *NetworkTransport) Name() string {
	return fmt.Sprintf("%s:%d", n.ip, n.port)
}

func (n *NetworkTransport) Type() string {
	return "network"
}

// PingNetwork checks if a printer responds at the given IP:port.
func PingNetwork(ip string, port int) bool {
	if port == 0 {
		port = 9100
	}
	addr := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
