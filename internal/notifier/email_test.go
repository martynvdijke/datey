package notifier

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/datey/datey/internal/config"
)

func newTestConfig() *config.Config {
	return &config.Config{
		SMTPHost:    "smtp.example.com",
		SMTPPort:    587,
		SMTPUser:    "user@example.com",
		SMTPPass:    "secret",
		SMTPTLS:     true,
		SMTPTimeout: 10,
		NotifyEmail: "test@example.com",
	}
}

func TestEmailNotifier_Name(t *testing.T) {
	n := NewEmailNotifier(newTestConfig())
	if got := n.Name(); got != "email" {
		t.Errorf("Name() = %q, want %q", got, "email")
	}
}

func TestEmailNotifier_IsConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{"fully configured", newTestConfig(), true},
		{"missing host", &config.Config{SMTPHost: "", NotifyEmail: "a@b.com"}, false},
		{"missing email", &config.Config{SMTPHost: "smtp.example.com", NotifyEmail: ""}, false},
		{"both missing", &config.Config{SMTPHost: "", NotifyEmail: ""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewEmailNotifier(tt.cfg)
			if got := n.IsConfigured(); got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEmailNotifier_RenderHTML(t *testing.T) {
	n := NewEmailNotifier(newTestConfig())
	html, err := n.renderHTML("Test Title", "Test Message")
	if err != nil {
		t.Fatalf("renderHTML() error = %v", err)
	}
	if !strings.Contains(html, "Test Title") {
		t.Errorf("renderHTML() missing title")
	}
	if !strings.Contains(html, "Test Message") {
		t.Errorf("renderHTML() missing message")
	}
	if !strings.Contains(html, "Sent at") {
		t.Errorf("renderHTML() missing timestamp")
	}
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Errorf("renderHTML() missing doctype")
	}
}

func TestEmailNotifier_Send_ReturnsErrorOnNoServer(t *testing.T) {
	cfg := newTestConfig()
	cfg.SMTPHost = "127.0.0.1"
	cfg.SMTPPort = 51999
	cfg.SMTPTimeout = 1

	n := NewEmailNotifier(cfg)
	err := n.Send(context.Background(), "Test", "Body")
	if err == nil {
		t.Fatal("Send() expected error, got nil")
	}
}

// generateTestCert creates a self-signed TLS cert for testing.
func generateTestCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create cert: %v", err)
	}
	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}
}

func TestEmailNotifier_TLSModeDirect(t *testing.T) {
	// Test sendDirectTLS by connecting to a plain TCP listener.
	// The TLS handshake will fail since the server doesn't speak TLS.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer ln.Close()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		buf := make([]byte, 1024)
		conn.Read(buf)
	}()

	var port int
	fmt.Sscanf(portStr, "%d", &port)

	cfg := newTestConfig()
	cfg.SMTPHost = host
	cfg.SMTPPort = port
	cfg.SMTPTLS = true
	cfg.SMTPTimeout = 1

	n := NewEmailNotifier(cfg)
	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)
	err = n.sendDirectTLS(context.Background(), addr, "test msg", time.Second)
	<-serverDone

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "tls dial") {
		t.Errorf("error = %q, want substring %q", err.Error(), "tls dial")
	}
}

func TestEmailNotifier_TLSModeSTARTTLS(t *testing.T) {
	// Test sendSTARTTLS: server sends greeting but doesn't support STARTTLS
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer ln.Close()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Send SMTP greeting but no STARTTLS capability
		conn.Write([]byte("220 localhost ESMTP test\r\n"))
		conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		buf := make([]byte, 1024)
		conn.Read(buf)
	}()

	var port int
	fmt.Sscanf(portStr, "%d", &port)

	cfg := newTestConfig()
	cfg.SMTPHost = host
	cfg.SMTPPort = port
	cfg.SMTPTLS = true
	cfg.SMTPTimeout = 1

	n := NewEmailNotifier(cfg)
	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)
	err = n.sendSTARTTLS(context.Background(), addr, "test msg", time.Second)
	<-serverDone

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "starttls") {
		t.Errorf("error = %q, want substring %q", err.Error(), "starttls")
	}
}

func TestEmailNotifier_TLSModePlain(t *testing.T) {
	// Test sendPlain: connects to a server that doesn't handle SMTP properly
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer ln.Close()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 1024)
		conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		conn.Read(buf)
	}()

	var port int
	fmt.Sscanf(portStr, "%d", &port)

	cfg := newTestConfig()
	cfg.SMTPHost = host
	cfg.SMTPPort = port
	cfg.SMTPTLS = false
	cfg.SMTPTimeout = 1

	n := NewEmailNotifier(cfg)
	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)
	err = n.sendPlain(context.Background(), addr, "test msg", time.Second)
	<-serverDone

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestEmailNotifier_SMTPTimeoutConfig(t *testing.T) {
	// Verify the SMTPTimeout config value is respected by the dial timeout.
	// Connect to a port where nobody is listening — should fail fast.
	cfg := newTestConfig()
	cfg.SMTPHost = "127.0.0.1"
	cfg.SMTPPort = 51999
	cfg.SMTPTimeout = 1

	n := NewEmailNotifier(cfg)
	start := time.Now()
	err := n.Send(context.Background(), "Test", "Body")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if elapsed > 3*time.Second {
		t.Errorf("Send() took %v, expected fast failure with 1s timeout", elapsed)
	}
}

