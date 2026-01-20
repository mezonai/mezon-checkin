package client

import (
	"fmt"
	"io"
	"log"
	"mezon-checkin-bot/internal/utils"
	rtapi "mezon-checkin-bot/mezon-protobuf/go/rtapi"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// ============================================================
// WEBSOCKET CONNECTION
// ============================================================

func (c *MezonClient) ConnectWebSocket() error {
	if c.session == nil || c.session.Token == "" {
		return fmt.Errorf("no session available, authenticate first")
	}

	wsURL := c.buildWebSocketURL()
	log.Printf("🔌 Connecting to Mezon WebSocket...")

	conn, wsResp, err := c.dialWebSocket(wsURL)
	if err != nil {
		c.logWebSocketError(wsResp, err)
		return fmt.Errorf("websocket connection failed: %w", err)
	}

	if wsResp.StatusCode != http.StatusSwitchingProtocols {
		return fmt.Errorf("unexpected websocket status: %d", wsResp.StatusCode)
	}

	c.connMu.Lock()
	c.conn = conn

	// Set read/write deadlines
	c.conn.SetReadDeadline(time.Now().Add(ReadTimeout * time.Second))
	c.conn.SetWriteDeadline(time.Now().Add(WriteTimeout * time.Second))

	// Set ping handler
	c.conn.SetPingHandler(func(appData string) error {
		c.connMu.Lock()
		defer c.connMu.Unlock()
		if c.conn != nil {
			return c.conn.WriteMessage(websocket.PongMessage, []byte(appData))
		}
		return nil
	})

	c.connMu.Unlock()

	c.logConnectionSuccess()

	// Start goroutines with waitgroup
	c.wg.Add(2)
	go c.handleMessages()
	go c.pingPong()

	return nil
}

// ============================================================
// WEBSOCKET URL BUILDER
// ============================================================

func (c *MezonClient) buildWebSocketURL() string {
	wsScheme := c.getWebSocketScheme()
	createStatus := true

	// Add format=protobuf parameter
	if c.isDefaultPort() {
		return fmt.Sprintf("%s%s/ws?lang=en&status=%s&token=%s&format=protobuf",
			wsScheme,
			c.config.SocketHost,
			utils.EncodeURIComponent(fmt.Sprintf("%t", createStatus)),
			utils.EncodeURIComponent(c.session.Token))
	}

	return fmt.Sprintf("%s%s:%s/ws?lang=en&status=%s&token=%s&format=protobuf",
		wsScheme,
		c.config.SocketHost,
		c.config.SocketPort,
		utils.EncodeURIComponent(fmt.Sprintf("%t", createStatus)),
		utils.EncodeURIComponent(c.session.Token))
}

func (c *MezonClient) getWebSocketScheme() string {
	if c.config.SocketUseSSL {
		return "wss://"
	}
	return "ws://"
}

func (c *MezonClient) isDefaultPort() bool {
	if c.config.UseSSL && c.config.Port == "443" {
		return true
	}
	if !c.config.UseSSL && c.config.Port == "80" {
		return true
	}
	return false
}

func (c *MezonClient) dialWebSocket(wsURL string) (*websocket.Conn, *http.Response, error) {
	headers := map[string][]string{
		"User-Agent": {"Mezon-Go-Bot/1.0"},
	}

	dialer := &websocket.Dialer{
		HandshakeTimeout: DefaultTimeout * time.Second,
	}

	return dialer.Dial(wsURL, headers)
}

func (c *MezonClient) logWebSocketError(wsResp *http.Response, err error) {
	if wsResp != nil {
		log.Printf("❌ HTTP Status: %d", wsResp.StatusCode)
		body, _ := io.ReadAll(wsResp.Body)
		if len(body) > 0 {
			log.Printf("❌ Response: %s", string(body))
		}
	}

	if err != nil {
		log.Printf("❌ WebSocket error: %v", err)
	}
}

func (c *MezonClient) logConnectionSuccess() {
	log.Println("✅ Connected to Mezon WebSocket")
	log.Printf("   Client ID: %s", c.ClientID)
}

// ============================================================
// MESSAGE HANDLING (PROTOBUF BINARY)
// ============================================================

func (c *MezonClient) handleMessages() {
	defer c.wg.Done()
	defer log.Println("🔌 Message handler stopped")

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		c.connMu.RLock()
		conn := c.conn
		c.connMu.RUnlock()

		if conn == nil {
			return
		}

		// Set read deadline
		conn.SetReadDeadline(time.Now().Add(ReadTimeout * time.Second))

		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if c.IsClosed() {
				return
			}

			log.Printf("❌ WebSocket read error: %v", err)
			if !c.isHardDisconnect {
				go c.handleDisconnect()
			}
			return
		}

		// Only process binary messages (Protobuf)
		switch messageType {
		case websocket.BinaryMessage:
			c.processProtobufMessage(message)
		case websocket.TextMessage:
			log.Printf("📄 Text message (unexpected): %s", string(message))
		}
	}
}

func (c *MezonClient) processProtobufMessage(message []byte) {
	var envelope rtapi.Envelope
	if err := proto.Unmarshal(message, &envelope); err != nil {
		log.Printf("⚠️ Protobuf decode error: %v", err)
		return
	}

	// Handle CID response
	if envelope.Cid != "" {
		c.resolveCID(envelope.Cid, &envelope)
		return
	}

	// Handle events from server
	c.handleEnvelopeMessage(&envelope)
}

