// ============================================================
// AUDIO PLAYER MODULE - Sử dụng độc lập
// ============================================================
package audio

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/oggreader"
)

// AudioItem đại diện cho một file audio cần phát
type AudioItem struct {
	FilePath string // Đường dẫn file OGG
	Name     string // Tên để log (VD: "greeting", "checkin_success")
	Loop     bool   // true = lặp lại, false = phát 1 lần
	OnFinish func() // Callback khi phát xong (optional)
}

// AudioPlayer quản lý việc phát audio cho một WebRTC track
type AudioPlayer struct {
	track       *webrtc.TrackLocalStaticSample
	stopChan    chan struct{}
	queue       chan AudioItem
	isPlaying   bool
	currentFile string
	mu          sync.Mutex
}

type AudioConfig struct {
	Enabled                bool
	WelcomeAudioPath       string
	CheckinSuccessPath     string
	CheckinFailPath        string
	BackgroundMusicPath    string
	BackgroundMusicEnabled bool
	GoodbyeAudioPath       string
}

// NewAudioPlayer tạo player mới
func NewAudioPlayer(track *webrtc.TrackLocalStaticSample, stopChan chan struct{}) *AudioPlayer {
	player := &AudioPlayer{
		track:     track,
		stopChan:  stopChan,
		queue:     make(chan AudioItem, 10), // Buffer 10 items
		isPlaying: false,
	}

	// Bắt đầu xử lý queue
	go player.processQueue()

	return player
}

// Play thêm audio vào queue (không ngắt audio đang phát)
func (ap *AudioPlayer) Play(item AudioItem) {
	select {
	case ap.queue <- item:
		log.Printf("🎵 Queued: %s", item.Name)
	case <-ap.stopChan:
		return
	default:
		log.Printf("⚠️  Queue full, skipping: %s", item.Name)
	}
}

// PlayNow ngắt audio hiện tại và phát ngay
func (ap *AudioPlayer) PlayNow(item AudioItem) {
	ap.mu.Lock()
	// Xóa toàn bộ queue
	for len(ap.queue) > 0 {
		<-ap.queue
	}
	ap.mu.Unlock()

	// Thêm vào queue (sẽ được phát ngay do queue rỗng)
	ap.Play(item)
}

// Stop dừng player
func (ap *AudioPlayer) Stop() {
	close(ap.stopChan)
}

// GetStatus trả về trạng thái hiện tại
func (ap *AudioPlayer) GetStatus() (isPlaying bool, currentFile string, queueSize int) {
	ap.mu.Lock()
	defer ap.mu.Unlock()
	return ap.isPlaying, ap.currentFile, len(ap.queue)
}

// processQueue xử lý queue audio (chạy trong goroutine)
func (ap *AudioPlayer) processQueue() {
	for {
		select {
		case <-ap.stopChan:
			log.Println("🛑 Audio player stopped")
			return

		case item := <-ap.queue:
			ap.playAudio(item)
		}
	}
}

// playAudio phát một file audio
func (ap *AudioPlayer) playAudio(item AudioItem) {
	ap.mu.Lock()
	ap.isPlaying = true
	ap.currentFile = item.Name
	ap.mu.Unlock()

	defer func() {
		ap.mu.Lock()
		ap.isPlaying = false
		ap.currentFile = ""
		ap.mu.Unlock()

		// Gọi callback nếu có
		if item.OnFinish != nil {
			item.OnFinish()
		}
	}()

	log.Printf("▶️  Playing: %s", item.Name)

	// Loop nếu cần
	for {
		err := ap.streamOGG(item.FilePath)

		if err == io.EOF {
			log.Printf("✅ Finished: %s", item.Name)
		} else if err != nil {
			log.Printf("❌ Error playing %s: %v", item.Name, err)
			return
		}

		// Không loop thì break
		if !item.Loop {
			break
		}

		// Kiểm tra nếu bị stop
		select {
		case <-ap.stopChan:
			return
		default:
			log.Printf("🔄 Looping: %s", item.Name)
		}
	}
}

// streamOGG đọc và stream file OGG Opus
func (ap *AudioPlayer) streamOGG(filePath string) error {
	// Mở file
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}
	defer file.Close()

	// Tạo OGG reader
	ogg, _, err := oggreader.NewWith(file)
	if err != nil {
		return fmt.Errorf("cannot create OGG reader: %w", err)
	}

	var lastGranule uint64
	packetCount := 0

	// Đọc từng Opus packet
	for {
		select {
		case <-ap.stopChan:
			return fmt.Errorf("stopped")
		default:
		}

		// Đọc page từ OGG
		pageData, pageHeader, err := ogg.ParseNextPage()
		if err == io.EOF {
			return io.EOF
		}
		if err != nil {
			return err
		}

		// Tính duration dựa trên granule position
		sampleDuration := time.Duration(0)
		if pageHeader.GranulePosition > lastGranule && lastGranule != 0 {
			sampleCount := pageHeader.GranulePosition - lastGranule
			// Opus = 48kHz
			sampleDuration = time.Duration((float64(sampleCount)/48000)*1000) * time.Millisecond
		}
		lastGranule = pageHeader.GranulePosition

		// Default 20ms nếu không tính được
		if sampleDuration == 0 {
			sampleDuration = 20 * time.Millisecond
		}

		// Ghi Opus frame vào WebRTC track
		if err := ap.track.WriteSample(media.Sample{
			Data:     pageData,
			Duration: sampleDuration,
		}); err != nil {
			return err
		}

		packetCount++

		// Sleep để giữ real-time playback
		time.Sleep(sampleDuration)
	}
}

// ============================================================
// AUDIO LIBRARY - Quản lý các file audio
// ============================================================

type AudioLibrary struct {
	sounds map[string]string // name -> file path
	mu     sync.RWMutex
}

func NewAudioLibrary() *AudioLibrary {
	return &AudioLibrary{
		sounds: make(map[string]string),
	}
}

// Register đăng ký một file audio
func (al *AudioLibrary) Register(name, filePath string) error {
	// Kiểm tra file tồn tại
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", filePath)
	}

	al.mu.Lock()
	al.sounds[name] = filePath
	al.mu.Unlock()

	log.Printf("📚 Registered audio: %s -> %s", name, filePath)
	return nil
}

// Get lấy đường dẫn file từ tên
func (al *AudioLibrary) Get(name string) (string, bool) {
	al.mu.RLock()
	defer al.mu.RUnlock()
	path, exists := al.sounds[name]
	return path, exists
}

// List liệt kê tất cả audio đã đăng ký
func (al *AudioLibrary) List() []string {
	al.mu.RLock()
	defer al.mu.RUnlock()

	names := make([]string, 0, len(al.sounds))
	for name := range al.sounds {
		names = append(names, name)
	}
	return names
}
