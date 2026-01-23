package client

import (
	"encoding/json"
	"fmt"
	"log"
	"mezon-checkin-bot/mezon-protobuf/go/api"
	"mezon-checkin-bot/mezon-protobuf/go/rtapi"
	"mezon-checkin-bot/models"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ============================================================
// CONSTANTS
// ============================================================

const (
	// Coordinate validation ranges
	MinLatitude  = -90.0
	MaxLatitude  = 90.0
	MinLongitude = -180.0
	MaxLongitude = 180.0

	// Google Maps URL patterns
	GoogleMapsPattern = "google.com/maps"
)

// ============================================================
// TYPES
// ============================================================

type MessageContent struct {
	T   string `json:"t"`   // Text content containing URLs
	Fwd bool   `json:"fwd"` // Forwarded message
}

type LocationInfo struct {
	Latitude  float64
	Longitude float64
	IsValid   bool
}

// ============================================================
// CHANNEL MESSAGE EVENT
// ============================================================

func (c *MezonClient) setupChannelMessageHandler() {
	c.On("channel_message", func(data interface{}) {
		c.handleChannelMessage(data)
	})
}

func (c *MezonClient) parseChannelMessage(eventData interface{}) (*api.ChannelMessage, error) {
	// Check if eventData is already *api.ChannelMessage
	if msg, ok := eventData.(*api.ChannelMessage); ok {
		return msg, nil
	}

	// Marshal and unmarshal if it's a map
	eventBytes, err := json.Marshal(eventData)
	if err != nil {
		return nil, fmt.Errorf("marshal event failed: %w", err)
	}

	var message api.ChannelMessage
	if err := json.Unmarshal(eventBytes, &message); err != nil {
		return nil, fmt.Errorf("unmarshal event failed: %w", err)
	}

	// Validate message has required data
	if message.MessageId == 0 {
		return nil, fmt.Errorf("invalid message: missing message_id")
	}

	return &message, nil
}

func (c *MezonClient) handleChannelMessage(eventData interface{}) {
	message, err := c.parseChannelMessage(eventData)
	if err != nil {
		log.Printf("❌ Failed to parse channel_message: %v", err)
		return
	}

	c.logChannelMessage(message)

	// Check and handle location messages
	locationInfo, err := c.extractLocationFromMessage(message)
	if err == nil && locationInfo.IsValid && message.Code == int32(models.CodeLocationSend) {
		c.handleLocationMessage(message, locationInfo)
	}
}

func (c *MezonClient) logChannelMessage(msg *api.ChannelMessage) {
	log.Printf("📨 Channel message received")
	log.Printf("   From: %s (%s)", msg.DisplayName, msg.Username)
	log.Printf("   Channel ID: %d", msg.ChannelId)
	log.Printf("   Message ID: %d", msg.MessageId)
	log.Printf("   Code      : %s", strconv.Itoa(int(msg.Code)))
	// Quick check for location link
	if strings.Contains(msg.Content, GoogleMapsPattern) {
		log.Printf("   📍 Contains location link")
	}
}

// ============================================================
// LOCATION PARSING
// ============================================================

// parseGoogleMapsURL extracts coordinates from various Google Maps URL formats
// Supported formats:
//   - https://www.google.com/maps?q=18.701103,105.679654
//   - https://maps.google.com/maps?q=18.701103,105.679654
//   - https://www.google.com/maps/@18.701103,105.679654,14z
func parseGoogleMapsURL(mapURL string) (float64, float64, error) {
	if mapURL == "" {
		return 0, 0, fmt.Errorf("empty URL")
	}

	// Parse URL
	u, err := url.Parse(mapURL)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid URL format: %w", err)
	}

	// Try to extract from query parameter 'q'
	if q := u.Query().Get("q"); q != "" {
		return parseCoordinatesString(q)
	}

	// Try to extract from path (format: /@lat,lon,zoom)
	if strings.Contains(u.Path, "/@") {
		parts := strings.Split(u.Path, "/@")
		if len(parts) >= 2 {
			coordsPart := strings.Split(parts[1], ",")
			if len(coordsPart) >= 2 {
				return parseCoordinatesString(coordsPart[0] + "," + coordsPart[1])
			}
		}
	}

	return 0, 0, fmt.Errorf("no coordinates found in URL")
}

