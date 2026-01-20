package client

import (
	"fmt"
	"log"
	"time"
)

// ============================================================
// RECONNECTION
// ============================================================

func (c *MezonClient) handleDisconnect() {
	c.reconnectMu.Lock()
	if c.isRetrying || c.isHardDisconnect || c.IsClosed() {
		c.reconnectMu.Unlock()
		return
	}
	c.isRetrying = true
	c.reconnectMu.Unlock()

	log.Println("🔄 Starting reconnection process...")

	if err := c.reconnectWithBackoff(); err != nil {
		log.Printf("❌ Reconnection failed: %v", err)
	}

	c.reconnectMu.Lock()
	c.isRetrying = false
	c.reconnectMu.Unlock()
}

func (c *MezonClient) reconnectWithBackoff() error {
	retryInterval := time.Duration(InitialRetryDelay) * time.Second
	maxRetryInterval := time.Duration(MaxRetryDelay) * time.Second
	attempts := 0

	for attempts < MaxRetries {
		if c.IsClosed() {
			return nil
		}

		attempts++
		log.Printf("🔄 Reconnection attempt %d/%d", attempts, MaxRetries)

		// Wait before retry
		select {
		case <-c.ctx.Done():
			return nil
		case <-time.After(retryInterval):
		}

		if err := c.attemptReconnect(); err != nil {
			log.Printf("❌ Reconnection attempt %d failed: %v", attempts, err)
			retryInterval = c.calculateNextRetryInterval(retryInterval, maxRetryInterval)
			continue
		}

		log.Println("✅ Reconnected successfully!")
		c.emit("reconnected", nil)
		return nil
	}

	return fmt.Errorf("max reconnection attempts (%d) reached", MaxRetries)
}

func (c *MezonClient) attemptReconnect() error {
	// Close old connection if exists
	c.connMu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.connMu.Unlock()

	// Try to login again
	return c.Login()
}

func (c *MezonClient) calculateNextRetryInterval(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}
