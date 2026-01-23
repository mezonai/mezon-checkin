package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	rtapi "mezon-checkin-bot/mezon-protobuf/go/rtapi"
	"mezon-checkin-bot/models"
	"time"
)

// ============================================================
// SEND DM MESSAGES
// ============================================================

func (dm *DMManager) SendDM(channelID int64, userID int64, content models.ChannelMessageContent) error {
	return dm.SendDMWithContext(context.Background(), channelID, userID, content)
}

func (dm *DMManager) SendDMWithContext(ctx context.Context, channelID int64, userID int64, content models.ChannelMessageContent) error {
	// Ensure DM clan is ready (lazy init)
	if err := dm.ensureDMReady(); err != nil {
		return fmt.Errorf("failed to ensure DM ready: %w", err)
	}

	// Check connection health
	if !dm.client.IsConnected() {
		log.Println("   ⚠️  WebSocket disconnected, waiting for reconnection...")

		// Wait for reconnection (max 5s)
		for i := 0; i < 10; i++ {
			time.Sleep(500 * time.Millisecond)
			if dm.client.IsConnected() {
				log.Println("   ✅ Connection restored, sending message...")
				break
			}
		}

		if !dm.client.IsConnected() {
			return fmt.Errorf("websocket not connected after waiting")
		}
	}

	// Build protobuf envelope
	envelope, err := dm.buildDMEnvelope(channelID, content)
	if err != nil {
		return err
	}

	// Send with response (to ensure message is delivered)
	if err := dm.sendDMMessage(ctx, envelope, channelID, userID); err != nil {
		return err
	}

	log.Printf("✅ DM sent successfully!")
	return nil
}

// ============================================================
// MESSAGE BUILDING (PROTOBUF)
// ============================================================

func (dm *DMManager) buildDMEnvelope(channelID int64, content models.ChannelMessageContent) (*rtapi.Envelope, error) {
	// Convert content to JSON string (models.ChannelMessageContent is not a protobuf message)
	contentJSON, err := json.Marshal(content)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal content: %w", err)
	}

	// Build protobuf envelope
	envelope := &rtapi.Envelope{
		Message: &rtapi.Envelope_ChannelMessageSend{
			ChannelMessageSend: &rtapi.ChannelMessageSend{
				ClanId:    DMClanID,
				ChannelId: channelID,
				Mode:      DMChannelType, // DM mode
				IsPublic:  false,
				Content:   string(contentJSON),
			},
		},
	}

	return envelope, nil
}

func (dm *DMManager) sendDMMessage(ctx context.Context, envelope *rtapi.Envelope, channelID int64, userID int64) error {
	// Check context
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-dm.client.ctx.Done():
		return fmt.Errorf("client closed")
	default:
	}

	dm.logSendDM(channelID, userID)

	// Send with response to ensure delivery
	timeout := 5 * time.Second
	response, err := dm.client.sendWithResponse(envelope, timeout)
	if err != nil {
		return fmt.Errorf("send message failed: %w", err)
	}

	// Check for server error
	if response.GetError() != nil {
		return fmt.Errorf("server error: code=%d, message=%s",
			response.GetError().Code, response.GetError().Message)
	}

	// Get message ACK
	if ack := response.GetChannelMessageAck(); ack != nil {
		log.Printf("   Message ID: %d", ack.MessageId)
		log.Printf("   Create Time: %d", ack.CreateTimeSeconds)
		return nil
	}

	return nil
}

func (dm *DMManager) logSendDM(channelID int64, userID int64) {
	log.Printf("📤 Sending DM...")
	log.Printf("   Channel ID: %d", channelID)
	log.Printf("   User ID: %d", userID)
}