// parseCoordinatesString parses "lat,lon" string format
func parseCoordinatesString(coords string) (float64, float64, error) {
	parts := strings.Split(coords, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid coordinates format: expected 'lat,lon', got '%s'", coords)
	}

	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid latitude '%s': %w", parts[0], err)
	}

	lon, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid longitude '%s': %w", parts[1], err)
	}

	// Validate coordinate ranges
	if err := validateCoordinates(lat, lon); err != nil {
		return 0, 0, err
	}

	return lat, lon, nil
}

// validateCoordinates checks if coordinates are within valid ranges
func validateCoordinates(lat, lon float64) error {
	if lat < MinLatitude || lat > MaxLatitude {
		return fmt.Errorf("latitude %.6f out of range [%.1f, %.1f]", lat, MinLatitude, MaxLatitude)
	}
	if lon < MinLongitude || lon > MaxLongitude {
		return fmt.Errorf("longitude %.6f out of range [%.1f, %.1f]", lon, MinLongitude, MaxLongitude)
	}
	return nil
}

// extractLocationFromMessage extracts and validates location from message content
// Returns LocationInfo with IsValid=true if location is found and valid
func (c *MezonClient) extractLocationFromMessage(msg *api.ChannelMessage) (LocationInfo, error) {
	var result LocationInfo

	// Parse message content
	var content MessageContent
	if err := json.Unmarshal([]byte(msg.Content), &content); err != nil && content.Fwd != true {
		return result, fmt.Errorf("failed to parse content: %w", err)
	}

	// Check if content contains Google Maps URL
	if !strings.Contains(content.T, GoogleMapsPattern) {
		return result, fmt.Errorf("not a Google Maps URL")
	}

	// Extract coordinates
	lat, lon, err := parseGoogleMapsURL(content.T)
	if err != nil {
		return result, fmt.Errorf("failed to parse coordinates: %w", err)
	}

	result.Latitude = lat
	result.Longitude = lon
	result.IsValid = true

	return result, nil
}

func (c *MezonClient) handleLocationMessage(msg *api.ChannelMessage, location LocationInfo) {
	log.Printf("📍 Processing location message from %s", msg.DisplayName)
	log.Printf("   📍 Coordinates: (%.6f, %.6f)", location.Latitude, location.Longitude)

	// Emit event with parsed coordinates
	c.emit("location_message_received", map[string]interface{}{
		"message":      msg,
		"latitude":     location.Latitude,
		"longitude":    location.Longitude,
		"user_id":      msg.SenderId,
		"channel_id":   msg.ChannelId,
		"username":     msg.Username,
		"display_name": msg.DisplayName,
	})

	log.Printf("✅ Location message event emitted")
}

// ============================================================
// EVENT HANDLER SETUP
// ============================================================

func (c *MezonClient) SetupEventHandlers() {
	log.Printf("🎧 Setting up event handlers for MezonClient...")
	c.setupUserChannelAddedHandler()
	c.setupChannelMessageHandler()
	log.Printf("✅ Event handlers setup complete")
}

// ============================================================
// USER CHANNEL ADDED EVENT
// ============================================================

func (c *MezonClient) setupUserChannelAddedHandler() {
	c.On("user_channel_added_event", func(data interface{}) {
		log.Printf("user_channel_added_event")
		c.handleUserChannelAdded(data)
	})
}

func (c *MezonClient) handleUserChannelAdded(eventData interface{}) {
	event, err := c.parseUserChannelAdded(eventData)
	if err != nil {
		log.Printf("❌ Failed to parse user_channel_added_event: %v", err)
		return
	}

	c.logUserChannelAdded(event)

	if !c.shouldAutoJoin(event) {
		log.Printf("ℹ️  Client not in added users, skipping auto-join")
		return
	}

	c.autoJoinChannel(event)
}

func (c *MezonClient) parseUserChannelAdded(eventData interface{}) (*rtapi.UserChannelAdded, error) {
	eventBytes, err := json.Marshal(eventData)
	if err != nil {
		return nil, fmt.Errorf("marshal event failed: %w", err)
	}

	var event rtapi.UserChannelAdded
	if err := json.Unmarshal(eventBytes, &event); err != nil {
		return nil, fmt.Errorf("unmarshal event failed: %w", err)
	}

	return &event, nil
}

