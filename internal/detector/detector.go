package detector

import (
	"fmt"
	"log"
	"mezon-checkin-bot/internal/api"
	"mezon-checkin-bot/models"

	"gocv.io/x/gocv"
)

// ============================================================
// FACE DETECTOR - Main detector with cascade classifier
// ============================================================

type FaceDetector struct {
	Classifier         gocv.CascadeClassifier
	Config             *models.FaceRecognitionConfig
	recognitionService *FaceRecognitionService
}

// NewFaceDetector creates a new face detector instance
func NewFaceDetector(config *models.FaceRecognitionConfig, apiClient *api.APIClient) (*FaceDetector, error) {
	detector := &FaceDetector{
		Config: config,
	}

	// Initialize face recognition service if enabled
	if config.Enabled {
		detector.recognitionService = NewFaceRecognitionService(
			apiClient,
		)

		// Load cascade classifier
		classifier := gocv.NewCascadeClassifier()
		if !classifier.Load("haarcascade_frontalface_default.xml") {
			return nil, fmt.Errorf("failed to load face cascade classifier")
		}
		detector.Classifier = classifier

		log.Println("✅ Face detector initialized")
		log.Printf("   Min face size: %dx%d", config.MinFaceSize, config.MinFaceSize)
		log.Printf("   JPEG quality: %d", config.JPEGQuality)
	}

	return detector, nil
}

// Close releases resources used by the detector
func (fd *FaceDetector) Close() {
	if fd.Config.Enabled && fd.Classifier != (gocv.CascadeClassifier{}) {
		fd.Classifier.Close()
	}
}

// SubmitSingleImageToAPI submits a single image to the face recognition API
// This method maintains backward compatibility with existing code
func (fd *FaceDetector) SubmitSingleImageToAPI(base64Img string, userId int64, attemptNum int) (*models.FaceRecognitionResponse, error) {
	if !fd.Config.Enabled {
		return nil, nil
	}

	if fd.recognitionService == nil {
		return nil, fmt.Errorf("face recognition service not initialized")
	}

	return fd.recognitionService.SubmitImage(base64Img, userId, attemptNum)
}

// GetRecognitionService returns the underlying face recognition service
// This allows direct access to the service if needed
func (fd *FaceDetector) GetRecognitionService() *FaceRecognitionService {
	return fd.recognitionService
}
