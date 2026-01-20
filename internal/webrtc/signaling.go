package webrtc

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"mezon-checkin-bot/internal/utils"
	"mezon-checkin-bot/mezon-protobuf/go/rtapi"
	"mezon-checkin-bot/models"
	"strings"
	"time"

	"github.com/pion/webrtc/v4"
)

// ============================================================
// MAIN SIGNAL HANDLER
// ============================================================

func (w *WebRTCManager) HandleSignal(userID string, signal *rtapi.WebrtcSignalingFwd) error {
	if signal == nil {
		return fmt.Errorf("signal cannot be nil")
	}

	log.Println("\n" + strings.Repeat("=", 60))
	log.Printf("📡 WebRTC Signal (Type: %d)", signal.DataType)
	log.Printf("   UserID: %s", userID)
	log.Printf("   CallerID: %s", signal.CallerId)
	log.Printf("   ChannelID: %s", signal.ChannelId)
	log.Println(strings.Repeat("=", 60))

	switch signal.DataType {
	case models.WebrtcSDPOffer:
		return w.handleOffer(userID, signal)
	case models.WebrtcICECandidate:
		return w.handleICECandidate(userID, signal)
	case models.WebrtcSDPStatusRemoteMedia:
		return nil
	case models.WebrtcSDPQuit:
		log.Printf("👋 Call ended by user")
		w.cleanupConnection(userID)
		return nil
	default:
		log.Printf("⚠️  Unknown signal type: %d", signal.DataType)
		return nil
	}
}

// ============================================================
// OFFER HANDLING
// ============================================================

func (w *WebRTCManager) handleOffer(userID string, signal *rtapi.WebrtcSignalingFwd) error {
	log.Println("📝 Processing offer...")
	log.Printf("   UserID: %s", userID)
	log.Printf("   ChannelID: %s", signal.ChannelId)

	// Decompress if needed
	offerData := signal.JsonData
	if strings.HasPrefix(offerData, "H4sI") {
		decompressed, err := utils.DecompressGzip(offerData)
		if err != nil {
			return fmt.Errorf("decompress failed: %w", err)
		}
		offerData = decompressed
	}

	// Parse offer
	var offer map[string]interface{}
	if err := json.Unmarshal([]byte(offerData), &offer); err != nil {
		offer = map[string]interface{}{
			"type": "offer",
			"sdp":  offerData,
		}
	}

	sdp, ok := offer["sdp"].(string)
	if !ok {
		return fmt.Errorf("invalid offer: missing sdp")
	}

	// Create peer connection
	pc, err := w.createPeerConnection()
	if err != nil {
		return fmt.Errorf("failed to create peer connection: %w", err)
	}

	// Setup context
	ctx, cancel := context.WithCancel(context.Background())
	state := &connectionState{
		pc:         pc,
		channelID:  signal.ChannelId,
		audioStop:  make(chan struct{}),
		cancelFunc: cancel,
		pendingICE: make([]webrtc.ICECandidateInit, 0, 10),
		iceReady:   false,
	}

	// Register connection
	w.mu.Lock()
	w.connections[userID] = state
	w.mu.Unlock()

	log.Printf("✅ Connection created for user %s", userID)

	// Setup handlers
	w.setupPeerConnectionHandlers(userID, pc, ctx)

	// Set remote description
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdp,
	}); err != nil {
		w.cleanupConnection(userID)
		return fmt.Errorf("failed to set remote description: %w", err)
	}

	// Setup audio
	if w.audioConfig.Enabled {
		if err := w.setupAudioTrack(userID, pc); err != nil {
			log.Printf("⚠️  Failed to setup audio: %v", err)
		}
	}

	// Create answer
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		w.cleanupConnection(userID)
		return fmt.Errorf("failed to create answer: %w", err)
	}

	// Set local description
	if err := pc.SetLocalDescription(answer); err != nil {
		w.cleanupConnection(userID)
		return fmt.Errorf("failed to set local description: %w", err)
	}

	// Patch SDP
	patchedSDP := utils.PatchSDPForQuality(answer.SDP, 2500, 1500, 3000)
	patchedAnswer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  patchedSDP,
	}

	// Compress answer
	answerJSON, _ := json.Marshal(patchedAnswer)
	compressedAnswer := utils.CompressGzip(string(answerJSON))

	// Wait for ICE gathering
	time.Sleep(500 * time.Millisecond)

	// Send answer
	if err := w.client.SendWebRTCSignal(
		userID,
		w.client.ClientID,
		signal.ChannelId,
		models.WebrtcSDPAnswer,
		compressedAnswer,
	); err != nil {
		w.cleanupConnection(userID)
		return fmt.Errorf("failed to send answer: %w", err)
	}

	// Process pending ICE candidates
	state.mu.Lock()
	state.iceReady = true
	pendingCandidates := state.pendingICE
	state.pendingICE = nil
	state.mu.Unlock()

	if len(pendingCandidates) > 0 {
		log.Printf("📦 Processing %d pending ICE candidates...", len(pendingCandidates))
		for i, candidate := range pendingCandidates {
			if err := pc.AddICECandidate(candidate); err != nil {
				log.Printf("⚠️  Failed to add pending ICE %d: %v", i+1, err)
			} else {
				log.Printf("✅ Added pending ICE %d/%d", i+1, len(pendingCandidates))
			}
		}
	}

	log.Println("✅ Answer sent!")
	log.Println(strings.Repeat("=", 60))

	return nil
}

// ============================================================
// ICE CANDIDATE HANDLING
// ============================================================

func (w *WebRTCManager) handleICECandidate(userID string, signal *rtapi.WebrtcSignalingFwd) error {
	var candidate webrtc.ICECandidateInit
	if err := json.Unmarshal([]byte(signal.JsonData), &candidate); err != nil {
		return fmt.Errorf("invalid candidate: %w", err)
	}

	w.mu.RLock()
	state, exists := w.connections[userID]
	w.mu.RUnlock()

	if !exists {
		log.Printf("⚠️  Connection not found for user %s", userID)
		return fmt.Errorf("connection not found")
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	// Queue if not ready
	if !state.iceReady {
		state.pendingICE = append(state.pendingICE, candidate)
		log.Printf("📦 Queued ICE (total: %d)", len(state.pendingICE))
		return nil
	}

	// Add immediately
	if err := state.pc.AddICECandidate(candidate); err != nil {
		log.Printf("⚠️  Failed to add ICE: %v", err)
		return err
	}

	// Null-safe logging
	sdpMid := "unknown"
	if candidate.SDPMid != nil {
		sdpMid = *candidate.SDPMid
	}
	log.Printf("✅ Added ICE (sdpMid: %s)", sdpMid)
	return nil
}
