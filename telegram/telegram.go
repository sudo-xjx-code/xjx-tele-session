package telegram

import (
	"github.com/sudo-xjx-code/xjx-tele-session/mtproto"
)

type TelegramClient struct {
	mtproto *mtproto.MTProto
}

func NewClient() *TelegramClient {
	return &TelegramClient{
		mtproto: &mtproto.MTProto{},
	}
}

func (client *TelegramClient) Connect(addr string) error {
	return client.mtproto.Connect(addr)
}

func (client *TelegramClient) SendMessage(chatID int64, message string) error {
	// Implement message sending logic
	msg := []byte(message) // Simplified for example purposes
	return client.mtproto.SendMessage(msg)
}
