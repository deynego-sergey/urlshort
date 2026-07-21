package notification

import (
	"context"
	"fmt"
	"log"
	"net/smtp"
	"os"
	"strings"
)

// EmailConfig инкапсулирует настройки, считываемые из переменных окружения
type EmailConfig struct {
	Host     string
	Port     string
	From     string
	Password string
	UserName string
}

// LoadEmailConfigFromEnv загружает переменные для работы с почтой
func LoadEmailConfigFromEnv() (*EmailConfig, error) {
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	from := os.Getenv("SMTP_FROM")
	pass := os.Getenv("SMTP_PASSWORD")
	smtpusername := os.Getenv("SMTP_USERNAME")

	// Если критические переменные не заданы, возвращаем ошибку при старте
	if host == "" || port == "" || from == "" {
		return nil, fmt.Errorf("missing critical SMTP environment variables")
	}

	return &EmailConfig{
		Host:     host,
		Port:     port,
		From:     from,
		Password: pass,
		UserName: smtpusername,
	}, nil
}

type EmailSender struct {
	config *EmailConfig
}

func NewEmailSender(cfg *EmailConfig) INotificationSender {
	return &EmailSender{config: cfg}
}
func (s *EmailSender) Send(ctx context.Context, n Notification) error {
	if n.Recipient == "" || n.Body == "" {
		return fmt.Errorf("invalid notification payload: empty recipient or body")
	}

	if s.config.From == "" {
		return fmt.Errorf("smtp send error: sender address (From) is empty")
	}

	msg := []byte("From: " + s.config.From + "\r\n" +
		"To: " + n.Recipient + "\r\n" +
		"Subject: " + n.Subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		n.Body + "\r\n")

	addr := fmt.Sprintf("%s:%s", s.config.Host, s.config.Port)

	var auth smtp.Auth
	if s.config.Password != "" {
		auth = smtp.PlainAuth("", s.config.UserName, s.config.Password, s.config.Host)
	}

	// s.config.From должен быть строго в формате "user@domain.com"
	err := smtp.SendMail(addr, auth, strings.TrimSpace(s.config.From), []string{n.Recipient}, msg)
	if err != nil {
		log.Printf("[ERROR] Failed to send email to %s: %v", n.Recipient, err)
		return fmt.Errorf("smtp send error: %w", err)
	}

	log.Printf("[SUCCESS] Email sent successfully to %s", n.Recipient)
	return nil
}
