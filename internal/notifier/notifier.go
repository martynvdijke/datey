package notifier

import "context"

type Notifier interface {
	Send(ctx context.Context, title, message string) error
	SendTo(ctx context.Context, title, message string, target string) error
	Name() string
	IsConfigured() bool
}
