# Clipboard

Un portapapeles compartido y efímero entre mis equipos: pego algo en una compu y lo copio desde otra en segundos.
Texto, links e imágenes chicas, como un chat de un solo canal. Para archivos grandes hay otras herramientas;
esto es solo lo que normalmente va por copiar y pegar.

- **`clipd`** – servidor (Go, sin dependencias). Guarda las entradas, las difunde en vivo (SSE) y las borra a las 24 horas.
- **`cb`** – cliente de terminal para Windows y Linux. Entrás como un usuario del chat: ves lo último, escribís y Enter manda,
  un clic sobre un mensaje lo copia entero.
- **Web** – una sola página, pensada para el celular, instalable como app (íconos y manifiesto incluidos).

Pensado para correr en una red privada (por ejemplo Tailscale). **No tiene cuentas ni contraseñas**: la barrera es la red.
No lo expongas a internet.

## Usar `cb`

```
cb                    # entra al chat
echo "texto" | cb     # manda texto y sale
```

| Dentro de `cb` | |
|---|---|
| texto + Enter | lo manda a todos los equipos |
| clic en un mensaje | lo copia entero (Shift + arrastrar para seleccionar a mano) |
| `c` / `c N` | copia la última entrada / la número N |
| `d N` | borra la entrada N |
| `m` | activa o desactiva el clic para copiar |
| `?` · `q` | ayuda · salir |

La primera vez pide la dirección del servidor y la guarda en `~/.config/clip/config.json`
(también se puede pasar con `--server` o `CLIP_SERVER`).

> En Windows el programa se instala como `cb.exe`, porque `clip.exe` ya existe en el sistema.

## Instalar

Binarios compilados en [Releases](../../releases): `cb-linux-amd64`, `cb-windows-amd64.exe` y `clipd-linux-amd64`
(con `SHA256SUMS`). O desde el código, con Go:

```
go test ./...
go build -o cb ./cmd/clip
go build -o clipd ./cmd/clipd
```

Para levantar el servidor y publicarlo dentro de tu red, mirá [`deploy/README.md`](deploy/README.md).

## Más

- Diseño: [`docs/superpowers/specs/2026-10-19-clipboard-design.md`](docs/superpowers/specs/2026-10-19-clipboard-design.md)
- Plan de construcción: [`docs/superpowers/plans/2026-10-19-clipboard.md`](docs/superpowers/plans/2026-10-19-clipboard.md)
- Límites: texto hasta 64 KB, imágenes hasta 5 MB (PNG, JPEG, WebP, GIF). Las imágenes se mandan desde la web; la terminal
  las recibe y las copia.
