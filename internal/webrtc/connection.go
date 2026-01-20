package webrtc

import (
	"log"
	"mezon-checkin-bot/models"
	"time"
)

// ============================================================
// CONNECTION STATE HELPERS
// ============================================================

// Safe close of audioStop channel
func (cs *connectionState) closeAudioStop() {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.audioStop != nil {
		select {
		case <-cs.audioStop:
			// Already closed
		default:
			close(cs.audioStop)
		}
	}
}

// ============================================================
// CONNECTION CLEANUP
// ============================================================

func (w *WebRTCManager) cleanupConnection(userID string) {
	w.mu.Lock()
	state, exists := w.connections[userID]
	if !exists {
		w.mu.Unlock()
		return
	}
	delete(w.connections, userID)
	w.mu.Unlock()

	state.cleanupOnce.Do(func() {
		log.Printf("🧹 Cleaning up %s", userID)

		// 1. Cancel context (stops goroutines)
		if state.cancelFunc != nil {
			state.cancelFunc()
		}

		// 2. Wait for goroutines to finish
		time.Sleep(100 * time.Millisecond)

		// 3. Stop audio
		state.closeAudioStop()

		// 4. Close peer connection
		if state.pc != nil {
			if err := state.pc.Close(); err != nil {
				log.Printf("   ⚠️  PC close: %v", err)
			}
		}

		// 5. Send quit signal (best effort)
		if err := w.client.SendWebRTCSignal(
			userID,
			w.client.ClientID,
			state.channelID,
			models.WebrtcSDPQuit,
			"",
		); err != nil {
			log.Printf("   ⚠️  Quit signal: %v", err)
		}

		log.Printf("   ✅ Cleanup complete")
	})
}

// ============================================================
// DELAYED CALL END
// ============================================================

func (w *WebRTCManager) endCallAfterDelay(userID, reason string, delay time.Duration) {
	log.Printf("📞 Scheduling call end for user %s (reason: %s, delay: %v)", userID, reason, delay)

	time.Sleep(delay)

	w.mu.RLock()
	state, exists := w.connections[userID]
	w.mu.RUnlock()

	if !exists {
		log.Printf("   ⚠️  Connection already cleaned up")
		return
	}

	state.endCallOnce.Do(func() {
		log.Printf("   ✅ Ending call for user %s", userID)
		w.cleanupConnection(userID)
	})
}
