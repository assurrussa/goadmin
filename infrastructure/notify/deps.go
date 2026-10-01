package notify

import "github.com/assurrussa/gonotify/transport"

type (
	NotificationLevel   string
	NotificationChannel string
	NotificationManager = transport.Transport
)

const (
	LevelInfo     NotificationLevel = "info"
	LevelWarning  NotificationLevel = "warning"
	LevelError    NotificationLevel = "error"
	LevelCritical NotificationLevel = "critical"

	ChannelEmail    NotificationChannel = "email"
	ChannelTelegram NotificationChannel = "telegram"
)
