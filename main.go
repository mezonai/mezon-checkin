// Mezon WebRTC Bot - OPTIMIZED REAL-TIME FACE DETECTION
// Key optimizations:
// - Detect on scaled-down images (320px wide)
// - Reduced samplebuilder latency (maxLate: 128)
// - Better JPEG quality control (90)
// - Faster capture interval (1s)
// - Persistent ffmpeg process option (commented, can enable)

package main

import (
	"fmt"
	"io"
	"log"
	"mezon-checkin-bot/internal/api"
	"mezon-checkin-bot/internal/audio"
	"mezon-checkin-bot/internal/client"
	"mezon-checkin-bot/models"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"mezon-checkin-bot/internal/webrtc"
)

// ============================================================
// MAIN
// ============================================================

func main() {
	fmt.Println("╔════════════════════════════════════════════════════╗")
	fmt.Println("║  Mezon WebRTC Bot - OPTIMIZED FACE DETECTION     ║")
	fmt.Println("║  🚀 Performance improvements:                      ║")
	fmt.Println("║     - Detect on scaled images (320px)             ║")
	fmt.Println("║     - Reduced latency (maxLate: 128)              ║")
	fmt.Println("║     - Faster capture interval (1s)                ║")
	fmt.Println("║     - Controlled JPEG quality (90)                ║")
	fmt.Println("╚════════════════════════════════════════════════════╝")

	env := os.Getenv("APP_ENV") // hoặc "MODE", "ENV"
	fmt.Println("⚙️  Running in environment:", env)
	if env == "prod" || env == "production" {
		log.SetOutput(io.Discard)
	}

	botID := os.Getenv("BOT_ID")
	botToken := os.Getenv("BOT_TOKEN")

	host := os.Getenv("MEZON_HOST")
	if host == "" {
		host = "gw.mezon.ai"
	}

	port := os.Getenv("MEZON_PORT")
	if port == "" {
		port = "443"
	}

	useSSL := true
	if os.Getenv("MEZON_USE_SSL") == "false" {
		useSSL = false
	}
	botIDInt, err := strconv.ParseInt(botID, 10, 64)
	config := models.Config{
		BotID:    botIDInt,
		BotToken: botToken,
		Host:     host,
		Port:     port,
		UseSSL:   useSSL,
	}

	log.Printf("📋 Bot ID: %d", config.BotID)
	apiClient := api.NewAPIClient(30 * time.Second)
	client := client.NewMezonClient(config)
	defer client.Close() // IMPORTANT: Always defer Close()
	// Khởi tạo location config
	locationConfig := &webrtc.LocationConfig{
		Enabled:         true,
		OfficesFilePath: "config/offices.json", // Đường dẫn tương đối từ thư mục chạy
	}

	faceConfig := &models.FaceRecognitionConfig{

		Enabled:     true,
		MinFaceSize: 80,
		JPEGQuality: 90, // High quality JPEG (range: 1-100)
	}
	audioConfig := audio.AudioConfig{
		WelcomeAudioPath:   "./audio/welcome.ogg",
		CheckinSuccessPath: "./audio/checkin-success.ogg",
		CheckinFailPath:    "./audio/checkin-failed.ogg",
		Enabled:            true,
	}
	if err := client.Login(); err != nil {
		log.Fatalf("❌ Failed to login: %v", err)
	}

	webrtcManager, err := webrtc.NewWebRTCManager(client, "./image-captures", faceConfig, audioConfig, locationConfig, apiClient)
	if err != nil {
		log.Fatalf("❌ Failed to create WebRTC manager: %v", err)
	}

	log.Println("\n✅ Bot started with OPTIMIZED FACE DETECTION!")
	log.Println("📞 Waiting for calls...")
	log.Println("🎯 Optimizations:")
	log.Println("   ✅ VP8 video capture")
	log.Println("   ⚡ Fast face detection on scaled images (320px)")
	log.Println("   ⚡ Reduced latency (maxLate: 128 vs 512)")
	log.Println("   ⚡ Faster capture interval (1s vs 2s)")
	log.Println("   ⚡ Faster PLI requests (1s vs 2s)")
	log.Println("   ✅ High quality JPEG encoding (quality: 90)")
	log.Println("   ✅ Sends expanded square images when face detected")
	log.Println("   ✅ Sequential API submission (max 5 attempts)")
	log.Printf("   - API: %s", models.APICheckIn)
	log.Printf("   - Min face size: %dpx", faceConfig.MinFaceSize)
	log.Println("   Press Ctrl+C to stop")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	log.Println("\n⚠️  Shutting down...")
	webrtcManager.CloseAll()
	client.Close()
	log.Println("✅ Done!")
}
