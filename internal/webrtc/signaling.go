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

	"github.com/pion/webrtc/v4"
)

// ============================================================
// MAIN SIGNAL HANDLER
// ============================================================

func (w *WebRTCManager) HandleSignal(userID int64, signal *rtapi.WebrtcSignalingFwd) error {
	if signal == nil {
		return fmt.Errorf("signal cannot be nil")
	}

	log.Println("\n" + strings.Repeat("=", 60))
	log.Printf("📡 WebRTC Signal (Type: %d)", signal.DataType)
	log.Printf("   UserID: %d", userID)
	log.Printf("   CallerID: %d", signal.CallerId)
	log.Printf("   ChannelID: %d", signal.ChannelId)
	log.Println(strings.Repeat("=", 60))

	switch signal.DataType {
	case models.WebrtcSDPCallRequest:
		return w.handleCallRequest(userID, signal)
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
// CALL REQUEST HANDLING (type 50)
// ============================================================

// handleCallRequest xử lý tín hiệu incoming call từ user (type 50).
// Bot phải phản hồi bằng WebrtcSDPInit (type 0) để báo hiệu sẵn sàng
// nhận cuộc gọi. Sau khi nhận được phản hồi này, user client sẽ tiếp
// tục gửi SDP offer (type 1).
func (w *WebRTCManager) handleCallRequest(userID int64, signal *rtapi.WebrtcSignalingFwd) error {
	log.Printf("📲 Incoming call request from user %d — sending init response", userID)

	if err := w.client.SendWebRTCSignal(
		userID,
		w.client.ClientID,
		signal.ChannelId,
		models.WebrtcSDPInit,
		"",
	); err != nil {
		return fmt.Errorf("failed to send call init response: %w", err)
	}

	log.Printf("✅ Call init sent to user %d — waiting for offer...", userID)
	return nil
}

// ============================================================
// OFFER HANDLING
// ============================================================

func (w *WebRTCManager) handleOffer(userID int64, signal *rtapi.WebrtcSignalingFwd) error {
	log.Println("📝 Processing offer...")
	log.Printf("   UserID: %d", userID)
	log.Printf("   ChannelID: %d", signal.ChannelId)

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

	// If a connection already exists for this user, decide how to handle it.
	w.mu.RLock()
	existing, exists := w.connections[userID]
	w.mu.RUnlock()

	if exists {
		pcState := existing.pc.ConnectionState()
		switch pcState {
		case webrtc.PeerConnectionStateConnected:
			// Live session: re-offer (e.g. user toggled camera). Renegotiate on
			// the existing PC — tearing it down would cause InvalidModificationError.
			log.Printf("🔄 Re-offer from user %d (pc state: connected) — renegotiating on existing connection", userID)
			return w.handleReoffer(userID, existing, sdp, signal)

		case webrtc.PeerConnectionStateNew, webrtc.PeerConnectionStateConnecting:
			// Another goroutine is still handling the first offer for this user.
			// This is a duplicate/retry from the client — drop it to avoid the
			// race where goroutine 2 destroys goroutine 1's freshly-created PC.
			log.Printf("⚠️  Duplicate offer from user %d during setup (pc state: %s) — ignoring", userID, pcState.String())
			return nil

		default:
			// Stale/failed/closed connection — clean it up before creating a fresh one.
			log.Printf("🧹 Replacing stale connection for user %d (pc state: %s)", userID, pcState.String())
			w.cleanupConnection(userID)
		}
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

	log.Printf("✅ Connection created for user %d", userID)

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

	// FIX: Patch SDP TRƯỚC khi SetLocalDescription để local desc và
	// SDP gửi cho user luôn khớp nhau (cùng ufrag, cùng fingerprint).
	patchedSDP := utils.PatchSDPForQuality(answer.SDP, 2500, 1500, 3000)
	patchedAnswer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  patchedSDP,
	}

	// FIX: Dùng GatheringCompletePromise thay vì time.Sleep cứng.
	gatherComplete := webrtc.GatheringCompletePromise(pc)

	if err := pc.SetLocalDescription(patchedAnswer); err != nil {
		w.cleanupConnection(userID)
		return fmt.Errorf("failed to set local description: %w", err)
	}

	// Chờ ICE gathering xong thật sự
	log.Println("⏳ Waiting for ICE gathering to complete...")
	<-gatherComplete
	log.Println("✅ ICE gathering complete")

	// Compress answer
	answerJSON, _ := json.Marshal(patchedAnswer)
	compressedAnswer := utils.CompressGzip(string(answerJSON))

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

	// Flush pending ICE candidates
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

// handleReoffer handles a subsequent SDP offer on an already-connected peer
// connection (e.g. the user toggled their camera). Instead of tearing down the
// existing connection, we renegotiate in-place.
func (w *WebRTCManager) handleReoffer(userID int64, state *connectionState, sdp string, signal *rtapi.WebrtcSignalingFwd) error {
	pc := state.pc

	if err := pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdp,
	}); err != nil {
		return fmt.Errorf("reoffer: set remote description: %w", err)
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return fmt.Errorf("reoffer: create answer: %w", err)
	}

	// NOTE: Do NOT patch the SDP here. Pion compares the SDP passed to
	// SetLocalDescription against the one it generated internally via
	// CreateAnswer. Any external modification triggers InvalidModificationError.
	// Quality params were already negotiated on the initial offer/answer.
	if err := pc.SetLocalDescription(answer); err != nil {
		return fmt.Errorf("reoffer: set local description: %w", err)
	}

	answerJSON, _ := json.Marshal(answer)
	compressedAnswer := utils.CompressGzip(string(answerJSON))

	if err := w.client.SendWebRTCSignal(
		userID,
		w.client.ClientID,
		signal.ChannelId,
		models.WebrtcSDPAnswer,
		compressedAnswer,
	); err != nil {
		return fmt.Errorf("reoffer: send answer: %w", err)
	}

	log.Printf("✅ Re-offer handled for user %d (camera/media change accepted)", userID)
	return nil
}

// ============================================================
// ICE CANDIDATE HANDLING
// ============================================================

func (w *WebRTCManager) handleICECandidate(userID int64, signal *rtapi.WebrtcSignalingFwd) error {
	var candidate webrtc.ICECandidateInit
	if err := json.Unmarshal([]byte(signal.JsonData), &candidate); err != nil {
		return fmt.Errorf("invalid candidate: %w", err)
	}

	w.mu.RLock()
	state, exists := w.connections[userID]
	w.mu.RUnlock()

	if !exists {
		log.Printf("⚠️  Connection not found for user %d", userID)
		return fmt.Errorf("connection not found")
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	// Queue nếu chưa sẵn sàng
	if !state.iceReady {
		state.pendingICE = append(state.pendingICE, candidate)
		log.Printf("📦 Queued ICE (total: %d)", len(state.pendingICE))
		return nil
	}

	// Add ngay lập tức
	if err := state.pc.AddICECandidate(candidate); err != nil {
		log.Printf("⚠️  Failed to add ICE: %v", err)
		return err
	}

	sdpMid := "unknown"
	if candidate.SDPMid != nil {
		sdpMid = *candidate.SDPMid
	}
	log.Printf("✅ Added ICE (sdpMid: %s)", sdpMid)
	return nil
}
