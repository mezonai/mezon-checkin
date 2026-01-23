package models

// ============================================================
// CONFIGURATION
// ============================================================

type Config struct {
	BotID        int64
	BotToken     string
	Host         string
	Port         string
	UseSSL       bool
	SocketHost   string
	SocketPort   string
	SocketUseSSL bool
}

type FaceRecognitionConfig struct {
	Enabled     bool
	MinFaceSize int
	JPEGQuality int // Configurable JPEG quality (85-95 recommended)
}

// ============================================================
// SESSION & AUTHENTICATION
// ============================================================

type AuthRequest struct {
	Account struct {
		Appid string `json:"appid"`
		Token string `json:"token"`
	} `json:"account"`
}

type AuthResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
	Created      bool   `json:"created"`
	ApiURL       string `json:"api_url"`
	IDToken      string `json:"id_token"`
	IsRemember   bool   `json:"is_remember"`
}
