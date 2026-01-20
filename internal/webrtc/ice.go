package webrtc

import (
	"encoding/json"
	"log"
	"mezon-checkin-bot/models"
	"strings"
	"time"

	"github.com/pion/webrtc/v4"
)

// ============================================================
// SEND ICE CANDIDATE
// ============================================================

func (w *WebRTCManager) sendICECandidate(userID string, candidate *webrtc.ICECandidate) {
	w.mu.RLock()
	state, exists := w.connections[userID]
	w.mu.RUnlock()

	if !exists {
		return
	}

	init := candidate.ToJSON()
	candidateJSON, err := json.Marshal(init)
	if err != nil {
		log.Printf("⚠️  Failed to marshal ICE candidate: %v", err)
		return
	}

	if err := w.client.SendWebRTCSignal(
		userID,
		w.client.ClientID,
		state.channelID,
		models.WebrtcICECandidate,
		string(candidateJSON),
	); err != nil {
		log.Printf("⚠️  Failed to send ICE candidate: %v", err)
	}
}

// ============================================================
// EXTRACT & SEND ICE FROM SDP
// ============================================================

func (w *WebRTCManager) sendICECandidatesFromSDP(userID, channelID, sdp string) {
	log.Println("🔍 Extracting ICE candidates from SDP...")

	lines := strings.Split(strings.ReplaceAll(sdp, "\r\n", "\n"), "\n")
	midMap := make(map[int]string)
	mLineIndex := -1

	// First pass: build mid map
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "m=") {
			mLineIndex++
		}
		if strings.HasPrefix(line, "a=mid:") {
			mid := strings.TrimPrefix(line, "a=mid:")
			midMap[mLineIndex] = mid
		}
	}

	// Second pass: extract candidates
	mLineIndex = -1
	count := 0

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "m=") {
			mLineIndex++
		}

		if strings.HasPrefix(line, "a=candidate:") {
			candidateStr := strings.TrimPrefix(line, "a=")
			mid, hasMid := midMap[mLineIndex]
			if !hasMid {
				continue
			}

			candidate := map[string]interface{}{
				"candidate":     candidateStr,
				"sdpMid":        mid,
				"sdpMLineIndex": mLineIndex,
			}

			candidateJSON, err := json.Marshal(candidate)
			if err != nil {
				log.Printf("⚠️  Failed to marshal candidate: %v", err)
				continue
			}

			if err := w.client.SendWebRTCSignal(
				userID,
				w.client.ClientID,
				channelID,
				models.WebrtcICECandidate,
				string(candidateJSON),
			); err == nil {
				count++
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	log.Printf("✅ Sent %d ICE candidates from SDP\n", count)
}
