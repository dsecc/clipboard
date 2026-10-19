#!/bin/sh
# Instala clipd como servicio y lo expone por HTTPS dentro de tu tailnet.
# Uso (en el servidor, con sudo): sudo sh install.sh [ruta-al-binario]
set -e
BIN="${1:-./clipd-linux-amd64}"
HERE="$(cd "$(dirname "$0")" && pwd)"

install -m 0755 "$BIN" /usr/local/bin/clipd
install -m 0644 "$HERE/clipd.service" /etc/systemd/system/clipd.service
systemctl daemon-reload
systemctl enable --now clipd
sleep 1
systemctl --no-pager --lines=5 status clipd || true

# HTTPS con certificado de Tailscale; el servicio escucha solo en 127.0.0.1
tailscale serve --bg 8787
tailscale serve status
echo
echo "Listo. Probá: curl -s http://127.0.0.1:8787/api/entries"
