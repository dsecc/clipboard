# Clipboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Un chat efímero de un solo canal (texto, links, imágenes chicas) entre las compus y el iPhone de Santi, con servidor propio por Tailscale, cliente de terminal `clip` y web móvil.

**Architecture:** Un servidor Go (`clipd`) guarda entradas en disco, las expone por REST y difunde eventos por SSE; sirve la web incrustada. El cliente `clip` (Go, un binario por SO) usa la misma API. Todo vive en el mismo módulo para compartir los tipos.

**Tech Stack:** Go (biblioteca estándar + `golang.org/x/term`), HTML/CSS/JS sin framework ni build, `tailscale serve` para HTTPS, systemd en Debian 12.

**Spec:** `docs/superpowers/specs/2026-10-19-clipboard-design.md`

## Global Constraints

- Entradas se borran solas a las **24 horas**; no hay fijar.
- Texto máximo **64 KB**; imagen máxima **5 MB**; solo PNG, JPEG, WebP, GIF (no SVG).
- Sin cuentas ni autenticación propia; el servidor escucha solo en loopback o en la IP de Tailscale, nunca en una interfaz pública.
- Un clic/toque en el globo copia y muestra "✓ Copiado"; Enter envía, Shift+Enter salto de línea (web de escritorio); sin botón Enviar; en iPhone `enterkeyhint="send"`.
- Estética oscura, sobria, tipo Claude (globos de borde fino, acento terracota `#d97757`, tipografía de sistema, monoespaciada para código). Interfaz en español.
- `clip`: al entrar imprime las últimas 24 h (de vieja a nueva) con prompt abajo; comandos `c`, `c N`, `d N`; imágenes como `[imagen 120 KB]`; no envía imágenes.
- Cada globo muestra el nombre del equipo de origen y cuánto hace.
- Clientes: Windows 11 y Kali Linux; servidor Debian 12; web en Safari de iPhone. Un ejecutable por SO, sin instalar nada en los clientes (si falta una herramienta de portapapeles, usar OSC 52 como alternativa).

## Review Focus

- Mensaje vacío o solo espacios: no se envía ni crea entrada.
- Texto mayor a 64 KB o imagen mayor a 5 MB: rechazo con error visible, nunca truncado en silencio.
- Corte de red o reinicio del servidor: web y `clip` se reconectan solos y no duplican entradas ya mostradas (se deduplica por ID).
- Una entrada vence o se borra mientras hay clientes abiertos: desaparece en vivo; `c N` sobre una entrada borrada avisa en vez de copiar vacío.
- Texto con HTML, emoji, saltos de línea o muchas líneas: se muestra literal (nunca interpretado como HTML) y se copia idéntico.
- Archivo que no es imagen permitida (por ejemplo SVG o HTML): rechazado.

---

## File Structure

```
go.mod
cmd/clipd/main.go            # arranque del servidor: flags, store, servidor HTTP, barrido
cmd/clip/main.go             # arranque del cliente de terminal
internal/model/model.go      # tipos compartidos y límites
internal/store/store.go      # persistencia, vencimiento, imágenes en disco
internal/server/server.go    # REST + SSE + sirve la web
internal/client/api.go       # cliente HTTP/SSE contra el servidor
internal/client/config.go    # dirección del servidor, nombre del equipo
internal/client/repl.go      # parseo de comandos, numeración, formato de líneas
internal/client/term.go      # línea de entrada con redibujo de prompt
internal/client/clipboard.go # copiar al portapapeles por SO (+ OSC 52)
web/index.html, web/app.css, web/app.js, web/embed.go
deploy/clipd.service, deploy/README.md
```

### Task 0: Entorno y prueba de HTTPS en iPhone

**Files:**
- Create: `go.mod`, `.gitignore` (con `.superpowers/` y `dist/`), `spike/index.html` (descartable)

**Interfaces:**
- Produces: repo git inicializado, módulo `clipboard`, Go instalado; respuesta confirmada a "¿copiar funciona en iOS Safari sobre `tailscale serve`?".

