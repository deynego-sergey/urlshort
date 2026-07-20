package notification

import (
	"context"
)

// TargetType определяет, куда отправляется уведомление
type TargetType string

const (
	TargetEmail    TargetType = "email"
	TargetTelegram TargetType = "telegram"
)

// Notification описывает единый объект уведомления
type Notification struct {
	TargetType TargetType
	Recipient  string // email адрес или telegram id/username
	Subject    string // Для Email (может быть пустым для бота)
	Body       string // Текст сообщения или код
}

// INotificationSender — базовый интерфейс для конкретных каналов отправки
type INotificationSender interface {
	Send(ctx context.Context, n Notification) error
}

// INotificationService — верхнеуровневый сервис, который вызывают другие части системы
type INotificationService interface {
	SendSync(ctx context.Context, n Notification) error
	SendAsync(ctx context.Context, n Notification)
}

// NotificationService объединяет каналы отправки
type NotificationService struct {
	senders map[TargetType]INotificationSender
}

func NewNotificationService(senders map[TargetType]INotificationSender) INotificationService {
	return &NotificationService{
		senders: senders,
	}
}

// SendSync отправляет уведомление в текущем потоке с возвратом ошибки
func (s *NotificationService) SendSync(ctx context.Context, n Notification) error {
	sender, exists := s.senders[n.TargetType]
	if !exists {
		return &SenderNotFoundError{Target: n.TargetType}
	}
	return sender.Send(ctx, n)
}

// SendAsync отправляет уведомление асинхронно (в горутине) без блокировки вызывающего потока
func (s *NotificationService) SendAsync(ctx context.Context, n Notification) {
	sender, exists := s.senders[n.TargetType]
	if !exists {
		return
	}
	go func() {
		// Используем Background контекст, чтобы отмена HTTP-запроса не убила отправку сообщения
		_ = sender.Send(context.Background(), n)
	}()
}

// Кастомная ошибка для логирования безопасных отказов
type SenderNotFoundError struct {
	Target TargetType
}

func (e *SenderNotFoundError) Error() string {
	return "notification sender not implemented for target: " + string(e.Target)
}
