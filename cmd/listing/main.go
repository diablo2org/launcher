// Command listing regenerates servers/index.json from servers/*.json. Run it
// after adding or changing a listing entry:
//
//	go run ./cmd/listing
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/diablo2org/launcher/internal/core"
)

func main() {
	dir := flag.String("dir", "servers", "folder of listing entries")
	flag.Parse()

	data, err := core.BuildIndex(*dir)
	if err != nil {
		log.Fatal(err)
	}

	out := filepath.Join(*dir, "index.json")
	if err := os.WriteFile(out, data, 0o644); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Wrote", out)
}
