package webrtc

import (
	"log"
	"mezon-checkin-bot/internal/audio"
	"time"
)

// ============================================================
// WELCOME AUDIO
// ============================================================

func (w *WebRTCManager) startWelcomeAudio(userID string) {
	if !w.audioConfig.Enabled {
		return
	}

	w.mu.RLock()
	state, exists := w.connections[userID]
	w.mu.RUnlock()

	if !exists || state.audioPlayer == nil {
		log.Printf("⚠️  No audio player found for user %s", userID)
		return
	}

	welcomePath, hasWelcome := w.audioLibrary.Get("welcome")
	musicPath, hasMusic := w.audioLibrary.Get("background_music")

	if !hasWelcome {
		log.Println("⚠️  Welcome audio not configured")
		return
	}

	log.Println("🎵 Starting welcome sequence...")

	state.audioPlayer.Play(audio.AudioItem{
		FilePath: welcomePath,
		Name:     "welcome",
		Loop:     false,
		OnFinish: func() {
			log.Println("✅ Welcome audio finished")

			if hasMusic && w.audioConfig.BackgroundMusicEnabled {
				log.Println("🎵 Starting background music...")
				state.audioPlayer.Play(audio.AudioItem{
					FilePath: musicPath,
					Name:     "background_music",
					Loop:     true,
				})
			}
		},
	})
}

// ============================================================
// CHECKIN FAIL AUDIO
// ============================================================

func (w *WebRTCManager) playCheckinFailAudio(userID string) {
	if !w.audioConfig.Enabled {
		go w.endCallAfterDelay(userID, "checkin_fail_no_audio_config", 500*time.Millisecond)
		return
	}

	w.mu.RLock()
	state, exists := w.connections[userID]
	w.mu.RUnlock()

	if !exists || state.audioPlayer == nil {
		log.Printf("⚠️  No audio player found for user %s", userID)
		go w.endCallAfterDelay(userID, "checkin_fail_no_player", 500*time.Millisecond)
		return
	}

	checkinPath, hasCheckin := w.audioLibrary.Get("checkin_fail")
	if !hasCheckin {
		log.Println("⚠️  Checkin fail audio not configured")
		go w.endCallAfterDelay(userID, "checkin_fail_no_file", 500*time.Millisecond)
		return
	}

	log.Println("❌ Playing checkin fail audio...")

	state.audioPlayer.PlayNow(audio.AudioItem{
		FilePath: checkinPath,
		Name:     "checkin_fail",
		Loop:     false,
		OnFinish: func() {
			log.Println("✅ Checkin fail audio finished")
			go w.endCallAfterDelay(userID, "checkin_fail_complete", 1*time.Second)
		},
	})
}
