package detector

import (
	"fmt"
	"log"
	"mezon-checkin-bot/internal/api"
	"mezon-checkin-bot/models"
	"time"
)

// ============================================================
// FACE RECOGNITION SERVICE
// ============================================================

type FaceRecognitionService struct {
	apiClient *api.APIClient
	config    *models.FaceRecognitionConfig
}

// NewFaceRecognitionService creates a new face recognition service
func NewFaceRecognitionService(apiClient *api.APIClient) *FaceRecognitionService {
	return &FaceRecognitionService{
		apiClient: api.NewAPIClient(30 * time.Second),
	}
}

// SubmitImage submits a base64 encoded image to the face recognition API
func (s *FaceRecognitionService) SubmitImage(base64Img string, userId int64, attemptNum int) (*models.FaceRecognitionResponse, error) {
	log.Printf("\n📤 [Attempt %d/5] Submitting image to API...", attemptNum)

	// Prepare request payload
	reqBody := models.FaceRecognitionRequest{
		UserId: userId,
		Imgs:   []string{base64Img},
	}

	// Send request
	body, statusCode, err := s.apiClient.SendRequest(reqBody, models.APICheckIn)
	if err != nil {
		log.Printf("❌ API request failed: %v", err)
		return nil, err
	}

	// Log response
	s.apiClient.LogResponse(body, statusCode)

	// Check status code
	if !s.apiClient.IsSuccessStatusCode(statusCode) {
		if len(body) > 0 && len(body) < 500 {
			log.Printf("   Error: %s", string(body))
		}
		return nil, fmt.Errorf("API returned status %d", statusCode)
	}

	// Parse response
	var result models.FaceRecognitionResponse
	if err := s.apiClient.ParseResponse(body, &result); err != nil {
		log.Printf("⚠️  Failed to parse response JSON: %v", err)
		return nil, err
	}

	// Log recognition details
	s.logRecognitionResult(&result)

	return &result, nil
}

// logRecognitionResult logs the details of the face recognition result
func (s *FaceRecognitionService) logRecognitionResult(result *models.FaceRecognitionResponse) {
	log.Printf("👤 Employee: %s %s", result.FirstName, result.LastName)
	log.Printf("🎯 Status: %s", result.FacialRecognitionStatus)
	log.Printf("✅ Identity Verified: %v", result.IdentityVerified)
	log.Printf("📊 Probability: %.2f%%", result.Probability*100)

	if result.HasLastClockEvent() {
		log.Printf("⏰ Last Clock: %s", result.LastClockEventDTO.StartTime)
	}
}
