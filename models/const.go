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

	// WebrtcSDPCallRequest - tín hiệu user gọi vào bot (incoming call).
	// Bot phải phản hồi bằng WebrtcSDPInit (type 0) thì user mới
	// tiếp tục gửi SDP offer (type 1). Nếu không phản hồi, user sẽ
	// timeout và call không bao giờ bắt đầu.
	WebrtcSDPCallRequest = 50
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

var (
	CodeLocationSend = 17
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
	UserId int64  `json:"userId"`
	Status string `json:"status"`
}