- [ ] **Step 1:** Instalar Go (última estable) en la PC con Windows y verificar `go version`. Instalar Go es una dependencia nueva: avisar a Santi antes.
- [ ] **Step 2:** `git init` en `Clipboard/`, `go mod init clipboard`, commit inicial con la documentación existente.
- [ ] **Step 3:** Con Santi, en el servidor Debian: servir `spike/index.html` (un botón que ejecuta `navigator.clipboard.writeText("prueba")`) y exponerlo con `tailscale serve`. Santi lo abre en el iPhone.
  Resultado esperado: tocar el botón y pegar en Notas muestra "prueba". Si no funciona, probar el plan B (texto seleccionado más `document.execCommand('copy')`) y anotar cuál sirve en `deploy/README.md`.
- [ ] **Step 4:** Confirmar si MagicDNS y HTTPS están activados en la tailnet; si no, indicar a Santi cómo activarlos. Commit.

### Task 1: Modelo y almacenamiento

**Files:**
- Create: `internal/model/model.go`, `internal/store/store.go`
- Test: `internal/store/store_test.go`

**Interfaces:**
- Produces (`model`): `type Entry struct { ID string; Kind string /* "text"|"image" */; Text string; Mime string; Size int; Device string; CreatedAt time.Time }`; constantes `MaxText = 64<<10`, `MaxImage = 5<<20`, `TTL = 24*time.Hour`; `var ErrTooLarge, ErrEmpty, ErrBadMime, ErrNotFound error`.
- Produces (`store`): `func Open(dir string, clock func() time.Time) (*Store, error)`; `(*Store).AddText(device, text string) (model.Entry, error)`; `(*Store).AddImage(device, mime string, r io.Reader) (model.Entry, error)`; `(*Store).List() []model.Entry` (no vencidas, de vieja a nueva); `(*Store).Delete(id string) error`; `(*Store).Image(id string) (io.ReadCloser, string, error)`; `(*Store).Sweep() []string` (IDs borrados por vencimiento).

- [ ] **Step 1:** Tests que fallen: `TestAddTextRejectsEmptyAndWhitespace` (`ErrEmpty`), `TestAddTextRejectsOver64KB` (`ErrTooLarge`; 64 KB exactos sí pasa), `TestAddImageRejectsOver5MBAndBadMime` (`image/svg+xml` y `text/html` dan `ErrBadMime`), `TestListOrderOldestFirst`, `TestSweepRemovesAfter24h` (reloj inyectado: a 23 h 59 min sigue, a 24 h 00 min 01 s no, y el archivo de imagen se borra), `TestPersistsAcrossReopen`, `TestDeleteUnknownIsErrNotFound`.
- [ ] **Step 2:** Correr `go test ./internal/store/...`; esperado: FAIL (compilación).
- [ ] **Step 3:** Implementar los tipos y el store; persistir metadatos en `entries.json` (escritura atómica con archivo temporal y rename) e imágenes en `images/<id>`. IDs aleatorios de 12+ caracteres. Seguro ante accesos concurrentes (mutex).
- [ ] **Step 4:** `go test ./...` en verde. Commit.

### Task 2: Servidor HTTP (REST + SSE)

**Files:**
- Create: `internal/server/server.go`, `cmd/clipd/main.go`, `web/embed.go` (con `//go:embed` y un `index.html` mínimo provisorio)
- Test: `internal/server/server_test.go` (con `httptest`)

**Interfaces:**
- Consumes: `store.Store` de la Tarea 1.
- Produces: `func New(s *store.Store, web fs.FS) http.Handler`. Rutas:
  `GET /api/entries` → `[]model.Entry`;
  `POST /api/entries` cuerpo JSON `{"text","device"}` → `201` + `model.Entry`;
  `POST /api/images` cuerpo = bytes de la imagen, cabeceras `Content-Type` y `X-Device` → `201` + `model.Entry`;
  `DELETE /api/entries/{id}` → `204` (`404` si no existe);
  `GET /api/images/{id}` → bytes con su `Content-Type`, `X-Content-Type-Options: nosniff`;
  `GET /api/events` (SSE) → eventos `event: added` con la `Entry` en JSON y `event: deleted` con `{"id":"…"}`, más comentario de latido cada 20 s.
  Errores: `400` vacío, `413` demasiado grande, `415` tipo no permitido; cuerpo `{"error":"mensaje en español"}`.
  `cmd/clipd` flags: `--listen` (default `127.0.0.1:8787`), `--data` (default `./data`), `--ttl` (default 24h; solo para pruebas); barrido de vencidos cada minuto que emite `deleted` por SSE.

