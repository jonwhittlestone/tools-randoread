package mail

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
)

// fakeSMTPServer speaks just enough SMTP (EHLO/AUTH PLAIN/MAIL/RCPT/DATA) to
// exercise Send() without hitting a real mail provider in tests. It accepts
// on 127.0.0.1 so net/smtp's PlainAuth allows unencrypted auth (its
// isLocalhost exception), same as connecting to a real host over STARTTLS
// would after the TLS handshake.
func fakeSMTPServer(t *testing.T) (host, port string, dataCh chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() }) //nolint:errcheck

	dataCh = make(chan string, 1)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close() //nolint:errcheck

		r := bufio.NewReader(conn)
		fmt.Fprint(conn, "220 fake.smtp ESMTP\r\n") //nolint:errcheck

		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)

			switch {
			case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
				fmt.Fprint(conn, "250-fake.smtp\r\n250 AUTH PLAIN\r\n") //nolint:errcheck
			case strings.HasPrefix(line, "AUTH PLAIN"):
				fmt.Fprint(conn, "235 OK\r\n") //nolint:errcheck
			case strings.HasPrefix(line, "MAIL FROM"), strings.HasPrefix(line, "RCPT TO"):
				fmt.Fprint(conn, "250 OK\r\n") //nolint:errcheck
			case strings.HasPrefix(line, "DATA"):
				fmt.Fprint(conn, "354 Send data\r\n") //nolint:errcheck
				var body strings.Builder
				for {
					dl, err := r.ReadString('\n')
					if err != nil || dl == ".\r\n" {
						break
					}
					body.WriteString(dl)
				}
				dataCh <- body.String()
				fmt.Fprint(conn, "250 OK\r\n") //nolint:errcheck
			case strings.HasPrefix(line, "QUIT"):
				fmt.Fprint(conn, "221 Bye\r\n") //nolint:errcheck
				return
			default:
				fmt.Fprint(conn, "500 unrecognized\r\n") //nolint:errcheck
			}
		}
	}()

	host, port, err = net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	return host, port, dataCh
}

func TestSendDeliversHTMLMessage(t *testing.T) {
	host, port, dataCh := fakeSMTPServer(t)

	cfg := Config{Host: host, Port: port, Username: "user@example.com", Password: "app-password"}
	err := Send(cfg, "user@example.com", "jon@howapped.com", "randoread: hello", "<h1>Hello</h1>")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case data := <-dataCh:
		if !strings.Contains(data, "Subject: randoread: hello") {
			t.Errorf("expected Subject header, got: %s", data)
		}
		if !strings.Contains(data, "Content-Type: text/html") {
			t.Errorf("expected HTML content type, got: %s", data)
		}
		if !strings.Contains(data, "<h1>Hello</h1>") {
			t.Errorf("expected the HTML body, got: %s", data)
		}
	default:
		t.Fatal("server never received a DATA payload")
	}
}
