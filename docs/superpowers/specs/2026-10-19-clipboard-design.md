# Clipboard — diseño

## Intención
Un "portapapeles compartido" personal entre las computadoras y el iPhone de Santi. Hoy pasa texto, links o alguna imagen chica de un equipo a otro con copiar y pegar; no existe una forma rápida de hacerlo entre máquinas. Se resuelve con un chat efímero de un solo canal, alojado en el servidor propio de Santi y accesible solo por Tailscale. Los archivos NO van acá: para eso ya usa Wormhole.

## Qué se construye
Tres piezas que comparten un servidor:
1. **Servidor** en el servidor de Santi (Tailscale). Guarda las entradas, las difunde en tiempo real y las borra a las 24 h.
2. **Cliente de terminal `clip`**: un ejecutable para Windows y otro para Linux, en el PATH. Es el uso principal en las computadoras.
3. **Web**: una sola página servida por el mismo servidor. Uso principal en el iPhone; debe estar muy pulida en móvil.

## Metas
- Pegar algo en un equipo y tenerlo copiable en otro en segundos.
- Texto, links e imágenes chicas.
- Tiempo real: lo que llega aparece solo en todos los equipos abiertos.
- Instalación mínima: un ejecutable por sistema, sin dependencias.
- Web cómoda y estética en iPhone.

## No metas
- Archivos (Wormhole).
- "Inteligencia" (clasificar, resumir, detectar contenido). Pedido explícito: no.
- Fijar entradas, carpetas, canales múltiples, búsqueda, historial largo.
- Cuentas, usuarios, permisos. Es de un solo dueño.
- Cifrado propio extremo a extremo.

## Anti-metas (sería un fracaso aunque funcione)
- Que se vuelva un historial eterno o un bloc de notas.
- Que quede expuesto a internet.
- Que copiar requiera más de un toque/clic o comando corto.
- Que en iPhone sea incómodo (zoom raro, teclado que tapa, botones diminutos).

## Decidido por Santi
- **Duración:** las entradas se borran solas a las 24 horas. No hay fijar.
- **Borrar:** cada entrada se puede borrar a mano.
- **Forma:** chat de un solo canal con globos; la más nueva abajo.
- **Copiar:** un clic/toque sobre el globo copia su contenido y muestra "✓ Copiado".
- **Enviar:** sin botón Enviar. Enter envía.
- **Estética:** oscura, sobria, tipo Claude. Referencia vista en maqueta: globos con borde fino, acento terracota, tipografía de sistema, monoespaciada para código.
- **Terminal:** al ejecutar `clip` te conectás al servidor y entrás como "un usuario del chat", estilo cliente de MySQL: no es pantalla completa. Al entrar imprime las entradas de las últimas 24 h (de vieja a nueva) y deja el prompt abajo; el scroll y la selección con mouse son los nativos de la terminal. Lo que llega después aparece solo arriba del prompt.
- **Web para móvil:** se usará en iPhone junto con las computadoras.

## Decisiones mías (Claude) — Santi las puede cambiar
- Shift+Enter hace salto de línea en la web de escritorio; Ctrl+V con una imagen la envía.
- En el iPhone, la tecla de retorno del teclado envía (`enterkeyhint="send"`); como en móvil no hay Shift+Enter, los mensajes de varias líneas se arman desde la web de escritorio o la terminal.
- En `clip`: escribir texto + Enter envía; `c` copia la última entrada al portapapeles local, `c N` la N-ésima, `d N` la borra. Las entradas se numeran en el listado. Las imágenes se muestran como `[imagen 120 KB]` y `c N` las copia al portapapeles.
- Cada globo muestra el nombre del equipo de origen (el hostname) y cuánto hace.
- El cliente habla con el servidor directo por la red de Tailscale (HTTP/WebSocket), no usa el comando `ssh`.
- Sin autenticación propia: la barrera es Tailscale. El servidor escucha solo en la interfaz de Tailscale, nunca en una interfaz pública.
- Para que copiar funcione bien en iOS Safari hace falta contexto seguro (HTTPS); se prevé servir con `tailscale serve`. Esto hay que verificarlo en el momento.
- Idioma de la interfaz: español.

## Decisiones mías, tras la revisión del constructor
- **HTTPS primero:** lo primero que se construye es una prueba mínima (spike) de la web en el iPhone sobre `tailscale serve` (HTTPS en `*.ts.net`) para confirmar que copiar funciona. Si Santi no tiene HTTPS/MagicDNS activado en su tailnet, hay que activarlo. Si aun así copiar falla en iOS, plan B: texto seleccionado automáticamente más `execCommand('copy')`. El servidor escucha solo en loopback o en la IP de Tailscale; `clip` usa la URL `https://…ts.net` si existe.
- **Dirección del servidor en `clip`:** la primera vez que se ejecuta pide la dirección y la guarda en un archivo de configuración; también se puede pasar con `--server` o la variable `CLIP_SERVER`.
- **Volumen:** uso personal, pocas decenas de entradas al día. Texto hasta 64 KB, imagen hasta 5 MB. Las imágenes se guardan como archivos y se piden aparte; el historial inicial trae solo metadatos y texto. API: REST para listar/borrar/enviar y Server-Sent Events (SSE) para eventos en vivo (nueva entrada, borrada); SSE se reconecta solo y no necesita librerías.
- **Terminal:** `clip` controla la línea de entrada para poder redibujar el prompt cuando llega algo mientras escribís. Las librerías se compilan dentro del ejecutable ("sin dependencias" se refiere a los equipos de Santi). Un pegado de varias líneas en la terminal (bracketed paste) se envía como una sola entrada.
- **Nombre del equipo:** en la web, la primera vez se pide un nombre ("iPhone", "PC web") y se guarda en el navegador. Borrar una entrada se propaga en vivo a todos los clientes abiertos.
- **Imágenes en el iPhone:** botón de adjuntar con selector de fotos, además del pegado.

## Imágenes y equipos (respondido por Santi)
- **Servidor:** Debian 12 (se arranca como servicio systemd). **PC de escritorio:** Windows 11. **Netbook:** Kali Linux (Debian). **Teléfono:** iPhone.
- **Imágenes:** se envían solo desde la web (pegar o adjuntar foto). La terminal las recibe y muestra `[imagen 120 KB]` y `c N` las copia al portapapeles; no se envían desde la terminal en esta versión.
- **Idea abierta de Santi (no incluida en v1):** poder mandar la imagen del portapapeles desde la terminal con un comando (`clip` avisaría "imagen enviada") y abrirla desde la web en el otro equipo. Queda anotada como mejora futura; requiere leer el portapapeles de imágenes en Windows y en Kali.

## Restricciones
- Todo dentro de la red Tailscale de Santi.
- Windows 11 y Linux (Kali, probablemente X11) para el cliente de terminal; iPhone para la web (Safari). Servidor Debian 12.
- Sin dependencias que instalar en los equipos clientes.

## Se deja al constructor
- Lenguaje y librerías (inclinación: Go para servidor y cliente, un solo binario cada uno, con la página web incrustada; almacenamiento simple en archivo o SQLite).
- Límite de tamaño de texto e imagen, y formato de guardado de imágenes.
- Reconexión automática si se corta la red o el servidor reinicia.
- Cómo se instala y se arranca el servidor (servicio, arranque automático).
- Detalles visuales de la web móvil, respetando la estética y las decisiones de arriba.
- Cómo copiar al portapapeles en cada sistema (Windows, Linux X11/Wayland).
