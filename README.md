# zenPOS Print Bridge

Agente local en Go que permite imprimir en impresoras térmicas USB o de red desde zenPOS (web app HTTPS).

## Cómo funciona

```
zenPOS (HTTPS) ──fetch──► http://localhost:7777 ──COM/TCP──► Impresora
```

El bridge expone un HTTP REST server en `localhost:7777`. Chrome y Firefox permiten llamadas `http://localhost` desde páginas HTTPS (localhost está exento del bloqueo de mixed content por spec W3C).

## Endpoints

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/status` | Estado del bridge y la impresora |
| GET | `/printers` | Lista puertos COM disponibles |
| POST | `/configure` | Configura impresora (serial o red) |
| POST | `/print` | Imprime bytes ESC/POS (base64) o receipt JSON |
| GET | `/test` | Imprime página de prueba |
| POST | `/disconnect` | Desconecta la impresora activa |

## Configurar impresora serial (USB)

```bash
curl -X POST http://localhost:7777/configure \
  -H "Content-Type: application/json" \
  -d '{"type":"serial","port":"COM3","baud":9600,"paperWidthMM":80}'
```

## Configurar impresora de red (WiFi/Ethernet)

```bash
curl -X POST http://localhost:7777/configure \
  -H "Content-Type: application/json" \
  -d '{"type":"network","ip":"192.168.1.50","networkPort":9100,"paperWidthMM":80}'
```

## Imprimir receipt estructurado

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

## Imprimir bytes ESC/POS raw (base64)

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
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o zenpos-bridge.exe .
```

## Instalar como servicio Windows

```bash
zenpos-bridge.exe -action install
zenpos-bridge.exe -action uninstall
```

## Config persistida

`%APPDATA%\zenPOS\PrintBridge\config.json`

## Log

`%APPDATA%\zenPOS\PrintBridge\bridge.log`
