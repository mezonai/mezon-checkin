package webrtc

import (
	"context"
	"mezon-checkin-bot/internal/api"
	"mezon-checkin-bot/internal/audio"
	"mezon-checkin-bot/internal/client"
	"mezon-checkin-bot/internal/detector"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

// ============================================================
// CORE MANAGER
// ============================================================

type WebRTCManager struct {
	connections          map[int64]*connectionState
	mu                   sync.RWMutex
	client               *client.MezonClient
	faceDetector         *detector.FaceDetector
	audioConfig          audio.AudioConfig
	audioLibrary         *audio.AudioLibrary
	bufferPool           *bufferPool
	captureConfig        CaptureConfig
	dimensionConfig      DimensionConfig
	dmManager            *client.DMManager
	pendingConfirmations map[int64]*confirmationState
	confirmationMu       sync.RWMutex
	locationConfig       *LocationConfig
	shutdown             chan struct{}
	shutdownOnce         sync.Once
	apiClient            *api.APIClient
}

// ============================================================
// CONNECTION STATE
// ============================================================

type connectionState struct {
	pc          *webrtc.PeerConnection
	channelID   int64
	audioPlayer *audio.AudioPlayer
	audioStop   chan struct{}
	cancelFunc  context.CancelFunc
	cleanupOnce sync.Once
	endCallOnce sync.Once
	mu          sync.Mutex
	pendingICE  []webrtc.ICECandidateInit
	iceReady    bool
}

// ============================================================
// CONFIRMATION STATE
// ============================================================

type confirmationState struct {
	userID     int64
	channelID  int64
	timer      *time.Timer
	cancelOnce sync.Once
	confirmed  bool
	mu         sync.Mutex
}

// ============================================================
// CAPTURE STATE
// ============================================================

type captureState struct {
	lastCaptureTime       time.Time
	totalAttempts         int
	successCount          int
	rtpCount              int
	firstKeyframeReceived bool
}

// ============================================================
// LOCATION STRUCTURES
// ============================================================

type LocationConfig struct {
	Enabled         bool
	OfficesFilePath string
	offices         []Office
	mu              sync.RWMutex
}

type Office struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	RadiusMeters float64 `json:"radius_meters"`
	Enabled      bool    `json:"enabled"`
}

type OfficeList struct {
	Offices []Office `json:"offices"`
}

type LocationMatch struct {
	Office   Office
	Distance float64
	IsValid  bool
}

// ============================================================
// DIMENSION & CAPTURE CONFIG
// ============================================================

type DimensionConfig struct {
	MaxDecodeWidth      int
	MaxDecodeHeight     int
	DetectionWidth      int
	SkipDetectionResize bool
	MinFaceSize         int
	ExpandRatio         float64
}

type CaptureConfig struct {
	CaptureTimeout  time.Duration
	PLITimeout      time.Duration
	InitialRTPCount int
	CaptureInterval time.Duration
	MaxAttempts     int
	SampleBufferMax uint16
}

// ============================================================
// BUFFER POOL
// ============================================================

type bufferPool struct {
	pool sync.Pool
}
