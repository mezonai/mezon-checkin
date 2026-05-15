package webrtc

import (
	"context"
	"fmt"
	"log"
	"mezon-checkin-bot/internal/audio"
	"strings"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

// ============================================================
// PEER CONNECTION CREATION
// ============================================================

func (w *WebRTCManager) createPeerConnection() (*webrtc.PeerConnection, error) {
	mediaEngine := &webrtc.MediaEngine{}

	// Register VP8 video codec
	if err := mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeVP8,
			ClockRate: 90000,
			RTCPFeedback: []webrtc.RTCPFeedback{
				{Type: "goog-remb"},
				{Type: "ccm", Parameter: "fir"},
				{Type: "nack"},
				{Type: "nack", Parameter: "pli"},
			},
		},
		PayloadType: 96,
	}, webrtc.RTPCodecTypeVideo); err != nil {
		return nil, fmt.Errorf("failed to register VP8: %w", err)
	}

	// Register Opus audio codec
	if err := mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeOpus,
			ClockRate:   48000,
			Channels:    2,
			SDPFmtpLine: "minptime=10;useinbandfec=1",
		},
		PayloadType: 111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return nil, fmt.Errorf("failed to register Opus: %w", err)
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
			{URLs: []string{"stun:stun1.l.google.com:19302"}},
			{
				URLs:       []string{"turn:relay.mezon.vn:5349"},
				Username:   "turnmezon",
				Credential: "QuTs4zUEcbylWemXL7MK",
			},
		},
	}

	return api.NewPeerConnection(config)
}

// ============================================================
// PEER CONNECTION HANDLERS
// ============================================================

func (w *WebRTCManager) setupPeerConnectionHandlers(userID int64, pc *webrtc.PeerConnection, ctx context.Context) {
	// ICE candidate handler
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			log.Println("✅ ICE gathering complete")
			time.Sleep(1 * time.Second)
			if pc.LocalDescription() != nil {
				w.mu.RLock()
				state, exists := w.connections[userID]
				w.mu.RUnlock()
				// Only send if this PC is still the active one
				if exists && state.pc == pc {
					go w.sendICECandidatesFromSDP(userID, state.channelID, pc.LocalDescription().SDP)
				}
			}
			return
		}
		w.sendICECandidate(userID, candidate)
	})

	// Connection state handler
	// IMPORTANT: capture `pc` so we can verify this callback belongs to the
	// currently-active peer connection before cleaning up. Without this check,
	// a stale PC that closes after being replaced by a re-offer would call
	// cleanupConnection and destroy the new, active connection.
	pc.OnConnectionStateChange(func(connState webrtc.PeerConnectionState) {
		log.Printf("🔗 Connection: %s", connState.String())
		switch connState {
		case webrtc.PeerConnectionStateConnected:
			log.Println("🎉 WebRTC CONNECTED!")
			w.startWelcomeAudio(userID)

		case webrtc.PeerConnectionStateClosed, webrtc.PeerConnectionStateFailed:
			log.Printf("🔴 Connection closed/failed: %s", connState.String())
			// Only cleanup if this PC is still the active one for this user.
			w.mu.RLock()
			current, exists := w.connections[userID]
			w.mu.RUnlock()
			if exists && current.pc == pc {
				w.cleanupConnection(userID)
			}
		}
	})

	// Track handler
	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("🎬 Track: %s (Codec: %s)", track.Kind().String(), track.Codec().MimeType)

		if track.Kind() == webrtc.RTPCodecTypeVideo {
			codec := track.Codec().MimeType

			if strings.Contains(codec, "VP8") {
				log.Println("   ✅ VP8 detected - OPTIMIZED real-time face detection enabled")

				ssrc := uint32(track.SSRC())

				// Send immediate PLI
				go func() {
					for i := 0; i < 3; i++ {
						if err := pc.WriteRTCP([]rtcp.Packet{
							&rtcp.PictureLossIndication{MediaSSRC: ssrc},
						}); err == nil {
							log.Println("   ⚡ Immediate PLI sent (forcing IDR)")
						}
						time.Sleep(100 * time.Millisecond)
					}
				}()

				// Periodic PLI sender
				go w.startPLISender(ctx, pc, ssrc)

				// Face detection
				go w.realtimeFaceDetectionCapture(userID, track, ctx)
			}
		}
	})
}

// ============================================================
// AUDIO TRACK SETUP
// ============================================================

func (w *WebRTCManager) setupAudioTrack(userID int64, pc *webrtc.PeerConnection) error {
	audioTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		"audio",
		"bot-audio-stream",
	)
	if err != nil {
		return fmt.Errorf("failed to create audio track: %w", err)
	}

	rtpSender, err := pc.AddTrack(audioTrack)
	if err != nil {
		return fmt.Errorf("failed to add track: %w", err)
	}

	log.Println("   ✅ Audio track added to peer connection")

	// RTCP reader
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
				return
			}
		}
	}()

	w.mu.Lock()
	defer w.mu.Unlock()

	if state, exists := w.connections[userID]; exists {
		state.audioPlayer = audio.NewAudioPlayer(audioTrack, state.audioStop)
		log.Println("   ✅ Audio player initialized")
	}

	return nil
}

// ============================================================
// PLI SENDER
// ============================================================

func (w *WebRTCManager) startPLISender(ctx context.Context, pc *webrtc.PeerConnection, ssrc uint32) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	consecutiveErrors := 0
	maxErrors := 3

	defer func() {
		log.Println("   🛑 PLI sender stopped")
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			// Check state before sending
			state := pc.ConnectionState()
			if state == webrtc.PeerConnectionStateClosed ||
				state == webrtc.PeerConnectionStateFailed {
				return
			}

			// Send PLI
			if err := pc.WriteRTCP([]rtcp.Packet{
				&rtcp.PictureLossIndication{MediaSSRC: ssrc},
			}); err != nil {
				consecutiveErrors++
				if consecutiveErrors >= maxErrors {
					log.Printf("   ⚠️  PLI stopping (errors: %d)", consecutiveErrors)
					return
				}
			} else {
				consecutiveErrors = 0
				log.Println("   ✉️  PLI sent")
			}
		}
	}
}
