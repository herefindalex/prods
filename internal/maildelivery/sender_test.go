package maildelivery

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"prods/internal/inquiries"
)

func TestSMTPSenderRequiresTLSAndClassifiesPostDATAUncertainty(t *testing.T) {
	if _, err := NewSMTPSender(SMTPConfig{Host: "smtp.example.test", Port: 25, From: "rfq@example.test"}); err == nil {
		t.Fatal("SMTP without an explicit secure TLS mode was accepted")
	}
	for _, accept := range []bool{true, false} {
		address, roots, done := startTLSTestSMTP(t, accept)
		host, portText, err := net.SplitHostPort(address)
		if err != nil {
			t.Fatal(err)
		}
		port, _ := strconv.Atoi(portText)
		sender, err := NewSMTPSender(SMTPConfig{
			Host: host, Port: port, From: "rfq@example.test", TLSMode: "tls", RootCAs: roots, Timeout: 2 * time.Second,
		})
		if err != nil {
			t.Fatal(err)
		}
		result := sender.Send(context.Background(), Message{To: "sales@example.test", Subject: "Prods RFQ rfq_test", Body: "hello\nworld"})
		if accept && result.Status != inquiries.DeliveryAccepted {
			t.Fatalf("accepted SMTP result = %#v", result)
		}
		if !accept && (result.Status != inquiries.DeliveryUnknown || result.ErrorClass != "data_commit") {
			t.Fatalf("post-DATA disconnect result = %#v", result)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("test SMTP server did not exit")
		}
	}
	address, roots, done := startSTARTTLSTestSMTP(t)
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portText)
	sender, err := NewSMTPSender(SMTPConfig{
		Host: host, Port: port, From: "rfq@example.test", TLSMode: "starttls", RootCAs: roots, Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result := sender.Send(context.Background(), Message{To: "sales@example.test", Subject: "Prods RFQ rfq_starttls", Body: "hello"}); result.Status != inquiries.DeliveryAccepted {
		t.Fatalf("STARTTLS result = %#v", result)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func startSTARTTLSTestSMTP(t *testing.T) (string, *x509.CertPool, <-chan error) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: "localhost"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}),
	)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots.AddCert(parsed)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	done := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer connection.Close()
		reader := bufio.NewReader(connection)
		writer := bufio.NewWriter(connection)
		write := func(value string) error {
			if _, err := writer.WriteString(value); err != nil {
				return err
			}
			return writer.Flush()
		}
		read := func(prefix string) error {
			line, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(line)), prefix) {
				return &smtpTestError{want: prefix, got: line}
			}
			return nil
		}
		if err := write("220 test ESMTP\r\n"); err != nil {
			done <- err
			return
		}
		if err := read("EHLO"); err != nil {
			done <- err
			return
		}
		if err := write("250-test\r\n250 STARTTLS\r\n"); err != nil {
			done <- err
			return
		}
		if err := read("STARTTLS"); err != nil {
			done <- err
			return
		}
		if err := write("220 ready\r\n"); err != nil {
			done <- err
			return
		}
		tlsConnection := tls.Server(connection, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
		if err := tlsConnection.Handshake(); err != nil {
			done <- err
			return
		}
		reader = bufio.NewReader(tlsConnection)
		writer = bufio.NewWriter(tlsConnection)
		for _, step := range []struct{ prefix, response string }{
			{"EHLO", "250 test\r\n"}, {"MAIL FROM", "250 ok\r\n"}, {"RCPT TO", "250 ok\r\n"}, {"DATA", "354 end with dot\r\n"},
		} {
			if err := read(step.prefix); err != nil {
				done <- err
				return
			}
			if err := write(step.response); err != nil {
				done <- err
				return
			}
		}
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				done <- err
				return
			}
			if line == ".\r\n" {
				break
			}
		}
		if err := write("250 queued\r\n"); err != nil {
			done <- err
			return
		}
		if err := read("QUIT"); err != nil {
			done <- err
			return
		}
		done <- write("221 bye\r\n")
	}()
	return listener.Addr().String(), roots, done
}

type smtpTestError struct{ want, got string }

func (err *smtpTestError) Error() string {
	return "SMTP command mismatch: want " + err.want + ", got " + err.got
}

func startTLSTestSMTP(t *testing.T, accept bool) (string, *x509.CertPool, <-chan error) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: "localhost"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER := x509.MarshalPKCS1PrivateKey(privateKey)
	certificate, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER}),
	)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	roots := x509.NewCertPool()
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots.AddCert(parsed)
	done := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer connection.Close()
		reader := bufio.NewReader(connection)
		writer := bufio.NewWriter(connection)
		write := func(value string) error {
			if _, err := writer.WriteString(value); err != nil {
				return err
			}
			return writer.Flush()
		}
		if err := write("220 test ESMTP\r\n"); err != nil {
			done <- err
			return
		}
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				done <- nil
				return
			}
			command := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(command, "EHLO"):
				err = write("250-test\r\n250 8BITMIME\r\n")
			case strings.HasPrefix(command, "MAIL FROM") || strings.HasPrefix(command, "RCPT TO"):
				err = write("250 ok\r\n")
			case command == "DATA":
				if err = write("354 end with dot\r\n"); err == nil {
					for {
						line, err = reader.ReadString('\n')
						if err != nil || line == ".\r\n" {
							break
						}
					}
					if err == nil && accept {
						err = write("250 queued\r\n")
					} else if err == nil {
						_ = connection.Close()
						done <- nil
						return
					}
				}
			case command == "QUIT":
				err = write("221 bye\r\n")
				done <- err
				return
			default:
				err = write("500 unexpected\r\n")
			}
			if err != nil {
				done <- err
				return
			}
		}
	}()
	return listener.Addr().String(), roots, done
}