- [ ] **Step 1:** Tests que fallen: `TestPostTextThenListAndSSEAdded`, `TestSSEDeliversDeletedOnDelete`, `TestStatusCodes400413415`, `TestImageServedWithNosniff`, `TestSweepEmitsDeletedEvent`, `TestMultipleSSEClientsAllReceive`.
- [ ] **Step 2:** Correr; esperado FAIL.
- [ ] **Step 3:** Implementar el servidor con un hub de suscriptores; limitar el cuerpo con `http.MaxBytesReader` (64 KB + margen en texto, 5 MB en imágenes).
- [ ] **Step 4:** Tests en verde; probar a mano con `curl` contra `clipd` local (enviar, listar, borrar). Commit.

### Task 3: Web (escritorio y móvil)

**Files:**
- Create/Modify: `web/index.html`, `web/app.css`, `web/app.js`

**Interfaces:**
- Consumes: la API de la Tarea 2.

- [ ] **Step 1:** Escribir la página según la maqueta aprobada (opción B): lista de globos con la más nueva abajo; nombre del equipo y "hace X"; clic/toque en el globo copia y muestra "✓ Copiado"; botón borrar por globo; caja de entrada abajo; imágenes como miniatura con clic para copiar; tema oscuro con los colores de Global Constraints.
- [ ] **Step 2:** Comportamiento: Enter envía, Shift+Enter salto de línea; Ctrl+V con imagen la envía; botón adjuntar con selector de fotos; primera vez pide el nombre del equipo y lo guarda en el navegador; contenido insertado siempre con `textContent` (nunca `innerHTML`); `EventSource` se reconecta y al reconectar recarga `GET /api/entries` y deduplica por ID; muestra errores del servidor (413 etc.) en una línea visible.
- [ ] **Step 3:** Móvil: `meta viewport` con `viewport-fit=cover`, entrada con `font-size` ≥16 px (evita el zoom de iOS), respeta `env(safe-area-inset-bottom)`, el teclado no tapa la caja (`visualViewport`), objetivos táctiles ≥44 px, `enterkeyhint="send"`, `manifest` mínimo para agregar a la pantalla de inicio.
- [ ] **Step 4:** Verificar con `clipd` local en el navegador integrado: emulación móvil (375×812) y escritorio; probar enviar, copiar, borrar, mensaje con `<b>hola</b>` y emoji, pegar imagen, desconectar/reconectar el servidor. Commit.

### Task 4: Cliente de terminal `clip`

**Files:**
- Create: `cmd/clip/main.go`, `internal/client/api.go`, `config.go`, `repl.go`, `term.go`, `clipboard.go`
- Test: `internal/client/repl_test.go`, `internal/client/api_test.go`, `internal/client/config_test.go`

**Interfaces:**
- Consumes: la API de la Tarea 2.
- Produces: `func ParseCommand(line string) Command` (`Command{Kind: Send|Copy|Delete|Quit|Help; N int; Text string}`; `c` sin número significa la última; `d N` borra; cualquier otra cosa es texto a enviar; `\c` envía literal "c"); `type Numbering` con `Assign(id string) int` y `ID(n int) (string, bool)` (número estable durante la sesión, no se reutiliza si algo se borra); `func FormatEntry(n int, e model.Entry, now time.Time) string` (por ejemplo `[3] netbook · hace 2 min  texto…` o `[4] … [imagen 120 KB]`, multilínea con sangría); `func LoadConfig() (Config, error)` / `Config{Server, Device string}` (archivo en el directorio de configuración del usuario; prioridad: flag `--server` > `CLIP_SERVER` > archivo > preguntar la primera vez y guardar); `func Copy(text string) error`, `func CopyImage(mime string, data []byte) error` (Windows: PowerShell; Linux: `wl-copy`/`xclip`/`xsel` si existen, si no OSC 52 para texto).