func (c *MezonClient) handleEnvelopeMessage(envelope *rtapi.Envelope) {
	if c.verbose {
		// Use proto package to format the message
		log.Printf("📥 Received message: %v", envelope.Message)
	}
	switch envelope.Message.(type) {
	case *rtapi.Envelope_Pong:
		if c.verbose {
			log.Printf("💓 Pong received")
		}
	case *rtapi.Envelope_UserChannelAddedEvent:
		userChannelAdded := envelope.GetUserChannelAddedEvent()
		log.Printf("👥 UserChannelAdded event received")
		c.emit("user_channel_added_event", userChannelAdded)
	case *rtapi.Envelope_Error:
		log.Printf("❌ Server Error: code=%d, message=%s",
			envelope.GetError().Code, envelope.GetError().Message)

	case *rtapi.Envelope_ClanJoin:
		log.Printf("✅ ClanJoin confirmation received")

	case *rtapi.Envelope_ChannelJoin:
		log.Printf("✅ ChannelJoin confirmation received")

	case *rtapi.Envelope_Channel:
		log.Printf("✅ Channel info received: %s", envelope.GetChannel().Id)

	case *rtapi.Envelope_ChannelMessageAck:
		log.Printf("✅ MessageAck received: %s", envelope.GetChannelMessageAck().MessageId)

	case *rtapi.Envelope_ChannelMessage:
		channelMsg := envelope.GetChannelMessage()
		log.Printf("📬 ChannelMessage received from %s", channelMsg.Username)
		c.emit("channel_message", channelMsg)

	case *rtapi.Envelope_WebrtcSignalingFwd:
		webrtcMsg := envelope.GetWebrtcSignalingFwd()
		log.Printf("📞 WebRTC signal received")
		c.emit("webrtc_signaling_fwd", webrtcMsg)
	}
}

// ============================================================
// SEND MESSAGE (PROTOBUF BINARY)
// ============================================================

// sendMessage - Gửi message KHÔNG chờ response
func (c *MezonClient) sendMessage(envelope *rtapi.Envelope) error {
	return c.sendMessageWithTimeout(envelope, WriteTimeout*time.Second)
}

func (c *MezonClient) sendMessageWithTimeout(envelope *rtapi.Envelope, timeout time.Duration) error {
	// KHÔNG set CID cho message thông thường
	envelope.Cid = ""

	// Marshal envelope thành binary protobuf
	data, err := proto.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal protobuf failed: %w", err)
	}

	c.connMu.RLock()
	conn := c.conn
	c.connMu.RUnlock()

	if conn == nil {
		return fmt.Errorf("websocket connection is nil")
	}

	// Set write deadline
	if err := conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
		return fmt.Errorf("set write deadline failed: %w", err)
	}

	// Send binary message
	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return fmt.Errorf("write message failed: %w", err)
	}

	if c.verbose {
		log.Printf("📤 Sent message (%d bytes protobuf)", len(data))
	}

	return nil
}

// sendWithResponse - Gửi message VÀ chờ response
func (c *MezonClient) sendWithResponse(envelope *rtapi.Envelope, timeout time.Duration) (*rtapi.Envelope, error) {
	c.connMu.RLock()
	conn := c.conn
	c.connMu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("socket connection has not been established yet")
	}

	// Generate CID
	cid := c.generateCID()
	envelope.Cid = cid

	// Tạo channel để nhận response
	responseChan := make(chan *rtapi.Envelope, 1)
	c.cidMu.Lock()
	c.cidHandlers[cid] = responseChan
	c.cidMu.Unlock()

	// Cleanup
	defer func() {
		c.cidMu.Lock()
		delete(c.cidHandlers, cid)
		c.cidMu.Unlock()
		close(responseChan)
	}()

	// Marshal envelope thành binary protobuf
	data, err := proto.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal protobuf: %w", err)
	}

	if c.verbose {
		log.Printf("📤 Sending CID=%s (%d bytes protobuf)", cid, len(data))
	}

	// Set write deadline
	conn.SetWriteDeadline(time.Now().Add(timeout))

	// Send binary qua WebSocket
	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return nil, fmt.Errorf("write message: %w", err)
	}

	// Đợi response hoặc timeout
	select {
	case response := <-responseChan:
		if response.GetError() != nil {
			return response, fmt.Errorf("server error: code=%d, message=%s",
				response.GetError().Code, response.GetError().Message)
		}
		return response, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for response")
	case <-c.ctx.Done():
		return nil, fmt.Errorf("context cancelled")
	}
}

// ============================================================
// PING/PONG
// ============================================================

func (c *MezonClient) pingPong() {
	defer c.wg.Done()
	defer log.Println("🏓 Ping/pong stopped")

	// Wait before starting ping
	time.Sleep(3 * time.Second)

	ticker := time.NewTicker(PingInterval * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if c.IsClosed() {
				return
			}

			if err := c.sendPing(); err != nil {
				log.Printf("❌ Ping failed: %v", err)
				if !c.isHardDisconnect {
					go c.handleDisconnect()
				}
				return
			}
		}
	}
}

func (c *MezonClient) sendPing() error {
	envelope := &rtapi.Envelope{
		Message: &rtapi.Envelope_Ping{
			Ping: &rtapi.Ping{},
		},
	}
	return c.sendMessage(envelope)
}

// ============================================================
// WEBRTC SIGNAL (PROTOBUF)
// ============================================================

func (c *MezonClient) SendWebRTCSignal(receiverID, callerID, channelID string, dataType int, jsonData string) error {
	if c.IsClosed() {
		return fmt.Errorf("client is closed")
	}

	envelope := &rtapi.Envelope{
		Message: &rtapi.Envelope_WebrtcSignalingFwd{
			WebrtcSignalingFwd: &rtapi.WebrtcSignalingFwd{
				ReceiverId: receiverID,
				CallerId:   callerID,
				ChannelId:  channelID,
				DataType:   int32(dataType),
				JsonData:   jsonData,
			},
		},
	}

	return c.sendMessage(envelope)
}
