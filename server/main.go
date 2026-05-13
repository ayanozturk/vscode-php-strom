package main

import (
	"log"
	"os"

	"github.com/ayanozturk/vscode-php-strom/phpls"
)

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.Ltime | log.Lshortfile)

	srv := phpls.NewServer(os.Stdin, os.Stdout)
	if err := srv.Run(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
