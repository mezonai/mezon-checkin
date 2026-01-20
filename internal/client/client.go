package client

import (
	"context"
	"fmt"
	mzapi "mezon-checkin-bot/mezon-protobuf/go/api"
	rtapi "mezon-checkin-bot/mezon-protobuf/go/rtapi"
	"mezon-checkin-bot/models"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ============================================================
// CONSTANTS
// ============================================================

const (
	DMClanID          = "0"
	DMChannelType     = 4
	PingInterval      = 10 // seconds
	InitialRetryDelay = 5  // seconds
	MaxRetryDelay     = 60 // seconds
	MaxRetries        = 10 // maximum reconnection attempts
	DefaultTimeout    = 30 // seconds
	MaxLogLength      = 200
	WriteTimeout      = 10 // seconds for WebSocket writes
	ReadTimeout       = 90 // seconds for WebSocket reads (match code 1)
	ShutdownTimeout   = 5  // seconds for graceful shutdown
)

// ============================================================
// MEZON CLIENT - CORE STRUCTURE
// ============================================================

type MezonClient struct {
	config   models.Config
	conn     *websocket.Conn
	session  *mzapi.Session
	ClientID string

	// Thread safety
	mu     sync.RWMutex
	connMu sync.RWMutex // Separate lock for connection operations

	// Event system
	handlers   map[string][]MessageHandler
	handlersMu sync.RWMutex

	// CID management for protobuf responses
	cidHandlers map[string]chan *rtapi.Envelope
	cidMu       sync.RWMutex
	nextCID     int

	// State management
	verbose          bool
	isRetrying       bool
	isHardDisconnect bool
	reconnectMu      sync.Mutex

	// Lifecycle management
	ctx             context.Context
	cancel          context.CancelFunc
	shutdownOnce    sync.Once
	wg              sync.WaitGroup
	autoJoinEnabled bool
}

type MessageHandler func(data interface{})

// ============================================================
// CONSTRUCTOR
// ============================================================

func NewMezonClient(config models.Config) *MezonClient {
	verbose := os.Getenv("VERBOSE") == "true"
	ctx, cancel := context.WithCancel(context.Background())

	client := &MezonClient{
		config:           config,
		handlers:         make(map[string][]MessageHandler),
		cidHandlers:      make(map[string]chan *rtapi.Envelope),
		nextCID:          1,
		verbose:          verbose,
		isHardDisconnect: false,
		ctx:              ctx,
		cancel:           cancel,
		autoJoinEnabled:  true,
	}

	client.SetupEventHandlers()
	return client
}

// ============================================================
// LOGIN & LIFECYCLE
// ============================================================

func (c *MezonClient) Login() error {
	if err := c.Authenticate(); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}
	if err := c.ConnectWebSocket(); err != nil {
		return fmt.Errorf("websocket connection failed: %w", err)
	}
	return nil
}

func (c *MezonClient) Close() error {
	var closeErr error

	c.shutdownOnce.Do(func() {
		// Mark as hard disconnect first
		c.mu.Lock()
		c.isHardDisconnect = true
		c.mu.Unlock()

		// Cancel all operations
		c.cancel()

		// Close WebSocket connection
		c.connMu.Lock()
		if c.conn != nil {
			// Send close frame with timeout
			closeErr = c.conn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
				time.Now().Add(time.Second),
			)
			c.conn.Close()
			c.conn = nil
		}
		c.connMu.Unlock()

		// Wait for goroutines with timeout
		done := make(chan struct{})
		go func() {
			c.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			if c.verbose {
				fmt.Println("✅ All goroutines finished")
			}
		case <-time.After(ShutdownTimeout * time.Second):
			if c.verbose {
				fmt.Println("⚠️  Shutdown timeout, forcing close")
			}
		}
	})

	return closeErr
}

// ============================================================
// EVENT SYSTEM
// ============================================================

func (c *MezonClient) On(event string, handler MessageHandler) {
	c.handlersMu.Lock()
	defer c.handlersMu.Unlock()
	c.handlers[event] = append(c.handlers[event], handler)
}

func (c *MezonClient) emit(event string, data interface{}) {
	c.handlersMu.RLock()
	handlers := c.handlers[event]
	c.handlersMu.RUnlock()

	// Execute handlers concurrently but don't wait
	for _, handler := range handlers {
		h := handler // Capture for goroutine
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("⚠️  Handler panic for event '%s': %v\n", event, r)
				}
			}()
			h(data)
		}()
	}
}

// ============================================================
// CID MANAGEMENT (PROTOBUF)
// ============================================================

func (c *MezonClient) generateCID() string {
	c.cidMu.Lock()
	defer c.cidMu.Unlock()
	cid := fmt.Sprintf("%d", c.nextCID)
	c.nextCID++
	return cid
}

func (c *MezonClient) resolveCID(cid string, envelope *rtapi.Envelope) {
	c.cidMu.RLock()
	ch, exists := c.cidHandlers[cid]
	c.cidMu.RUnlock()

	if !exists {
		if c.verbose {
			fmt.Printf("⚠️  No handler found for CID=%s\n", cid)
		}
		return
	}

	select {
	case ch <- envelope:
		if c.verbose {
			fmt.Printf("✅ Response delivered to CID=%s\n", cid)
		}
	case <-time.After(100 * time.Millisecond):
		fmt.Printf("⚠️  Response channel timeout for CID=%s\n", cid)
	}
}

// ============================================================
// STATE CHECKS
// ============================================================

func (c *MezonClient) IsConnected() bool {
	c.connMu.RLock()
	defer c.connMu.RUnlock()

	c.mu.RLock()
	hardDisconnect := c.isHardDisconnect
	c.mu.RUnlock()

	return c.conn != nil && !hardDisconnect
}

func (c *MezonClient) IsClosed() bool {
	select {
	case <-c.ctx.Done():
		return true
	default:
		c.mu.RLock()
		defer c.mu.RUnlock()
		return c.isHardDisconnect
	}
}

// ============================================================
// AUTO JOIN CONTROL
// ============================================================

func (c *MezonClient) EnableAutoJoin() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.autoJoinEnabled = true
}

func (c *MezonClient) DisableAutoJoin() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.autoJoinEnabled = false
}

func (c *MezonClient) IsAutoJoinEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.autoJoinEnabled
}

// ============================================================
// UTILITY METHODS
// ============================================================

func (c *MezonClient) GetVerbose() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.verbose
}

func (c *MezonClient) SetVerbose(verbose bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.verbose = verbose
}

func (c *MezonClient) GetSession() *mzapi.Session {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.session
}

func (c *MezonClient) GetConfig() models.Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config
}
