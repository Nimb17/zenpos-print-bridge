package printer

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

// Windows Spooler API (winspool.drv)
var (
	winspool             = syscall.NewLazyDLL("winspool.drv")
	procOpenPrinterW     = winspool.NewProc("OpenPrinterW")
	procClosePrinter     = winspool.NewProc("ClosePrinter")
	procStartDocPrinterW = winspool.NewProc("StartDocPrinterW")
	procEndDocPrinter    = winspool.NewProc("EndDocPrinter")
	procStartPagePrinter = winspool.NewProc("StartPagePrinter")
	procEndPagePrinter   = winspool.NewProc("EndPagePrinter")
	procWritePrinter     = winspool.NewProc("WritePrinter")
	procEnumPrintersW    = winspool.NewProc("EnumPrintersW")
	procGetPrinterW      = winspool.NewProc("GetPrinterW")
)

// PRINTER_INFO_2 status/attribute flags used to detect an unavailable printer.
const (
	printerAttributeWorkOffline = 0x00000400 // user/system marked "Use Printer Offline"
	printerStatusOffline        = 0x00000080
	printerStatusError          = 0x00000002
	printerStatusNotAvailable   = 0x00001000
)

// PRINTER_INFO_2W layout (pointer-size fields first, then DWORDs)
// We only read a few fields we need — rest are skipped as uintptrs.
type printerInfo2W struct {
	PServerName         *uint16
	PPrinterName        *uint16
	PShareName          *uint16
	PPortName           *uint16
	PDriverName         *uint16
	PComment            *uint16
	PLocation           *uint16
	PDevMode            uintptr
	PSepFile            *uint16
	PPrintProcessor     *uint16
	PDatatype           *uint16
	PParameters         *uint16
	PSecurityDescriptor uintptr
	Attributes          uint32
	Priority            uint32
	DefaultPriority     uint32
	StartTime           uint32
	UntilTime           uint32
	Status              uint32
	CJobs               uint32
	AveragePPM          uint32
}

const (
	printerEnumLocal       = 0x00000002
	printerEnumConnections = 0x00000004
)

// DOC_INFO_1 for StartDocPrinter
type docInfo1 struct {
	pDocName    *uint16
	pOutputFile *uint16
	pDatatype   *uint16
}

// USBTransport writes raw ESC/POS bytes to a Windows printer via the Spooler API.
// The printerName is the Windows printer name (e.g. "POS-58-Raw").
type USBTransport struct {
	printerName string
}

func NewUSBTransport(printerName string) *USBTransport {
	return &USBTransport{printerName: strings.TrimSpace(printerName)}
}

func (u *USBTransport) Connect() error {
	// Verify the printer exists by opening and immediately closing
	handle, err := openPrinter(u.printerName)
	if err != nil {
		return fmt.Errorf("no se pudo abrir la impresora %q: %w — verifica que esté encendida y conectada", u.printerName, err)
	}
	closePrinter(handle)
	return nil
}

func (u *USBTransport) Write(data []byte) error {
	handle, err := openPrinter(u.printerName)
	if err != nil {
		return fmt.Errorf("no se pudo abrir la impresora %q: %w", u.printerName, err)
	}
	defer closePrinter(handle)

	docName, _ := syscall.UTF16PtrFromString("zenPOS Receipt")
	dataType, _ := syscall.UTF16PtrFromString("RAW")

	di := docInfo1{
		pDocName:    docName,
		pOutputFile: nil,
		pDatatype:   dataType,
	}

	ret, _, _ := procStartDocPrinterW.Call(handle, 1, uintptr(unsafe.Pointer(&di)))
	if ret == 0 {
		return fmt.Errorf("StartDocPrinter falló para %q", u.printerName)
	}
	defer procEndDocPrinter.Call(handle)

	ret, _, _ = procStartPagePrinter.Call(handle)
	if ret == 0 {
		return fmt.Errorf("StartPagePrinter falló para %q", u.printerName)
	}
	defer procEndPagePrinter.Call(handle)

	// Write in chunks
	const chunkSize = 4096
	for i := 0; i < len(data); i += chunkSize {
		end := i + chunkSize
		if end > len(data) {
			end = len(data)
		}
		chunk := data[i:end]
		var written uint32
		ret, _, _ = procWritePrinter.Call(
			handle,
			uintptr(unsafe.Pointer(&chunk[0])),
			uintptr(len(chunk)),
			uintptr(unsafe.Pointer(&written)),
		)
		if ret == 0 {
			return fmt.Errorf("WritePrinter falló en offset %d para %q", i, u.printerName)
		}
	}
	return nil
}

// Ping reports whether the Windows printer is actually available. It reads
// PRINTER_INFO_2 via GetPrinter and treats the WORK_OFFLINE attribute and the
// OFFLINE/NOT_AVAILABLE/ERROR status flags as "not reachable". This catches the
// common case where a USB printer is powered off — Windows sets WorkOffline —
// so the bridge stops reporting a false "connected" and /test stops lying.
//
// Note: some cheap thermal printers on a write-only RAW/USB port never report
// offline at all; for those Windows itself cannot tell, and neither can we.
func (u *USBTransport) Ping() error {
	handle, err := openPrinter(u.printerName)
	if err != nil {
		return fmt.Errorf("no se pudo abrir la impresora %q: %w", u.printerName, err)
	}
	defer closePrinter(handle)

	// First call with a nil buffer to learn the required size.
	var needed uint32
	procGetPrinterW.Call(handle, 2, 0, 0, uintptr(unsafe.Pointer(&needed)))
	if needed == 0 {
		return nil // can't determine — don't report a false offline
	}
	buf := make([]byte, needed)
	ret, _, _ := procGetPrinterW.Call(
		handle,
		2, // level 2 = PRINTER_INFO_2
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(needed),
		uintptr(unsafe.Pointer(&needed)),
	)
	if ret == 0 {
		return nil // couldn't read status — assume ok rather than false-negative
	}
	info := (*printerInfo2W)(unsafe.Pointer(&buf[0]))
	if info.Attributes&printerAttributeWorkOffline != 0 {
		return fmt.Errorf("la impresora está en modo sin conexión")
	}
	if info.Status&(printerStatusOffline|printerStatusNotAvailable|printerStatusError) != 0 {
		return fmt.Errorf("la impresora no está disponible")
	}
	return nil
}

