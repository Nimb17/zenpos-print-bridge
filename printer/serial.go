package printer

import (
	"fmt"
	"strings"
	"time"

	"go.bug.st/serial"
)

// SerialTransport sends ESC/POS bytes over a COM port (USB or Bluetooth Serial).
type SerialTransport struct {
	port     serial.Port
	portName string
	baudRate int
}

func NewSerialTransport(portName string, baudRate int) *SerialTransport {
	if baudRate == 0 {
		baudRate = 9600
	}
	return &SerialTransport{portName: portName, baudRate: baudRate}
}

func (s *SerialTransport) Connect() error {
	mode := &serial.Mode{
		BaudRate: s.baudRate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}
	port, err := serial.Open(s.portName, mode)
	if err != nil {
		return fmt.Errorf("abrir puerto %s: %w", s.portName, err)
	}
	if err := port.SetReadTimeout(2 * time.Second); err != nil {
		port.Close()
		return fmt.Errorf("configurar timeout: %w", err)
	}
	s.port = port
	return nil
}

func (s *SerialTransport) Write(data []byte) error {
	if s.port == nil {
		if err := s.Connect(); err != nil {
			return err
		}
	}
	if err := s.writeChunks(data); err != nil {
		// First write failed — reconnect once and retry
		s.port.Close()
		s.port = nil
		if reconnErr := s.Connect(); reconnErr != nil {
			return fmt.Errorf("escribir en %s falló y no se pudo reconectar: %w", s.portName, err)
		}
		if retryErr := s.writeChunks(data); retryErr != nil {
			s.port.Close()
			s.port = nil
			return fmt.Errorf("escribir en %s tras reconexión: %w", s.portName, retryErr)
		}
	}
	return nil
}

func (s *SerialTransport) writeChunks(data []byte) error {
	const chunkSize = 512
	for i := 0; i < len(data); i += chunkSize {
		end := i + chunkSize
		if end > len(data) {
			end = len(data)
		}
		_, err := s.port.Write(data[i:end])
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SerialTransport) Close() {
	if s.port != nil {
		s.port.Close()
		s.port = nil
	}
}

func (s *SerialTransport) Name() string {
	return s.portName
}

func (s *SerialTransport) Type() string {
	return "serial"
}

// ListSerialPorts returns available COM ports on the system.
func ListSerialPorts() ([]PortInfo, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil, fmt.Errorf("listar puertos: %w", err)
	}
	var result []PortInfo
	for _, p := range ports {
		label := p
		lp := strings.ToLower(p)
		if strings.Contains(lp, "bluetooth") || strings.Contains(lp, "bt") {
			label = p + " (Bluetooth)"
		}
		result = append(result, PortInfo{Name: p, Label: label, Type: "serial"})
	}
	return result, nil
}

// PortInfo holds displayable info about a port.
type PortInfo struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Type  string `json:"type"`
}
