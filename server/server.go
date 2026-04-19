package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/rs/cors"
	"zenpos-print-bridge/config"
	"zenpos-print-bridge/printer"
)

const maxBodySize = 1 << 20 // 1 MB

type Server struct {
	manager *printer.Manager
	cfg     *config.Config
	httpSrv *http.Server
}

func New(manager *printer.Manager, cfg *config.Config) *Server {
	return &Server{manager: manager, cfg: cfg}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /status", s.handleStatus)
	mux.HandleFunc("GET /printers", s.handlePrinters)
	mux.HandleFunc("POST /print", s.handlePrint)
	mux.HandleFunc("POST /configure", s.handleConfigure)
	mux.HandleFunc("GET /test", s.handleTest)
	mux.HandleFunc("POST /disconnect", s.handleDisconnect)

	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: false,
	})
	return c.Handler(mux)
}

func (s *Server) Start() error {
	addr := fmt.Sprintf("127.0.0.1:%d", s.cfg.HTTPPort)
	s.httpSrv = &http.Server{
		Addr:         addr,
		Handler:      s.routes(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	log.Printf("zenPOS Print Bridge escuchando en http://%s", addr)
	return s.httpSrv.ListenAndServe()
}

func (s *Server) Stop() {
	if s.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.httpSrv.Shutdown(ctx)
	}
}

// GET /status
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.manager.Status())
}

// GET /printers
// Returns: { installed: [...], serialPorts: [...] }
// - installed: Windows-installed printers (use printerName with type=usb)
// - serialPorts: COM ports (use port with type=serial)
func (s *Server) handlePrinters(w http.ResponseWriter, r *http.Request) {
	serialPorts, err := printer.ListSerialPorts()
	if err != nil {
		serialPorts = nil
	}
	installed, err := printer.ListInstalledPrinters()
	if err != nil {
		installed = []printer.InstalledPrinter{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"installed":   installed,
		"serialPorts": serialPorts,
	})
}

// POST /print
// Body: { "bytes": "<base64>" }
// OR:   { "receipt": { ...ReceiptData fields... } }
func (s *Server) handlePrint(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	var body struct {
		Bytes   string               `json:"bytes"`
		Receipt *printer.ReceiptData `json:"receipt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return
	}

	var printErr error

	if body.Receipt != nil {
		printErr = s.manager.PrintReceipt(*body.Receipt)
	} else if body.Bytes != "" {
		data, err := base64.StdEncoding.DecodeString(body.Bytes)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bytes base64 inválidos: "+err.Error())
			return
		}
		printErr = s.manager.PrintBytes(data)
	} else {
		writeError(w, http.StatusBadRequest, "se requiere 'bytes' (base64) o 'receipt' (objeto)")
		return
	}

	if printErr != nil {
		writeError(w, http.StatusInternalServerError, printErr.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /configure
// Body serial:  { "type": "serial", "port": "COM3", "baud": 9600, "paperWidthMM": 80 }
// Body network: { "type": "network", "ip": "192.168.1.50", "port": 9100, "paperWidthMM": 80 }
func (s *Server) handleConfigure(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	var body struct {
		Type         string `json:"type"`
		Port         string `json:"port"`
		Baud         int    `json:"baud"`
		IP           string `json:"ip"`
		NetworkPort  int    `json:"networkPort"`
		USBPort      string `json:"usbPort"`
		PaperWidthMM int    `json:"paperWidthMM"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return
	}

	if body.PaperWidthMM > 0 {
		s.manager.SetPaperWidth(body.PaperWidthMM)
		s.cfg.PaperWidth = body.PaperWidthMM
	}

	switch body.Type {
	case "serial":
		if body.Port == "" {
			writeError(w, http.StatusBadRequest, "se requiere 'port' (ej: COM3)")
			return
		}
		baud := body.Baud
		if baud == 0 {
			baud = 9600
		}
		if err := s.manager.ConfigureSerial(body.Port, baud); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.cfg.Transport = config.TransportSerial
		s.cfg.SerialPort = body.Port
		s.cfg.SerialBaud = baud

	case "network":
		if body.IP == "" {
			writeError(w, http.StatusBadRequest, "se requiere 'ip'")
			return
		}
		netPort := body.NetworkPort
		if netPort == 0 {
			netPort = 9100
		}
		if err := s.manager.ConfigureNetwork(body.IP, netPort); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.cfg.Transport = config.TransportNetwork
		s.cfg.NetworkIP = body.IP
		s.cfg.NetworkPort = netPort

	case "usb":
		usbPort := body.USBPort
		if usbPort == "" {
			usbPort = body.Port // allow "port" field as fallback
		}
		if usbPort == "" {
			writeError(w, http.StatusBadRequest, "se requiere 'usbPort' (ej: USB002)")
			return
		}
		if err := s.manager.ConfigureUSB(usbPort); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.cfg.Transport = config.TransportUSB
		s.cfg.USBPort = usbPort

	default:
		writeError(w, http.StatusBadRequest, "tipo inválido — usar 'serial', 'network' o 'usb'")
		return
	}

	if err := s.cfg.Save(); err != nil {
		log.Printf("advertencia: no se pudo guardar config: %v", err)
	}
	writeJSON(w, http.StatusOK, s.manager.Status())
}

// GET /test
func (s *Server) handleTest(w http.ResponseWriter, r *http.Request) {
	if err := s.manager.TestPrint(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /disconnect
func (s *Server) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	s.manager.Disconnect()
	s.cfg.Transport = config.TransportNone
	s.cfg.SerialPort = ""
	s.cfg.NetworkIP = ""
	if err := s.cfg.Save(); err != nil {
		log.Printf("advertencia: no se pudo guardar config: %v", err)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
