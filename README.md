# Go API + Playwright Automation

Minimal project that exposes a Go HTTP API to list and run Playwright tests. Tests use pure Playwright (no @playwright/test) and are simple Node scripts. Tests are organized by folders, and routes are derived from those folders.

## Structure

- `automation/`: Playwright project
  - `core/`: Reusable flows (e.g., `login.js`).
  - `runtimes/`: Page-specific helpers (e.g., `bateo_ventas.js`).
  - `tests/<group>/<name>.js`: Add new tests here; each `<group>` automatically becomes an API route segment.
  - `run.js`: Lightweight runner that executes Node test files sequentially.
- `cmd/server/main.go`: Go HTTP server with routes.
- `internal/runner/`: Helpers to list and run tests.

## Requisitos

- Go 1.21+
- Node.js 18+ con `npm`/`npx`

## Instalar Go

Elige el método según tu sistema operativo:

- macOS (Homebrew):
  - `brew install go`
- Ubuntu/Debian (APT):
  - `sudo apt-get update && sudo apt-get install -y golang-go`
- Windows:
  - Descarga el instalador desde la web oficial de Go y sigue el asistente.

Verifica la instalación:

```
go version
```

## Instalar Node.js y npm

Recomendado: NVM para gestionar versiones de Node.

1) Instala NVM (según documentación oficial) y luego:

```
nvm install 18
nvm use 18
node -v
npm -v
```

Alternativas:
- macOS: `brew install node`
- Ubuntu/Debian: `sudo apt-get install -y nodejs npm`
- Windows: instalador de Node.js LTS desde nodejs.org

## Instalar Playwright y dependencias

Desde la carpeta `automation/`:

```
cd automation
npm install
npx playwright install --with-deps
```

Esto instala la librería `playwright`, los navegadores y dependencias del sistema (especialmente en Linux).

## Ejecutar el servidor Go

Desde la raíz del repo:

```
go run ./cmd/server
```

El servidor queda en `http://localhost:8080`.

## Rutas de la API

- `GET /health`: Basic health check
- `GET /tests`: List groups and tests discovered under `automation/tests`
- `POST /run/all`: Run all tests
- `POST /run/{group}`: Run all tests in a group folder
- `POST /run/{group}/{test}`: Run a specific test file (with or without extension). Defaults to `.js` if no extension.
- `POST /bateo/ventas/fecha-rango`: Login + fija fechas (desde el primer día del mes actual hasta mañana), pulsa Exportar, INGESTA el Excel en SQLite y devuelve el archivo como descarga. Acepta body JSON opcional `{ "baseUrl", "user", "pass" }` o usa `ERP_*`.
- `GET /bateo/ventas/export?[date=YYYY-MM-DD][&baseUrl=...&user=...&pass=...]`: Ejecuta login + bateo de ventas. Si no pasas `date`, usa la fecha de hoy. Fija rango desde el primer día del mes hasta el día siguiente a la fecha efectiva, exporta y transmite el Excel. También ingesta el archivo en SQLite local antes de enviarlo.

Ejemplos:

```
curl -X POST http://localhost:8080/run/all
curl -X POST http://localhost:8080/run/smoke
curl -X POST http://localhost:8080/run/smoke/example.js
curl -X POST http://localhost:8080/run/bateo/fecha_rango.js
curl -X POST -o export-hoy.xlsx http://localhost:8080/bateo/ventas/fecha-rango
curl -L -o export.xlsx "http://localhost:8080/bateo/ventas/export"
curl -L -o export-2025-01-15.xlsx "http://localhost:8080/bateo/ventas/export?date=2025-01-15"
curl -X POST -o export-hoy.xlsx http://localhost:8080/bateo/ventas/fecha-rango \
  -H 'Content-Type: application/json' \
  -d '{"baseUrl":"http://erpvm.kurigage.com","user":"ricardo.valencia@farmaciasbustillos.com","pass":"P4u1A280325*"}'
```

Las respuestas incluyen comando ejecutado, código de salida, duración y logs.

## Modo Headless

- By default, browsers launch with `headless: false` so you can see the UI.
- Override with env var: `HEADLESS=true` to run headless when needed.

## Agregar más pruebas (Folder Method)

- Create a new group folder under `automation/tests/`, e.g. `automation/tests/checkout/`.
- Add Node files in that folder with the naming `*.js` (or `*.mjs`). Each file should run Playwright directly and exit with code 0 on success.
- The API will automatically expose:
  - `POST /run/checkout` to run the group
  - `POST /run/checkout/<file>.js` to run a specific test

## Credenciales del ERP

Configura variables de entorno para no hardcodear secretos:

```
export ERP_BASE_URL="http://erpvm.kurigage.com"
export ERP_USER="ricardo.valencia@farmaciasbustillos.com"
export ERP_PASS="P4u1A280325*"
```

Si no las defines, se usan estos valores por defecto.

## Base de datos (SQLite)

- Archivo: `automation/data/erp.sqlite` (se crea automáticamente).
- Ingesta: al llamar `GET /bateo/ventas/export?date=YYYY-MM-DD`, el servidor parsea el Excel exportado y lo guarda en la base.
- Tablas principales:
  - `ingest_batches(id, range_start, range_end, filename, created_at)`
  - `bateo_ventas_rows(id, batch_id, row_index, data_json)`
- `range_start` es el primer día del mes de la fecha consultada y `range_end` es el día siguiente a la fecha consultada. Esto actúa como la referencia primaria lógica para el lote.

### Dependencias Go para la ingesta

Para compilar/ejecutar con la ingesta activa, asegura red para resolver módulos y luego:

```
go get modernc.org/sqlite@v1.30.1 github.com/xuri/excelize/v2@v2.8.1
go run ./cmd/server
```

Consulta rápida en SQLite (opcional):

```
sqlite3 automation/data/erp.sqlite \
  'SELECT id, range_start, range_end, filename, created_at FROM ingest_batches ORDER BY id DESC LIMIT 5;'
```

## Notes

- This setup avoids third-party Go routers to keep things dependency-free; it uses the standard library and dynamic path parsing.
- Tests are pure Playwright scripts; no @playwright/test runner is used.
