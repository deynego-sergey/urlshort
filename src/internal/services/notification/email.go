package notification

import (
	"context"
	"log"
)

type IEmailService interface {
	SendConfirmationCode(ctx context.Context, toEmail, code string) error
	SendResetToken(ctx context.Context, toEmail, token string) error
}

type emailService struct {
	// Здесь в будущем будет конфигурация SMTP/Mailgun/SendGrid из окружения
}

func NewEmailService() IEmailService {
	return &emailService{}
}

func (s *emailService) SendConfirmationCode(ctx context.Context, toEmail, code string) error {
	// Асинхронно или через горутину (безопасно для HTTP-потока) имитируем отправку
	go func() {
		log.Printf("[EMAIL] Отправлено письмо на %s. Текст: Ваш код подтверждения регистрации: %s. Или перейдите по ссылке: http://localhost:8080/confirm?token=%s", toEmail, code, code)
	}()
	return nil
}

func (s *emailService) SendResetToken(ctx context.Context, toEmail, token string) error {
	go func() {
		log.Printf("[EMAIL] Отправлено письмо на %s. Текст: Для сброса пароля используйте код: %s", toEmail, token)
	}()
	return nil
}
