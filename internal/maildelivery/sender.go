package maildelivery

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"prods/internal/inquiries"
)

type Message struct {
	To      string
	Subject string
	Body    string
}

type Result struct {
	Status       inquiries.DeliveryStatus
	ErrorClass   string
	ErrorMessage string
}

type Sender interface {
	Send(context.Context, Message) Result
}

type SMTPConfig struct {
	Host       string
	Port       int
	Username   string
	Password   string
	From       string
	TLSMode    string
	Timeout    time.Duration
	ServerName string
	RootCAs    *x509.CertPool
}

type SMTPSender struct {
	config SMTPConfig
}

func NewSMTPSender(config SMTPConfig) (*SMTPSender, error) {
	config.Host = strings.TrimSpace(config.Host)
	config.Username = strings.TrimSpace(config.Username)
	config.From = strings.TrimSpace(config.From)
	config.TLSMode = strings.ToLower(strings.TrimSpace(config.TLSMode))
	config.ServerName = strings.TrimSpace(config.ServerName)
	if config.ServerName == "" {
		config.ServerName = config.Host
	}
	if config.Timeout <= 0 {
		config.Timeout = 15 * time.Second
	}
	if config.Host == "" || config.Port < 1 || config.Port > 65535 || !bareEmail(config.From) ||
		(config.TLSMode != "starttls" && config.TLSMode != "tls") ||
		(config.Username == "") != (config.Password == "") {
		return nil, errors.New("invalid secure SMTP configuration")
	}
	return &SMTPSender{config: config}, nil
}

func (sender *SMTPSender) Send(ctx context.Context, message Message) Result {
	if sender == nil || !bareEmail(message.To) || strings.ContainsAny(message.Subject, "\r\n") {
		return failed("invalid_message", errors.New("invalid SMTP recipient or subject"))
	}
	config := sender.config
	deadline := time.Now().Add(config.Timeout)
	if value, ok := ctx.Deadline(); ok && value.Before(deadline) {
		deadline = value
	}
	address := net.JoinHostPort(config.Host, fmt.Sprintf("%d", config.Port))
	dialer := &net.Dialer{Timeout: config.Timeout}
	var connection net.Conn
	var err error
	tlsConfig := &tls.Config{ServerName: config.ServerName, MinVersion: tls.VersionTLS12, RootCAs: config.RootCAs}
	if config.TLSMode == "tls" {
		connection, err = (&tls.Dialer{NetDialer: dialer, Config: tlsConfig}).DialContext(ctx, "tcp", address)
	} else {
		connection, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return failed("connect", err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(deadline); err != nil {
		return failed("deadline", err)
	}
	client, err := smtp.NewClient(connection, config.ServerName)
	if err != nil {
		return failed("greeting", err)
	}
	defer client.Close()
	if config.TLSMode == "starttls" {
		if err := client.StartTLS(tlsConfig); err != nil {
			return failed("starttls", err)
		}
	}
	if config.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", config.Username, config.Password, config.ServerName)); err != nil {
			return failed("auth", err)
		}
	}
	if err := client.Mail(config.From); err != nil {
		return failed("mail_from", err)
	}
	if err := client.Rcpt(message.To); err != nil {
		return failed("recipient", err)
	}
	writer, err := client.Data()
	if err != nil {
		return failed("data_open", err)
	}
	payload := smtpPayload(config.From, message)
	if _, err := io.WriteString(writer, payload); err != nil {
		_ = writer.Close()
		return unknown("data_write", err)
	}
	if err := writer.Close(); err != nil {
		return unknown("data_commit", err)
	}
	_ = client.Quit()
	return Result{Status: inquiries.DeliveryAccepted}
}

func smtpPayload(from string, message Message) string {
	return "From: " + from + "\r\n" +
		"To: " + message.To + "\r\n" +
		"Subject: " + message.Subject + "\r\n" +
		"Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"Content-Transfer-Encoding: 8bit\r\n\r\n" +
		strings.ReplaceAll(strings.ReplaceAll(message.Body, "\r\n", "\n"), "\n", "\r\n")
}

func bareEmail(value string) bool {
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Name == "" && strings.EqualFold(parsed.Address, value)
}

func failed(class string, err error) Result {
	return Result{Status: inquiries.DeliveryFailed, ErrorClass: class, ErrorMessage: err.Error()}
}

func unknown(class string, err error) Result {
	return Result{Status: inquiries.DeliveryUnknown, ErrorClass: class, ErrorMessage: err.Error()}
}
