package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()
	fmt.Fprintf(os.Stderr, "http://localhost%s\n", *addr)
	http.ListenAndServe(*addr, http.FileServer(http.Dir(".")))
}
