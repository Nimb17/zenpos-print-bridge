# Mivy Print Bridge

Agente local en Go que conecta cualquier SaaS web (zenPOS, MesaDigital, futuros productos) con impresoras térmicas USB, de red o serial en Windows.

## Cómo funciona

```
SaaS web (HTTPS) ──fetch──► http://localhost:7777 ──USB/TCP/COM──► Impresora
```

El bridge expone un HTTP REST server en `localhost:7777`. Chrome y Firefox permiten llamadas `http://localhost` desde páginas HTTPS (localhost está exento del bloqueo de mixed content por spec W3C).

Un **único** binario sirve para todas las apps del ecosistema Mivy. El cliente lo instala una vez y cualquier app web del ecosistema puede imprimir.

## Endpoints

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET  | `/status`     | Estado del bridge y la impresora |
| GET  | `/printers`   | Impresoras instaladas + puertos COM disponibles |
| POST | `/configure`  | Configura impresora (USB, serial o red) |
| POST | `/print`      | Imprime bytes ESC/POS (base64) o receipt JSON |
| GET  | `/test`       | Imprime página de prueba |
| POST | `/disconnect` | Desconecta la impresora activa |

## Ejemplos rápidos

### Configurar impresora USB instalada (recomendado en Windows)

```bash
curl -X POST http://localhost:7777/configure \
  -H "Content-Type: application/json" \
  -d '{"type":"usb","usbPort":"POS-58-Raw","paperWidthMM":58}'
```

### Configurar impresora serial

```bash
curl -X POST http://localhost:7777/configure \
  -H "Content-Type: application/json" \
  -d '{"type":"serial","port":"COM3","baud":9600,"paperWidthMM":80}'
```

### Configurar impresora de red (WiFi/Ethernet)

```bash
curl -X POST http://localhost:7777/configure \
  -H "Content-Type: application/json" \
  -d '{"type":"network","ip":"192.168.1.50","networkPort":9100,"paperWidthMM":80}'
```

### Imprimir receipt estructurado

```bash
curl -X POST http://localhost:7777/print \
  -H "Content-Type: application/json" \
  -d '{
    "receipt": {
      "businessName": "Mi Tienda",
      "date": "18/04/2026 15:30",
      "txId": "12345",
      "items": [{"name":"Producto A","quantity":2,"price":1500}],
      "subtotal": 3000,
      "total": 3000,
      "paymentMethod": "efectivo",
      "amountTendered": 5000,
      "change": 2000,
      "paperWidth": 80
    }
  }'
```

### Imprimir bytes ESC/POS raw (base64)

```bash
curl -X POST http://localhost:7777/print \
  -H "Content-Type: application/json" \
  -d '{"bytes":"G0A..."}'
```

## Desarrollo local

```bash
go mod tidy
go run .
```

## Build para Windows

```bash
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o mivy-bridge.exe .
```

## Instalar como servicio Windows

```bash
mivy-bridge.exe -action=install
mivy-bridge.exe -action=uninstall
```

## Instalador oficial

El flujo normal para clientes es descargar el instalador `.exe` firmado desde GitHub Releases — se encarga de todo automáticamente (service, firewall, autoarranque). Ver `installer/README.md`.

## Config persistida

`%PROGRAMDATA%\Mivy\PrintBridge\config.json` (compartido entre todos los usuarios del PC)

Fallback: `%APPDATA%\Mivy\PrintBridge\config.json`

Migración automática desde la marca previa (`zenPOS`) — el bridge detecta config viejo en `%PROGRAMDATA%\zenPOS\PrintBridge\` o `%APPDATA%\zenPOS\PrintBridge\` y lo adopta al primer arranque.

## Log

`%PROGRAMDATA%\Mivy\PrintBridge\bridge.log`

## Documentación extendida

- `installer/README.md` — cómo compilar el instalador
- `RELEASING.md` — cómo publicar nuevas versiones
- `PLAYBOOK.md` — cómo replicar este patrón para otro tipo de hardware
