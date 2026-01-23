package models

import (
	"fmt"
	"mezon-checkin-bot/internal/api"

	"gocv.io/x/gocv"
)

// ============================================================
// FACE RECOGNITION
// ============================================================
type FaceDetector struct {
	Classifier         gocv.CascadeClassifier
	Config             FaceRecognitionConfig
	RecognitionService *FaceRecognitionService
}

type FaceRecognitionService struct {
	apiClient *api.APIClient
}
type FaceRecognitionRequest struct {
	UserId int64    `json:"userId"`
	Imgs   []string `json:"imgs"`
}

// ============================================================
// RESPONSE STRUCTURES
// ============================================================

type FaceRecognitionResponse struct {
	FacialRecognitionStatus string             `json:"facialRecognitionStatus"`
	ImageVerifyID           string             `json:"imageVerifyId"`
	EmployeeID              string             `json:"employeeId"`
	AccountEmployeeID       string             `json:"accountEmployeeId"`
	FirstName               string             `json:"firstName"`
	LastName                string             `json:"lastName"`
	Shifts                  []interface{}      `json:"shifts"`
	LastClockEventDTO       *LastClockEventDTO `json:"lastClockEventDTO"`
	IdentityVerified        bool               `json:"identityVerified"`
	Probability             float64            `json:"probability"`
	ShowMessage             bool               `json:"showMessage"`
	IsWFH                   bool               `json:"isWFH"`
}

type LastClockEventDTO struct {
	ClockID   string  `json:"clockId"`
	ShiftID   *string `json:"shiftId"`
	StartTime string  `json:"startTime"`
	EndTime   *string `json:"endTime"`
	LastBreak *string `json:"lastBreak"`
}

// ============================================================
// HELPER METHODS
// ============================================================

// GetFullName returns the full name of the employee
func (r *FaceRecognitionResponse) GetFullName() string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf("%s %s", r.FirstName, r.LastName)
}

// String returns a formatted string representation of the response
func (r *FaceRecognitionResponse) String() string {
	if r == nil {
		return "nil"
	}

	return fmt.Sprintf(
		"FaceRecognition{Name: %s, Status: %s, Verified: %v, Probability: %.2f%%}",
		r.GetFullName(),
		r.FacialRecognitionStatus,
		r.IdentityVerified,
		r.Probability*100,
	)
}

// IsSuccessful checks if the face recognition was successful
func (r *FaceRecognitionResponse) IsSuccessful() bool {
	return r != nil && r.IdentityVerified && r.FacialRecognitionStatus != ""
}

// HasLastClockEvent checks if there's a last clock event
func (r *FaceRecognitionResponse) HasLastClockEvent() bool {
	return r != nil && r.LastClockEventDTO != nil
}
