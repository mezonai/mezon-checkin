package client

import (
	"fmt"
	"mezon-checkin-bot/models"
	"time"
)

// ============================================================
// MESSAGE BUILDER CONSTANTS
// ============================================================

const (
	ColorPurple = "#71368A"
	ColorGreen  = "#00FF00"
	ColorRed    = "#FF0000"

	ButtonStyleSuccess = 3
	ButtonStyleDanger  = 4
	ButtonTypePrimary  = 1

	MezonIconURL = "https://cdn.mezon.vn/1837043892743049216/1840654271217930240/1827994776956309500/857_0246x0w.webp"
	FooterText   = "Powered by Mezon"
)

// ============================================================
// CHECK-IN MESSAGES
// ============================================================

func BuildCheckinConfirmationMessage(userName string) models.ChannelMessageContent {
	return models.ChannelMessageContent{
		Embed: []models.InteractiveMessageEmbed{
			buildEmbed(
				ColorPurple,
				"Xác định danh tính thành công - Cần xác minh vị trí",
				fmt.Sprintf("Xin chào %s. Vui lòng gửi vị trí của bạn về cho hệ thống trong vòng 1 phút để hoàn thành check-in!", userName),
			),
		},
	}
}

func BuildCheckinSuccessMessage(userName string) models.ChannelMessageContent {
	return models.ChannelMessageContent{
		Embed: []models.InteractiveMessageEmbed{
			buildEmbed(
				ColorGreen,
				"✅ Check-in thành công!",
				fmt.Sprintf("Chào mừng %s! Bạn đã check-in thành công.", userName),
			),
		},
	}
}

func BuildCheckinFailedMessage(reason string) models.ChannelMessageContent {
	return models.ChannelMessageContent{
		Embed: []models.InteractiveMessageEmbed{
			buildEmbed(
				ColorRed,
				"❌ Check-in thất bại",
				fmt.Sprintf("Lý do: %s", reason),
			),
		},
	}
}

// ============================================================
// EMBED BUILDER
// ============================================================

func buildEmbed(color, title, description string) models.InteractiveMessageEmbed {
	return models.InteractiveMessageEmbed{
		Color:       color,
		Title:       title,
		Description: description,
		Timestamp:   time.Now().Format(time.RFC3339),
		Footer:      buildFooter(),
	}
}

func buildFooter() *models.EmbedFooter {
	return &models.EmbedFooter{
		Text:    FooterText,
		IconURL: MezonIconURL,
	}
}

// ============================================================
// BUTTON BUILDER
// ============================================================

func buildButton(id, label string, style int) models.MessageComponent {
	return models.MessageComponent{
		ID:   id,
		Type: ButtonTypePrimary,
		Component: models.ComponentDetails{
			Label: label,
			Style: style,
		},
	}
}

// ============================================================
// CUSTOM MESSAGE BUILDERS
// ============================================================

func BuildSimpleTextMessage(text string) models.ChannelMessageContent {
	return models.ChannelMessageContent{
		Embed: []models.InteractiveMessageEmbed{
			buildEmbed(ColorPurple, "", text),
		},
	}
}

func BuildSuccessMessage(title, description string) models.ChannelMessageContent {
	return models.ChannelMessageContent{
		Embed: []models.InteractiveMessageEmbed{
			buildEmbed(ColorGreen, title, description),
		},
	}
}

func BuildErrorMessage(title, description string) models.ChannelMessageContent {
	return models.ChannelMessageContent{
		Embed: []models.InteractiveMessageEmbed{
			buildEmbed(ColorRed, title, description),
		},
	}
}

func BuildMessageWithButtons(title, description string, buttons []MessageButton) models.ChannelMessageContent {
	components := make([]models.MessageComponent, len(buttons))
	for i, btn := range buttons {
		components[i] = buildButton(btn.ID, btn.Label, btn.Style)
	}

	return models.ChannelMessageContent{
		Embed: []models.InteractiveMessageEmbed{
			buildEmbed(ColorPurple, title, description),
		},
		Components: components,
	}
}

// ============================================================
// HELPER TYPES
// ============================================================

type MessageButton struct {
	ID    string
	Label string
	Style int
}
