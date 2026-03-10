package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	port := flag.Int("port", 8080, "server port")
	dbPath := flag.String("db", "frictionax.db", "SQLite database path")
	flag.Parse()

	store, err := NewStore(*dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer store.Close()

	srv := NewServer(store)
	addr := fmt.Sprintf(":%d", *port)
	fmt.Fprintf(os.Stderr, "frictionax server listening on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, srv))
}
