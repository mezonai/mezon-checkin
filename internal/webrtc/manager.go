package webrtc

import (
	"fmt"
	"log"
	"mezon-checkin-bot/internal/api"
	"mezon-checkin-bot/internal/audio"
	"mezon-checkin-bot/internal/client"
	"mezon-checkin-bot/internal/detector"
	"mezon-checkin-bot/mezon-protobuf/go/rtapi"
	"mezon-checkin-bot/models"
	"sync"
	"time"
)

// ============================================================
// MANAGER INITIALIZATION
// ============================================================

func NewWebRTCManager(
	mezonClient *client.MezonClient,
	outputDir string,
	faceConfig *models.FaceRecognitionConfig,
	audioConfig audio.AudioConfig,
	locationConfig *LocationConfig,
	apiClient *api.APIClient,
) (*WebRTCManager, error) {
	if mezonClient == nil {
		return nil, fmt.Errorf("MezonClient cannot be nil")
	}

	faceDetector, err := detector.NewFaceDetector(faceConfig, apiClient)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize face detector: %w", err)
	}

	if err := locationConfig.LoadOffices(); err != nil {
		return nil, fmt.Errorf("failed to load offices: %w", err)
	}

	audioLibrary := audio.NewAudioLibrary()

	if audioConfig.Enabled {
		audioFiles := map[string]string{
			"welcome":          audioConfig.WelcomeAudioPath,
			"checkin_success":  audioConfig.CheckinSuccessPath,
			"checkin_fail":     audioConfig.CheckinFailPath,
			"background_music": audioConfig.BackgroundMusicPath,
		}

		for name, path := range audioFiles {
			if path != "" {
				if err := audioLibrary.Register(name, path); err != nil {
					log.Printf("⚠️  Failed to register %s audio: %v", name, err)
				}
			}
		}

		log.Printf("🎵 Audio system initialized: %d audio files registered", len(audioLibrary.List()))
	}

	dmManager := client.NewDMManager(mezonClient)

	webrtc := &WebRTCManager{
		connections:          make(map[string]*connectionState),
		client:               mezonClient,
		faceDetector:         faceDetector,
		audioConfig:          audioConfig,
		audioLibrary:         audioLibrary,
		bufferPool:           newBufferPool(),
		captureConfig:        DefaultCaptureConfig(),
		dimensionConfig:      DefaultDimensionConfig(),
		dmManager:            dmManager,
		pendingConfirmations: make(map[string]*confirmationState),
		locationConfig:       locationConfig,
		shutdown:             make(chan struct{}),
		apiClient:            apiClient,
	}

	webrtc.SetupLocationHandler()
	webrtc.SetupProtobufHandler()
	return webrtc, nil
}

// ============================================================
// PROTOBUF HANDLER SETUP
// ============================================================

func (w *WebRTCManager) SetupProtobufHandler() {
	log.Println("🎧 Setting up WebRTC protobuf handler...")

	w.client.On("webrtc_signaling_fwd", func(data interface{}) {
		pbMsg, ok := data.(*rtapi.WebrtcSignalingFwd)
		if !ok {
			log.Printf("❌ Invalid webrtc_signaling_fwd data type: %T", data)
			return
		}

		event := &rtapi.WebrtcSignalingFwd{
			CallerId:   pbMsg.GetCallerId(),
			ReceiverId: pbMsg.GetReceiverId(),
			ChannelId:  pbMsg.GetChannelId(),
			DataType:   int32(pbMsg.GetDataType()),
			JsonData:   pbMsg.GetJsonData(),
		}

		// Determine user ID
		var userID string

		// If Bot is receiver → signal from User to bot
		if event.ReceiverId == w.client.ClientID {
			userID = event.CallerId
			log.Printf("📞 Signal FROM user %s TO bot", userID)
		} else if event.CallerId == w.client.ClientID {
			// If Bot is caller → echo back of signal bot sent
			userID = event.ReceiverId
			log.Printf("📞 Signal FROM bot TO user %s (echo)", userID)
		} else {
			// Signal not related to bot
			log.Printf("⚠️  Signal không liên quan đến bot (Caller: %s, Receiver: %s)",
				event.CallerId, event.ReceiverId)
			return
		}

		if userID == "" {
			log.Printf("❌ Could not determine user ID")
			return
		}

		log.Printf("📞 WebRTC signal - Type: %d, Channel: %s, UserID: %s",
			event.DataType, event.ChannelId, userID)

		go func() {
			if err := w.HandleSignal(userID, event); err != nil {
				log.Printf("❌ Error handling WebRTC signal: %v", err)
			}
		}()
	})

	log.Println("✅ WebRTC protobuf handler setup complete")
}

// ============================================================
// SHUTDOWN
// ============================================================

func (w *WebRTCManager) CloseAll() {
	w.shutdownOnce.Do(func() {
		close(w.shutdown)
		log.Println("🛑 Shutdown starting...")

		// 1. Cancel confirmations
		w.confirmationMu.Lock()
		for _, state := range w.pendingConfirmations {
			state.cancelOnce.Do(func() {
				if state.timer != nil {
					state.timer.Stop()
				}
			})
		}
		w.pendingConfirmations = make(map[string]*confirmationState)
		w.confirmationMu.Unlock()

		// 2. Get connections
		w.mu.Lock()
		connections := make([]*connectionState, 0, len(w.connections))
		userIDs := make([]string, 0, len(w.connections))
		for uid, state := range w.connections {
			connections = append(connections, state)
			userIDs = append(userIDs, uid)
		}
		w.connections = make(map[string]*connectionState)
		w.mu.Unlock()

		// 3. Parallel cleanup with timeout
		done := make(chan struct{})
		go func() {
			var wg sync.WaitGroup
			for i, state := range connections {
				wg.Add(1)
				go func(s *connectionState, uid string) {
					defer wg.Done()
					if s.cancelFunc != nil {
						s.cancelFunc()
					}
					s.closeAudioStop()
					if s.pc != nil {
						s.pc.Close()
					}
					log.Printf("   ✅ Closed: %s", uid)
				}(state, userIDs[i])
			}
			wg.Wait()
			close(done)
		}()

		// Wait with timeout
		select {
		case <-done:
			log.Println("   ✅ All closed")
		case <-time.After(5 * time.Second):
			log.Println("   ⚠️  Timeout")
		}

		// 4. Close detector
		if w.faceDetector != nil {
			w.faceDetector.Close()
		}

		log.Println("🛑 Shutdown complete")
	})
}
