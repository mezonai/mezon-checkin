package client

import (
	"fmt"
	"log"
	"sync"
	"time"

	rtapi "mezon-checkin-bot/mezon-protobuf/go/rtapi"
)

// ============================================================
// DM MANAGER - CORE STRUCTURE
// ============================================================

type DMManager struct {
	client     *MezonClient
	dmChannels map[string]string // userID -> channelID
	mu         sync.RWMutex
	clanID     int64
	isDMReady  bool
	readyMu    sync.RWMutex
}

// ============================================================
// CONSTRUCTOR
// ============================================================

func NewDMManager(client *MezonClient) *DMManager {
	dm := &DMManager{
		client:     client,
		dmChannels: make(map[string]string),
		clanID:     DMClanID,
		isDMReady:  false,
	}
	err := dm.ensureDMReady()
	if err != nil {
		log.Printf(" DM Manager Error %s", err)
	}
	client.On("reconnected", func(data interface{}) {
		dm.isDMReady = false
		err := dm.ensureDMReady()
		if err != nil {
			log.Printf(" DM Manager Error %s", err)
		}
	})
	log.Printf("✅ DM Manager created (lazy init mode)")
	return dm
}

// ============================================================
// INITIALIZATION
// ============================================================

func (dm *DMManager) ensureDMReady() error {
	// Fast path: already ready
	dm.readyMu.RLock()
	if dm.isDMReady {
		dm.readyMu.RUnlock()
		return nil
	}
	dm.readyMu.RUnlock()

	// Slow path: need to initialize
	dm.readyMu.Lock()
	defer dm.readyMu.Unlock()

	// Double-check after acquiring lock
	if dm.isDMReady {
		return nil
	}

	if !dm.client.IsConnected() {
		return fmt.Errorf("WebSocket connection not ready, call Login() first")
	}

	log.Printf("🔗 Initializing DM clan (lazy init)")
	if err := dm.joinDMClan(); err != nil {
		return fmt.Errorf("failed to join DM clan: %w", err)
	}

	dm.isDMReady = true
	log.Printf("✅ DM clan initialized successfully")
	return nil
}

func (dm *DMManager) joinDMClan() error {
	return dm.joinClanInternal(DMClanID)
}

// ============================================================
// CLAN OPERATIONS - PROTOBUF VERSION
// ============================================================

func (dm *DMManager) joinClanInternal(clanID int64) error {
	if !dm.client.IsConnected() {
		return fmt.Errorf("WebSocket connection is nil")
	}

	log.Printf("🔗 Joining clan: %d", clanID)

	// ⚡ SỬ DỤNG PROTOBUF thay vì JSON
	envelope := &rtapi.Envelope{
		Message: &rtapi.Envelope_ClanJoin{
			ClanJoin: &rtapi.ClanJoin{
				ClanId: clanID,
			},
		},
	}

	// ⚡ SỬ DỤNG sendWithResponse thay vì truy cập trực tiếp conn
	timeout := 10 * time.Second
	response, err := dm.client.sendWithResponse(envelope, timeout)
	if err != nil {
		return fmt.Errorf("join clan failed: %w", err)
	}

	// Check response
	if response.GetError() != nil {
		return fmt.Errorf("server error: code=%d, message=%s",
			response.GetError().Code, response.GetError().Message)
	}

	log.Printf("✅ Joined clan: %d", clanID)
	return nil
}
