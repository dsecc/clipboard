# Instalar Clipboard

Binarios (`go build`, ver al final, o la sección Releases del repositorio):

| Archivo | Dónde va |
|---|---|
| `clipd-linux-amd64` | el servidor (Linux x86_64) |
| `cb-windows-amd64.exe` | PC con Windows (se instala como `cb.exe`) |
| `cb-linux-amd64` | equipos con Linux |

**Nombre del comando:** `cb`. Windows ya trae un `clip.exe` propio en `System32` (copia la salida de otros comandos al
portapapeles) y siempre le gana a cualquier `clip` del PATH, así que el cliente se llama `cb` en todos los sistemas.

## 1. Servidor, sin sudo (servicio de usuario)

Sirve para cualquier Linux con systemd. Reemplazá `<usuario>` y `<servidor>`:

```
ssh <usuario>@<servidor> "mkdir -p /opt/clipboard/bin /opt/clipboard/data ~/.config/systemd/user"
scp clipd-linux-amd64 <usuario>@<servidor>:/opt/clipboard/bin/clipd
scp deploy/clipd.user.service <usuario>@<servidor>:.config/systemd/user/clipd.service
ssh <usuario>@<servidor> "chmod +x /opt/clipboard/bin/clipd && loginctl enable-linger \$USER && systemctl --user daemon-reload && systemctl --user enable --now clipd"
```

- `clipd.user.service` deja el servidor escuchando solo en `127.0.0.1:8787`, con los datos en `/opt/clipboard/data`.
- `enable-linger` hace que arranque solo al iniciar el equipo, sin sudo. Estado: `systemctl --user status clipd`.
- Actualizar: copiar el binario nuevo a `/opt/clipboard/bin/clipd.new`, `mv` sobre `clipd` y `systemctl --user restart clipd`.

## 2. Publicarlo dentro de tu red (Tailscale)

```
tailscale serve --bg --https=443 8787
```

Queda en `https://<servidor>.<tu-tailnet>.ts.net`, solo para tus equipos. Si pide activar HTTPS, hay que habilitarlo una
vez en la consola de Tailscale (DNS → HTTPS Certificates). HTTPS es necesario para que el iPhone deje copiar al
portapapeles desde la web.

### Alternativa con sudo: servicio del sistema

`deploy/install.sh` y `deploy/clipd.service` instalan `clipd` como servicio del sistema:

```
scp clipd-linux-amd64 deploy/clipd.service deploy/install.sh <usuario>@<servidor>:/tmp/clip/
ssh -t <usuario>@<servidor> "cd /tmp/clip && sudo sh install.sh"
```

## 3. Celular

Con la app de Tailscale conectada, abrí la dirección HTTPS en Safari. La primera vez te pide un nombre para el equipo.
Para tenerla como app: Compartir → Agregar a pantalla de inicio.

## 4. PCs

Copiá el ejecutable a una carpeta del PATH con el nombre `cb`:

- Windows: `cb-windows-amd64.exe` → `cb.exe` (por ejemplo en `%USERPROFILE%\.local\bin`).
- Linux: `sudo install -m 0755 cb-linux-amd64 /usr/local/bin/cb` (o `~/.local/bin/cb` sin sudo). Para copiar al
  portapapeles conviene tener `xclip` (`sudo apt install xclip`) o `wl-clipboard` en Wayland; sin ellos el texto se copia
  por la terminal (OSC 52).

La primera vez que lo ejecutás pide la dirección del servidor y la guarda en `~/.config/clip/config.json`
(en Windows, `C:\Users\<vos>\.config\clip\config.json`). No usa `%APPDATA%` a propósito: las apps instaladas como
paquete ven una copia privada de esa carpeta y la configuración no llegaría a tu terminal. También se puede pasar con
`--server` o la variable `CLIP_SERVER`. Desde un script: `echo "texto" | cb`.

## Compilar

```
go test ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/clipd-linux-amd64 ./cmd/clipd
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/cb-linux-amd64 ./cmd/clip
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/cb-windows-amd64.exe ./cmd/clip
```

## Notas

- Las entradas se borran solas a las 24 h. Para probar el vencimiento: `clipd --ttl 2m` (solo pruebas).
- Seguridad: no hay autenticación propia. El servidor solo debe ser accesible desde tu red privada.
