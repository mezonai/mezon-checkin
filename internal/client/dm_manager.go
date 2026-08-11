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
	client         *MezonClient
	dmChannels     map[int64]int64 // userID -> DM channelID
	joinedChannels map[int64]bool  // channelID -> joined on socket
	mu             sync.RWMutex
	joinedMu       sync.RWMutex
	clanID         int64
	isDMReady      bool
	readyMu        sync.RWMutex
}

// ============================================================
// CONSTRUCTOR
// ============================================================

func NewDMManager(client *MezonClient) *DMManager {
	dm := &DMManager{
		client:         client,
		dmChannels:     make(map[int64]int64),
		joinedChannels: make(map[int64]bool),
		clanID:         DMClanID,
		isDMReady:      false,
	}
	err := dm.ensureDMReady()
	if err != nil {
		log.Printf("⚠️  DM Manager init error: %s", err)
	}
	client.On("reconnected", func(data interface{}) {
		dm.readyMu.Lock()
		dm.isDMReady = false
		dm.readyMu.Unlock()

		dm.joinedMu.Lock()
		dm.joinedChannels = make(map[int64]bool)
		dm.joinedMu.Unlock()

		dm.mu.Lock()
		dm.dmChannels = make(map[int64]int64)
		dm.mu.Unlock()

		if err := dm.ensureDMReady(); err != nil {
			log.Printf("⚠️  DM Manager reconnect error: %s", err)
		}
	})
	log.Printf("✅ DM Manager created (lazy init mode)")
	return dm
}

// ============================================================
// INITIALIZATION
// ============================================================

func (dm *DMManager) ensureDMReady() error {
	dm.readyMu.RLock()
	if dm.isDMReady {
		dm.readyMu.RUnlock()
		return nil
	}
	dm.readyMu.RUnlock()

	dm.readyMu.Lock()
	defer dm.readyMu.Unlock()

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

	go func() {
		if err := dm.loadExistingDMChannels(); err != nil {
			log.Printf("   ⚠️  Failed to load existing DM channels: %v", err)
		}
	}()

	return nil
}

func (dm *DMManager) loadExistingDMChannels() error {
	channels, err := dm.client.ListDMChannels()
	if err != nil {
		return err
	}

	dm.mu.Lock()
	defer dm.mu.Unlock()

	for _, ch := range channels {
		for _, userID := range ch.GetUserIds() {
			if userID != dm.client.ClientID {
				dm.dmChannels[userID] = ch.GetChannelId()
			}
		}
	}

	log.Printf("   📋 Loaded %d existing DM channel(s)", len(dm.dmChannels))
	return nil
}

func (dm *DMManager) joinDMClan() error {
	return dm.joinClanInternal(DMClanID)
}

// GetOrCreateDMChannel resolves the DM channel for a user via REST API.
func (dm *DMManager) GetOrCreateDMChannel(userID int64) (int64, error) {
	dm.mu.RLock()
	if channelID, ok := dm.dmChannels[userID]; ok {
		dm.mu.RUnlock()
		return channelID, nil
	}
	dm.mu.RUnlock()

	dm.mu.Lock()
	defer dm.mu.Unlock()

	if channelID, ok := dm.dmChannels[userID]; ok {
		return channelID, nil
	}

	log.Printf("🔗 Creating/resolving DM channel for user %d", userID)
	desc, err := dm.client.CreateDMChannel(userID)
	if err != nil {
		return 0, err
	}

	dm.dmChannels[userID] = desc.ChannelId
	log.Printf("✅ DM channel for user %d: %d", userID, desc.ChannelId)
	return desc.ChannelId, nil
}

func (dm *DMManager) ensureChannelJoined(channelID int64) error {
	dm.joinedMu.RLock()
	if dm.joinedChannels[channelID] {
		dm.joinedMu.RUnlock()
		return nil
	}
	dm.joinedMu.RUnlock()

	log.Printf("🔗 Joining DM channel %d before send...", channelID)
	_, err := dm.client.JoinChatWithResponse(DMClanID, channelID, DMChannelType, false, 10*time.Second)
	if err != nil {
		return fmt.Errorf("join DM channel %d: %w", channelID, err)
	}

	dm.joinedMu.Lock()
	dm.joinedChannels[channelID] = true
	dm.joinedMu.Unlock()

	log.Printf("✅ Joined DM channel %d", channelID)
	return nil
}

// ============================================================
// CLAN OPERATIONS - PROTOBUF VERSION
// ============================================================

func (dm *DMManager) joinClanInternal(clanID int64) error {
	if !dm.client.IsConnected() {
		return fmt.Errorf("WebSocket connection is nil")
	}

	log.Printf("🔗 Joining clan: %d", clanID)

	envelope := &rtapi.Envelope{
		Message: &rtapi.Envelope_ClanJoin{
			ClanJoin: &rtapi.ClanJoin{
				ClanId: clanID,
			},
		},
	}

	timeout := 10 * time.Second
	response, err := dm.client.sendWithResponse(envelope, timeout)
	if err != nil {
		return fmt.Errorf("join clan failed: %w", err)
	}

	if response.GetError() != nil {
		return fmt.Errorf("server error: code=%d, message=%s",
			response.GetError().Code, response.GetError().Message)
	}

	log.Printf("✅ Joined clan: %d", clanID)
	return nil
}
