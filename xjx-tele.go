package main

import (
	"fmt"

	"github.com/sudo-xjx-code/xjx-tele-session/telegram"
)

func main() {
	client := telegram.NewClient()

	err := client.Connect("149.154.167.50:443") // Telegram test server address
	if err != nil {
		fmt.Println("Failed to connect:", err)
		return
	}

	// Example: Send authentication code
	err = client.SendAuthCode("+6281234567890")
	if err != nil {
		fmt.Println("Failed to send auth code:", err)
		return
	}

	// Example: Login with code
	err = client.Login("12345")
	if err != nil {
		fmt.Println("Failed to login:", err)
		return
	}

	// Example: Send a message
	err = client.SendMessage(123456789, "Hello, Telegram!")
	if err != nil {
		fmt.Println("Failed to send message:", err)
		return
	}

	// Start receiving updates
	client.StartReceivingUpdates()
}
