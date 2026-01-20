package models

import "os"

// ============================================================
// WEBRTC CONSTANTS
// ============================================================

const (
	WebrtcSDPInit              = 0
	WebrtcSDPOffer             = 1
	WebrtcSDPAnswer            = 2
	WebrtcICECandidate         = 3
	WebrtcSDPQuit              = 4
	WebrtcSDPTimeout           = 5
	WebrtcSDPNotAvailable      = 6
	WebrtcSDPJoinedOtherCall   = 7
	WebrtcSDPStatusRemoteMedia = 8
)

// ============================================================
// API CONSTANTS - Sử dụng function để lấy env runtime
// ============================================================

var (
	// BaseURL được lấy từ environment variable
	BaseURL = getBaseURL()

	// API endpoints
	APICheckIn      = BaseURL + "/employees/bot/check-in"
	APIUpdateStatus = BaseURL + "/employees/bot/update-status"
)

// getBaseURL lấy BASE_URL từ environment variable
// Nếu không có, trả về default value
func getBaseURL() string {
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		// Default fallback (optional)
		return "http://localhost:8080"
	}
	return baseURL
}

// ============================================================
// WEBRTC SIGNALING MODELS
// ============================================================

type UpdateStatus struct {
	UserId string `json:"userId"`
	Status string `json:"status"`
}