- [ ] **Step 1:** Tests que fallen: `TestParseCommand` (tabla: `c`, `c 3`, `d 3`, `c x`, texto normal, `\c`, vacío), `TestNumberingStableAfterDelete`, `TestFormatEntryText/Image/Multiline`, `TestConfigPrecedence`, `TestEventStreamReconnectDedupes` (servidor `httptest` que corta la conexión; el cliente no repite IDs ya vistos), `TestClientReportsServerErrors` (413 devuelve el mensaje en español).
- [ ] **Step 2:** Correr; esperado FAIL.
- [ ] **Step 3:** Implementar `api.go` (SSE con `net/http` y reconexión con espera creciente), `config.go`, `repl.go`.
- [ ] **Step 4:** Implementar `term.go`: modo crudo con `golang.org/x/term`, edición de línea (flechas, borrar, historial de líneas enviadas), bracketed paste (un pegado multilínea = una sola entrada), y `Print(line)` que borra el prompt, imprime y lo redibuja sin perder lo que se estaba escribiendo; Ctrl+C/Ctrl+D salen y restauran la terminal. En Windows habilitar secuencias virtuales de la consola.
- [ ] **Step 5:** `main.go`: conecta, imprime las últimas 24 h, escucha eventos, procesa comandos. `c N` sobre una entrada inexistente o ya borrada avisa en vez de copiar. Tests en verde.
- [ ] **Step 6:** Verificación manual contra `clipd` local: dos instancias de `clip` más la web abiertas; enviar desde cada una, copiar con `c`, borrar con `d`, escribir a mitad de una línea mientras llega otra entrada. Commit.

### Task 5: Despliegue

**Files:**
- Create: `deploy/clipd.service`, `deploy/README.md`, `Makefile` o `build.ps1` con los destinos de compilación

**Interfaces:**
- Produces: `dist/clipd-linux-amd64`, `dist/clip-windows-amd64.exe`, `dist/clip-linux-amd64`.

- [ ] **Step 1:** Compilar los tres binarios con `GOOS`/`GOARCH` y `CGO_ENABLED=0`. Confirmar arquitectura del servidor y de la netbook con Santi (`uname -m`) antes de compilar.
- [ ] **Step 2:** `clipd.service` (systemd): usuario sin privilegios, `ExecStart=/usr/local/bin/clipd --listen 127.0.0.1:8787 --data /var/lib/clipd`, `Restart=always`, `StateDirectory=clipd`. README: cómo copiar el binario, `systemctl enable --now clipd`, `tailscale serve --bg 8787`, y la dirección resultante.
- [ ] **Step 3:** Con acceso de Santi al servidor, instalar y arrancar. Verificar `systemctl status clipd` activo y que reinicia solo tras `systemctl restart`.
- [ ] **Step 4:** Instalar `clip` en el PATH de Windows y de Kali (`/usr/local/bin`). Documentar. Commit.

### Task 6: Verificación de punta a punta

- [ ] **Step 1:** Escenario real: pegar un texto en la PC con Windows (`clip`), verlo en la netbook (`clip`) y en el iPhone; copiar en cada uno y pegar en otra app.
- [ ] **Step 2:** Mandar una imagen desde el iPhone (foto) y desde la web de escritorio (Ctrl+V); verla como `[imagen …]` en `clip` y copiarla con `c N`.
- [ ] **Step 3:** Cada punto de Review Focus, a mano: vacío, 64 KB + 1, SVG, reinicio del servidor con clientes abiertos, borrar mientras otro mira, HTML y emoji. Anotar resultado de cada uno.
- [ ] **Step 4:** Comprobar que el vencimiento funciona: arrancar `clipd` con TTL corto (flag de prueba `--ttl`, solo para verificación) y observar el borrado en vivo. Commit final y actualizar `deploy/README.md`.