func (u *USBTransport) Close() {
	// Spooler is opened/closed per write — nothing to persist
}

func (u *USBTransport) Name() string {
	return u.printerName
}

func (u *USBTransport) Type() string {
	return "usb"
}

func openPrinter(name string) (uintptr, error) {
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	var handle uintptr
	ret, _, lastErr := procOpenPrinterW.Call(
		uintptr(unsafe.Pointer(namePtr)),
		uintptr(unsafe.Pointer(&handle)),
		0,
	)
	if ret == 0 {
		return 0, lastErr
	}
	return handle, nil
}

func closePrinter(handle uintptr) {
	procClosePrinter.Call(handle)
}

// InstalledPrinter describes a Windows-installed printer discoverable by EnumPrinters.
type InstalledPrinter struct {
	Name     string `json:"name"`     // Printer name (use this for USB transport)
	Port     string `json:"port"`     // e.g. USB002, COM3, LPT1, nul:
	Driver   string `json:"driver"`   // e.g. "Generic / Text Only"
	IsRaw    bool   `json:"isRaw"`    // true if driver looks RAW/thermal-friendly
	IsLikely bool   `json:"isLikely"` // true if this looks like a thermal/receipt printer
}

// ListInstalledPrinters enumerates local Windows printers via EnumPrintersW level 2.
func ListInstalledPrinters() ([]InstalledPrinter, error) {
	// First call with nil buffer to get required size
	var needed, returned uint32
	procEnumPrintersW.Call(
		uintptr(printerEnumLocal|printerEnumConnections),
		0, // name (null = local)
		2, // level 2 = PRINTER_INFO_2
		0, // pPrinterEnum
		0, // cbBuf
		uintptr(unsafe.Pointer(&needed)),
		uintptr(unsafe.Pointer(&returned)),
	)
	if needed == 0 {
		return []InstalledPrinter{}, nil
	}

	buf := make([]byte, needed)
	ret, _, lastErr := procEnumPrintersW.Call(
		uintptr(printerEnumLocal|printerEnumConnections),
		0,
		2,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(needed),
		uintptr(unsafe.Pointer(&needed)),
		uintptr(unsafe.Pointer(&returned)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("EnumPrinters falló: %v", lastErr)
	}

	result := make([]InstalledPrinter, 0, returned)
	infoSize := unsafe.Sizeof(printerInfo2W{})
	for i := uint32(0); i < returned; i++ {
		info := (*printerInfo2W)(unsafe.Pointer(&buf[uintptr(i)*infoSize]))
		p := InstalledPrinter{
			Name:   utf16PtrToString(info.PPrinterName),
			Port:   utf16PtrToString(info.PPortName),
			Driver: utf16PtrToString(info.PDriverName),
		}
		p.IsRaw = isRawDriver(p.Driver)
		p.IsLikely = isLikelyThermalPrinter(p.Name, p.Port, p.Driver)
		result = append(result, p)
	}
	return result, nil
}

func utf16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	// Find length
	var n int
	ptr := unsafe.Pointer(p)
	for {
		c := *(*uint16)(unsafe.Pointer(uintptr(ptr) + uintptr(n)*2))
		if c == 0 {
			break
		}
		n++
	}
	slice := unsafe.Slice(p, n)
	return syscall.UTF16ToString(slice)
}

func isRawDriver(driver string) bool {
	d := strings.ToLower(driver)
	return strings.Contains(d, "generic") ||
		strings.Contains(d, "text only") ||
		strings.Contains(d, "raw") ||
		strings.Contains(d, "pos-") ||
		strings.Contains(d, "thermal") ||
		strings.Contains(d, "escpos") ||
		strings.Contains(d, "epson") ||
		strings.Contains(d, "bixolon") ||
		strings.Contains(d, "star ")
}

func isLikelyThermalPrinter(name, port, driver string) bool {
	n := strings.ToLower(name)
	p := strings.ToLower(port)
	// Port-based hints: USB/COM/LPT ports are likely physical printers (not virtual)
	physicalPort := strings.HasPrefix(p, "usb") ||
		strings.HasPrefix(p, "com") ||
		strings.HasPrefix(p, "lpt")
	// Exclude obvious non-thermal/virtual printers
	if strings.Contains(n, "pdf") ||
		strings.Contains(n, "xps") ||
		strings.Contains(n, "onenote") ||
		strings.Contains(n, "fax") ||
		strings.Contains(p, "portprompt") ||
		strings.Contains(p, "shrfax") ||
		p == "nul:" {
		return false
	}
	return physicalPort || isRawDriver(driver)
}

// ListUSBPorts kept as alias for backwards compat — now returns installed printers as ports.
func ListUSBPorts() []map[string]string {
	printers, err := ListInstalledPrinters()
	if err != nil {
		return nil
	}
	var out []map[string]string
	for _, p := range printers {
		if !p.IsLikely {
			continue
		}
		out = append(out, map[string]string{
			"name":   p.Name,
			"label":  fmt.Sprintf("%s (%s)", p.Name, p.Port),
			"type":   "usb",
			"port":   p.Port,
			"driver": p.Driver,
		})
	}
	return out
}
