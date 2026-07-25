// Command send-to-remarkable uploads an EPUB to a reMarkable tablet over
// SFTP and registers it in xochitl's document store, so it shows up in
// "My Files" rather than sitting invisibly in the root user's home dir.
//
// Requires SSH over WLAN to already be enabled on the tablet (one-time
// setup via USB — see main-remarkable.md in the 26-remarkable-tablet vault
// project) and REMARKABLE_PASSWORD set in the environment.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/jonwhittlestone/tools-randoread/internal/remarkable"
)

func main() {
	host := flag.String("host", envOr("REMARKABLE_HOST", "192.168.0.147"), "reMarkable tablet IP address")
	port := flag.String("port", envOr("REMARKABLE_SSH_PORT", "22"), "SSH port")
	user := flag.String("user", envOr("REMARKABLE_USER", "root"), "SSH user")
	title := flag.String("title", "", "Visible name shown in My Files (default: filename without extension)")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: %s [flags] <path-to-epub>\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}
	epubPath := flag.Arg(0)

	password := os.Getenv("REMARKABLE_PASSWORD")
	if password == "" {
		fmt.Fprintln(os.Stderr, "REMARKABLE_PASSWORD environment variable must be set (tablet's Settings > Help > Copyrights and licenses > General information > GPLv3 Compliance screen)")
		os.Exit(1)
	}

	cfg := remarkable.Config{Host: *host, Port: *port, User: *user, Password: password}

	result, err := remarkable.SendEpub(cfg, epubPath, *title)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAILED: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("SUCCESS: %q uploaded as %s (%d bytes), xochitl restarted.\n", result.VisibleName, result.UUID, result.Bytes)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
