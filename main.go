package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/kardianos/service"
	"zenpos-print-bridge/config"
	"zenpos-print-bridge/printer"
	srv "zenpos-print-bridge/server"
)

const (
	svcName        = "zenposPrintBridge"
	svcDisplayName = "zenPOS Print Bridge"
	svcDescription = "Permite imprimir desde zenPOS en cualquier impresora térmica USB o de red."
)

// program implements service.Interface
type program struct {
	cfg     *config.Config
	manager *printer.Manager
	server  *srv.Server
}

func (p *program) Start(s service.Service) error {
	go p.run()
	return nil
}

func (p *program) run() {
	// Reconnect last known transport on startup
	switch p.cfg.Transport {
	case config.TransportSerial:
		if p.cfg.SerialPort != "" {
			if err := p.manager.ConfigureSerial(p.cfg.SerialPort, p.cfg.SerialBaud); err != nil {
				log.Printf("advertencia: no se pudo reconectar serial %s: %v", p.cfg.SerialPort, err)
			} else {
				log.Printf("reconectado a puerto serial: %s", p.cfg.SerialPort)
			}
		}
	case config.TransportNetwork:
		if p.cfg.NetworkIP != "" {
			if err := p.manager.ConfigureNetwork(p.cfg.NetworkIP, p.cfg.NetworkPort); err != nil {
				log.Printf("advertencia: no se pudo reconectar red %s:%d: %v", p.cfg.NetworkIP, p.cfg.NetworkPort, err)
			} else {
				log.Printf("reconectado a impresora de red: %s:%d", p.cfg.NetworkIP, p.cfg.NetworkPort)
			}
		}
	case config.TransportUSB:
		if p.cfg.USBPort != "" {
			if err := p.manager.ConfigureUSB(p.cfg.USBPort); err != nil {
				log.Printf("advertencia: no se pudo reconectar USB %s: %v", p.cfg.USBPort, err)
			} else {
				log.Printf("reconectado a impresora USB: %s", p.cfg.USBPort)
			}
		}
	}

	if err := p.server.Start(); err != nil {
		log.Printf("servidor HTTP terminó: %v", err)
	}
}

func (p *program) Stop(s service.Service) error {
	p.server.Stop()
	p.manager.Disconnect()
	return nil
}

func setupLogger() {
	// Prefer PROGRAMDATA (shared — works under LocalSystem service account).
	// Fallback to APPDATA for interactive/non-admin execution.
	baseDir := os.Getenv("PROGRAMDATA")
	if baseDir == "" {
		baseDir = os.Getenv("APPDATA")
	}
	if baseDir == "" {
		return
	}
	logDir := filepath.Join(baseDir, "zenPOS", "PrintBridge")
	os.MkdirAll(logDir, 0755)
	logPath := filepath.Join(logDir, "bridge.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	log.SetOutput(f)
}

func main() {
	action := flag.String("action", "", "install | uninstall | run (default: run como servicio si disponible)")
	flag.Parse()

	// Setup logger with timestamps in both modes
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[bridge] ")
	if !service.Interactive() {
		setupLogger()
	}

	cfg, err := config.Load()
	if err != nil {
		log.Printf("advertencia: error cargando config: %v — usando defaults", err)
	}

	mgr := printer.NewManager(cfg.PaperWidth)
	server := srv.New(mgr, cfg)

	prg := &program{cfg: cfg, manager: mgr, server: server}

	svcConfig := &service.Config{
		Name:        svcName,
		DisplayName: svcDisplayName,
		Description: svcDescription,
		Option: service.KeyValue{
			"StartType": "automatic",
		},
	}

	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatalf("error creando servicio: %v", err)
	}

	switch *action {
	case "install":
		if err := s.Install(); err != nil {
			fmt.Fprintf(os.Stderr, "Error instalando servicio: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Servicio instalado correctamente.")
		if err := s.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Advertencia: error iniciando servicio: %v\n", err)
		} else {
			fmt.Println("Servicio iniciado.")
		}

	case "uninstall":
		s.Stop()
		if err := s.Uninstall(); err != nil {
			fmt.Fprintf(os.Stderr, "Error desinstalando servicio: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Servicio desinstalado.")

	case "run", "":
		if service.Interactive() {
			fmt.Printf("zenPOS Print Bridge v%s — http://localhost:%d\n", printer.BridgeVersion, cfg.HTTPPort)
			fmt.Println("Presiona Ctrl+C para detener.")
		}
		if err := s.Run(); err != nil {
			log.Fatalf("error ejecutando servicio: %v", err)
		}

	default:
		fmt.Fprintf(os.Stderr, "Acción desconocida: %s\n (usa: install | uninstall | run)\n", *action)
		os.Exit(1)
	}
}