func TestEmailNotifier_Send_SMTPDialog(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer ln.Close()

	serverDone := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()

		conn.Write([]byte("220 localhost ESMTP test\r\n"))

		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			serverDone <- fmt.Errorf("read error: %w", err)
			return
		}
		line := string(buf[:n])
		if !strings.HasPrefix(line, "EHLO") && !strings.HasPrefix(line, "HELO") {
			serverDone <- fmt.Errorf("expected EHLO/HELO, got: %s", line)
			return
		}
		conn.Write([]byte("250-localhost\r\n250 AUTH LOGIN PLAIN\r\n"))

		n, err = conn.Read(buf)
		if err != nil {
			serverDone <- fmt.Errorf("read error: %w", err)
			return
		}
		line = string(buf[:n])
		if !strings.HasPrefix(line, "AUTH") {
			serverDone <- fmt.Errorf("expected AUTH, got: %s", line)
			return
		}
		conn.Write([]byte("235 Authentication successful\r\n"))

		n, err = conn.Read(buf)
		if err != nil {
			serverDone <- fmt.Errorf("read error: %w", err)
			return
		}
		line = string(buf[:n])
		if !strings.HasPrefix(line, "MAIL FROM") {
			serverDone <- fmt.Errorf("expected MAIL FROM, got: %s", line)
			return
		}
		conn.Write([]byte("250 OK\r\n"))

		n, err = conn.Read(buf)
		if err != nil {
			serverDone <- fmt.Errorf("read error: %w", err)
			return
		}
		line = string(buf[:n])
		if !strings.HasPrefix(line, "RCPT TO") {
			serverDone <- fmt.Errorf("expected RCPT TO, got: %s", line)
			return
		}
		conn.Write([]byte("250 OK\r\n"))

		n, err = conn.Read(buf)
		if err != nil {
			serverDone <- fmt.Errorf("read error: %w", err)
			return
		}
		line = string(buf[:n])
		if !strings.HasPrefix(line, "DATA") {
			serverDone <- fmt.Errorf("expected DATA, got: %s", line)
			return
		}
		conn.Write([]byte("354 Start mail input\r\n"))

		var msgBuf strings.Builder
		for {
			b := make([]byte, 1)
			_, err := conn.Read(b)
			if err != nil {
				break
			}
			msgBuf.WriteByte(b[0])
			s := msgBuf.String()
			if strings.HasSuffix(s, "\r\n.\r\n") {
				break
			}
		}
		conn.Write([]byte("250 OK: queued\r\n"))

		n, err = conn.Read(buf)
		if err != nil {
			serverDone <- fmt.Errorf("read error: %w", err)
			return
		}
		line = string(buf[:n])
		if !strings.HasPrefix(line, "QUIT") {
			serverDone <- fmt.Errorf("expected QUIT, got: %s", line)
			return
		}
		conn.Write([]byte("221 Bye\r\n"))

		serverDone <- nil
		_ = msgBuf
	}()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	cfg := newTestConfig()
	cfg.SMTPHost = host
	cfg.SMTPPort = port
	cfg.SMTPTLS = false
	cfg.SMTPTimeout = 2

	n := NewEmailNotifier(cfg)
	err = n.Send(context.Background(), "Test Subject", "Hello Body")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if err := <-serverDone; err != nil {
		t.Fatalf("SMTP server error: %v", err)
	}
}

func TestEmailNotifier_TLS_Direct(t *testing.T) {
	cert := generateTestCert(t)

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
	})
	if err != nil {
		t.Fatalf("failed to start TLS listener: %v", err)
	}
	defer ln.Close()

	serverDone := make(chan error, 1)
	go func() {
		defer func() { serverDone <- nil }()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		conn.Write([]byte("220 localhost ESMTP test\r\n"))

		buf := make([]byte, 1024)
		_, err = conn.Read(buf)
		if err != nil {
			return
		}

		// Read remaining commands until quit
		for {
			b := make([]byte, 1024)
			_, err := conn.Read(b)
			if err != nil {
				break
			}
			conn.Write([]byte("250 OK\r\n"))
		}
	}()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	cfg := newTestConfig()
	cfg.SMTPHost = "localhost"
	cfg.SMTPPort = port
	cfg.SMTPTLS = true
	cfg.SMTPTimeout = 2

	n := NewEmailNotifier(cfg)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	err = n.sendDirectTLS(context.Background(), addr, "", time.Duration(cfg.SMTPTimeout)*time.Second)
	<-serverDone

	// Expect TLS cert verification error (self-signed cert not trusted)
	if err == nil {
		t.Fatal("expected TLS verification error, got nil")
	}
	if !strings.Contains(err.Error(), "tls: failed to verify certificate") {
		t.Fatalf("unexpected error: %v", err)
	}
}
