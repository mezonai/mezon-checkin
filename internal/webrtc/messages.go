package webrtc

import (
	"fmt"
	"log"
	"mezon-checkin-bot/internal/client"
)

// ============================================================
// CHECKIN CONFIRMATION MESSAGE
// ============================================================

func (w *WebRTCManager) SendCheckinConfirmation(channelID int64, userID int64, detectedName string) error {
	if w.dmManager == nil {
		return fmt.Errorf("DM manager not initialized")
	}

	log.Printf("📧 Sending check-in confirmation to user %d", userID)

	content := client.BuildCheckinConfirmationMessage(detectedName)

	if err := w.dmManager.SendDM(channelID, userID, content); err != nil {
		log.Printf("❌ Failed to send DM: %v", err)
		return err
	}

	log.Println("✅ Check-in confirmation sent!")

	w.startConfirmationTimeout(userID, channelID)

	return nil
}

// ============================================================
// CHECKIN SUCCESS MESSAGE
// ============================================================

func (w *WebRTCManager) SendCheckinSuccess(channelID int64, userID int64, userName string) error {
	if w.dmManager == nil {
		return fmt.Errorf("DM manager not initialized")
	}

	log.Printf("📧 Sending check-in success to user %d", userID)

	content := client.BuildCheckinSuccessMessage(userName)

	if err := w.dmManager.SendDM(channelID, userID, content); err != nil {
		log.Printf("❌ Failed to send DM: %v", err)
		return err
	}

	log.Println("✅ Check-in success message sent!")
	return nil
}

// ============================================================
// CHECKIN FAILED MESSAGE
// ============================================================

func (w *WebRTCManager) SendCheckinFailed(channelID int64, userID int64, reason string) error {
	if w.dmManager == nil {
		return fmt.Errorf("DM manager not initialized")
	}

	log.Printf("📧 Sending check-in failed to user %d", userID)

	content := client.BuildCheckinFailedMessage(reason)

	if err := w.dmManager.SendDM(channelID, userID, content); err != nil {
		log.Printf("❌ Failed to send DM: %v", err)
		return err
	}

	log.Println("✅ Check-in failed message sent!")
	return nil
}
