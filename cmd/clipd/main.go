// Command clipd is the Clipboard server.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"time"

	"clipboard/internal/server"
	"clipboard/internal/store"
	"clipboard/web"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8787", "dirección donde escucha (loopback o IP de Tailscale)")
	data := flag.String("data", "./data", "carpeta donde se guardan las entradas")
	ttl := flag.Duration("ttl", 24*time.Hour, "duración de las entradas (solo para pruebas)")
	flag.Parse()

	st, err := store.OpenTTL(*data, time.Now, *ttl)
	if err != nil {
		log.Fatal(err)
	}
	srv := server.New(st, web.FS)
	go srv.RunSweeper(context.Background(), time.Minute)
	if *ttl < 5*time.Minute {
		go srv.RunSweeper(context.Background(), time.Second)
	}
	log.Printf("clipd escuchando en http://%s", *listen)
	log.Fatal(http.ListenAndServe(*listen, srv))
}