func (c *MezonClient) logUserChannelAdded(event *rtapi.UserChannelAdded) {
	log.Printf("📨 Received user_channel_added_event")
	log.Printf("   Clan ID: %d", event.ClanId)
	log.Printf("   Channel ID: %d", event.ChannelDesc.ChannelId)
	log.Printf("   Channel Label: %s", event.ChannelDesc.ChannelLabel)

	channelType := c.getChannelType(event)
	log.Printf("   Channel Type: %d", channelType)
	log.Printf("   Users count: %d", len(event.Users))

	if event.Caller != nil {
		log.Printf("   Caller: %s (%d)", event.Caller.Username, event.Caller.UserId)
	}

	if event.Status != "" {
		log.Printf("   Status: %s", event.Status)
	}
}

func (c *MezonClient) getChannelType(event *rtapi.UserChannelAdded) int {
	channelType := int(event.ChannelDesc.Type)
	if channelType == 0 && event.ChannelDesc.Type != 0 {
		channelType = int(event.ChannelDesc.Type)
	}
	return channelType
}

func (c *MezonClient) shouldAutoJoin(event *rtapi.UserChannelAdded) bool {
	// Check if auto-join is enabled
	c.mu.RLock()
	autoJoinEnabled := c.autoJoinEnabled
	c.mu.RUnlock()

	if !autoJoinEnabled {
		return false
	}

	for _, user := range event.Users {
		if user.UserId == c.ClientID {
			return true
		}
	}
	return false

}

func (c *MezonClient) autoJoinChannel(event *rtapi.UserChannelAdded) {
	log.Printf("✅ Client was added to channel, auto-joining...")

	channelType := c.getChannelType(event)
	err := c.JoinChat(
		event.ClanId,
		event.ChannelDesc.ChannelId,
		channelType,
		event.ChannelDesc.ChannelPrivate == 0,
	)

	if err != nil {
		c.handleAutoJoinError(event, err)
		return
	}

	log.Printf("✅ Successfully auto-joined channel: %d", event.ChannelDesc.ChannelId)
	c.emit("user_channel_joined", event)
}

func (c *MezonClient) handleAutoJoinError(event *rtapi.UserChannelAdded, err error) {
	log.Printf("❌ Failed to auto-join channel: %v", err)
	c.emit("user_channel_added_error", map[string]interface{}{
		"event": event,
		"error": err.Error(),
	})
}

// ============================================================
// JOIN CHAT METHOD
// ============================================================
func (c *MezonClient) JoinChat(clanID int64, channelID int64, channelType int, isPublic bool) error {
	if c.conn == nil {
		return fmt.Errorf("WebSocket connection is nil")
	}

	c.logJoinChat(clanID, channelID, channelType, isPublic)

	// Build Protobuf envelope
	envelope := &rtapi.Envelope{
		Message: &rtapi.Envelope_ChannelJoin{
			ChannelJoin: &rtapi.ChannelJoin{
				ClanId:      clanID,
				ChannelId:   channelID,
				ChannelType: int32(channelType),
				IsPublic:    isPublic,
			},
		},
	}

	// Send using the existing sendMessage function
	if err := c.sendMessage(envelope); err != nil {
		return fmt.Errorf("send join chat message failed: %w", err)
	}

	log.Printf("✅ Join chat request sent successfully")
	return nil
}

// JoinChatWithResponse joins a channel and waits for confirmation
func (c *MezonClient) JoinChatWithResponse(clanID int64, channelID int64, channelType int, isPublic bool, timeout time.Duration) (*rtapi.Envelope, error) {
	if c.conn == nil {
		return nil, fmt.Errorf("WebSocket connection is nil")
	}

	c.logJoinChat(clanID, channelID, channelType, isPublic)

	// Build Protobuf envelope
	envelope := &rtapi.Envelope{
		Message: &rtapi.Envelope_ChannelJoin{
			ChannelJoin: &rtapi.ChannelJoin{
				ClanId:      clanID,
				ChannelId:   channelID,
				ChannelType: int32(channelType),
				IsPublic:    isPublic,
			},
		},
	}

	// Send with response using the existing sendWithResponse function
	response, err := c.sendWithResponse(envelope, timeout)
	if err != nil {
		return nil, fmt.Errorf("send join chat message failed: %w", err)
	}

	log.Printf("✅ Successfully joined channel: %d", channelID)
	return response, nil
}

func (c *MezonClient) logJoinChat(clanID int64, channelID int64, channelType int, isPublic bool) {
	log.Printf("🔗 Joining chat...")
	log.Printf("   Clan ID: %d", clanID)
	log.Printf("   Channel ID: %d", channelID)
	log.Printf("   Channel Type: %d", channelType)
	log.Printf("   Is Public: %v", isPublic)
}
