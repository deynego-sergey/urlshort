package notification

import (
	"context"
	"fmt"
	"log"
	"net/smtp"
	"os"
)

// EmailConfig инкапсулирует настройки, считываемые из переменных окружения
type EmailConfig struct {
	Host     string
	Port     string
	From     string
	Password string
}

// LoadEmailConfigFromEnv загружает переменные для работы с почтой
func LoadEmailConfigFromEnv() (*EmailConfig, error) {
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	from := os.Getenv("SMTP_FROM")
	pass := os.Getenv("SMTP_PASSWORD")

	// Если критические переменные не заданы, возвращаем ошибку при старте
	if host == "" || port == "" || from == "" {
		return nil, fmt.Errorf("missing critical SMTP environment variables")
	}

	return &EmailConfig{
		Host:     host,
		Port:     port,
		From:     from,
		Password: pass,
	}, nil
}

type EmailSender struct {
	config *EmailConfig
}

func NewEmailSender(cfg *EmailConfig) INotificationSender {
	return &EmailSender{config: cfg}
}

func (s *EmailSender) Send(ctx context.Context, n Notification) error {
	// Базовая очистка и валидация перед отправкой
	if n.Recipient == "" || n.Body == "" {
		return fmt.Errorf("invalid notification payload: empty recipient or body")
	}

	msg := []byte("To: " + n.Recipient + "\r\n" +
		"Subject: " + n.Subject + "\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		n.Body + "\r\n")

	addr := fmt.Sprintf("%s:%s", s.config.Host, s.config.Port)

	// Если пароль пустой, работаем без аутентификации (например, локальный Mailpit/Mailhog для тестов)
	var auth smtp.Auth
	if s.config.Password != "" {
		auth = smtp.PlainAuth("", s.config.From, s.config.Password, s.config.Host)
	}

	// Выполняем сетевой запрос отправки
	err := smtp.SendMail(addr, auth, s.config.From, []string{n.Recipient}, msg)
	if err != nil {
		log.Printf("[ERROR] Failed to send email to %s: %v", n.Recipient, err)
		return fmt.Errorf("smtp send error: %w", err)
	}

	log.Printf("[SUCCESS] Email sent successfully to %s", n.Recipient)
	return nil
}
