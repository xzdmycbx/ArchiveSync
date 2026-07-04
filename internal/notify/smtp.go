package notify

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"archivesync/internal/models"
)

func init() { Register(models.NotifierSMTP, newSMTP) }

// smtpNotifier sends notification emails over SMTP.
type smtpNotifier struct {
	host string
	port int
	user string
	pass string
	from string
	to   []string
	tls  bool // implicit TLS (port 465); otherwise STARTTLS is attempted
}

// newSMTP builds an SMTP notifier, validating the required connection fields.
func newSMTP(n models.Notifier) (Notifier, error) {
	c := n.Config
	if c.SMTPHost == "" {
		return nil, fmt.Errorf("smtp: smtp_host is required")
	}
	if c.SMTPPort == 0 {
		return nil, fmt.Errorf("smtp: smtp_port is required")
	}
	if c.SMTPFrom == "" {
		return nil, fmt.Errorf("smtp: smtp_from is required")
	}
	if len(c.SMTPTo) == 0 {
		return nil, fmt.Errorf("smtp: at least one smtp_to recipient is required")
	}
	return &smtpNotifier{
		host: c.SMTPHost,
		port: c.SMTPPort,
		user: c.SMTPUser,
		pass: c.SMTPPass,
		from: c.SMTPFrom,
		to:   c.SMTPTo,
		tls:  c.SMTPTLS,
	}, nil
}

// Kind returns the notifier type identifier.
func (s *smtpNotifier) Kind() string { return "smtp" }

// Send delivers ev as an email. It honors ctx by aborting the wait when the
// context is cancelled.
func (s *smtpNotifier) Send(ctx context.Context, ev Event) error {
	errCh := make(chan error, 1)
	go func() { errCh <- s.send(ctx, ev) }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// send performs the synchronous SMTP conversation.
func (s *smtpNotifier) send(ctx context.Context, ev Event) error {
	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))
	msg := s.buildMessage(ev)

	dialer := &net.Dialer{Timeout: 15 * time.Second}

	var conn net.Conn
	var err error
	if s.tls {
		td := &tls.Dialer{NetDialer: dialer, Config: &tls.Config{ServerName: s.host}}
		conn, err = td.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("smtp: tls dial %s: %w", addr, err)
		}
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("smtp: dial %s: %w", addr, err)
		}
	}

	// Propagate context cancellation to the connection so a cancelled/timed-out
	// Send does not leave this goroutine blocked on the SMTP conversation.
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	ctxDone := make(chan struct{})
	defer close(ctxDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-ctxDone:
		}
	}()

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp: new client: %w", err)
	}
	defer client.Close()

	if !s.tls {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: s.host}); err != nil {
				return fmt.Errorf("smtp: starttls: %w", err)
			}
		} else if !isLoopbackHost(s.host) {
			// Refuse to send credentials/mail in cleartext to a remote server.
			// A local relay (localhost) is trusted and allowed without TLS.
			return fmt.Errorf("smtp: server %q does not offer STARTTLS; refusing to send in cleartext (use port 465 with implicit TLS)", s.host)
		}
	}

	// Authenticate only after TLS has been established.
	if s.user != "" {
		auth := smtp.PlainAuth("", s.user, s.pass, s.host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp: auth: %w", err)
		}
	}

	if err := client.Mail(s.from); err != nil {
		return fmt.Errorf("smtp: mail from %s: %w", s.from, err)
	}
	for _, rcpt := range s.to {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp: rcpt %s: %w", rcpt, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp: data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp: write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp: close body: %w", err)
	}
	return client.Quit()
}

// isLoopbackHost reports whether host is localhost or a loopback IP, for which
// cleartext SMTP (a local relay) is acceptable.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// buildMessage assembles an RFC822 message with a UTF-8 text/plain body.
func (s *smtpNotifier) buildMessage(ev Event) []byte {
	body := plainText(ev)
	subj := mime.QEncoding.Encode("utf-8", subject(ev))

	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", s.from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(s.to, ", "))
	fmt.Fprintf(&b, "Subject: %s\r\n", subj)
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n")
	b.WriteString("\r\n")

	enc := base64.StdEncoding.EncodeToString([]byte(body))
	for i := 0; i < len(enc); i += 76 {
		end := i + 76
		if end > len(enc) {
			end = len(enc)
		}
		b.WriteString(enc[i:end])
		b.WriteString("\r\n")
	}
	return []byte(b.String())
}
