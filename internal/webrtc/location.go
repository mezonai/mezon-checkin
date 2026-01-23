package webrtc

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"mezon-checkin-bot/models"
	"os"
	"path/filepath"
	"time"
)

// ============================================================
// LOAD OFFICES
// ============================================================

func (c *LocationConfig) LoadOffices() error {
	if !c.Enabled {
		log.Println("📍 Location validation is disabled")
		return nil
	}

	workDir, _ := os.Getwd()
	log.Printf("🔍 Current working directory: %s", workDir)
	log.Printf("🔍 Looking for offices file at: %s", c.OfficesFilePath)

	if _, err := os.Stat(c.OfficesFilePath); os.IsNotExist(err) {
		log.Printf("⚠️  Offices file not found, creating default...")

		dir := filepath.Dir(c.OfficesFilePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		if err := c.createDefaultOfficesFile(); err != nil {
			return fmt.Errorf("failed to create default offices file: %w", err)
		}

		log.Printf("✅ Created default offices file at: %s", c.OfficesFilePath)
	}

	data, err := os.ReadFile(c.OfficesFilePath)
	if err != nil {
		return fmt.Errorf("failed to read offices file: %w", err)
	}

	var officeList OfficeList
	if err := json.Unmarshal(data, &officeList); err != nil {
		return fmt.Errorf("failed to parse offices JSON: %w", err)
	}

	c.mu.Lock()
	c.offices = make([]Office, 0, len(officeList.Offices))
	for _, office := range officeList.Offices {
		if office.Enabled {
			c.offices = append(c.offices, office)
		}
	}
	c.mu.Unlock()

	if len(c.offices) == 0 {
		return fmt.Errorf("no enabled offices found in %s", c.OfficesFilePath)
	}

	log.Printf("✅ Loaded %d office location(s):", len(c.offices))
	for _, office := range c.offices {
		log.Printf("   - %s: (%.6f, %.6f) - radius: %.0fm",
			office.Name, office.Latitude, office.Longitude, office.RadiusMeters)
	}

	return nil
}

func (c *LocationConfig) createDefaultOfficesFile() error {
	defaultOffices := OfficeList{
		Offices: []Office{
			{
				ID:           "HN1",
				Name:         "Văn phòng Hà Nội 1 - 2nd Floor, CT3 The Pride, To Huu Street, Ha Dong, Ha Noi",
				Latitude:     20.9725054,
				Longitude:    105.7575887,
				RadiusMeters: 100,
				Enabled:      true,
			},
			{
				ID:           "HN2",
				Name:         "Văn phòng Hà Nội 2 - 7th Floor, VinFast My Dinh Building, 8 Pham Hung Street, Tu Liem, Ha Noi",
				Latitude:     21.033618,
				Longitude:    105.7796304,
				RadiusMeters: 100,
				Enabled:      true,
			},
			{
				ID:           "HN3",
				Name:         "Văn phòng Hà Nội 3 - 8th Floor, Vinaconex Diamond Tower, 459C Bach Mai street, Bach Mai, Ha Noi",
				Latitude:     21.0019608,
				Longitude:    105.8466433,
				RadiusMeters: 100,
				Enabled:      true,
			},
			{
				ID:           "DN",
				Name:         "Văn phòng Đà Nẵng - NCC Building, 498 - 500 Nguyen Huu Tho Street, Cam Le, Da Nang",
				Latitude:     16.0293578,
				Longitude:    108.2086351,
				RadiusMeters: 100,
				Enabled:      true,
			},
			{
				ID:           "HCM",
				Name:         "Văn phòng TP.HCM - 8th Floor, ST. MORITZ Tower, 1014 Pham Van Dong Street, Hiep Binh, Ho Chi Minh City",
				Latitude:     10.8380556,
				Longitude:    106.7351069,
				RadiusMeters: 100,
				Enabled:      true,
			},
			{
				ID:           "VINH",
				Name:         "Văn phòng Vinh - 4th Floor, HD Building, Vinh – Cua Lo Boulevard, Block 17, Vinh Phu Ward, Nghe An",
				Latitude:     18.7007581,
				Longitude:    105.6798281,
				RadiusMeters: 100,
				Enabled:      true,
			},
			{
				ID:           "QN",
				Name:         "Văn phòng Quy Nhơn - 3rd Floor, Hibecco Building, 307 Nguyen Thi Minh Khai Street, Quy Nhon Nam, Gia Lai",
				Latitude:     13.760556,
				Longitude:    109.213177,
				RadiusMeters: 100,
				Enabled:      true,
			},
		},
	}

	data, err := json.MarshalIndent(defaultOffices, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal default offices: %w", err)
	}

	if err := os.WriteFile(c.OfficesFilePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func (c *LocationConfig) GetOffices() []Office {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy to prevent external modification
	offices := make([]Office, len(c.offices))
	copy(offices, c.offices)
	return offices
}

// ============================================================
// DISTANCE CALCULATION (Haversine Formula)
// ============================================================

func toRadians(degrees float64) float64 {
	return degrees * math.Pi / 180.0
}

func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusMeters = 6371000.0

	lat1Rad := toRadians(lat1)
	lat2Rad := toRadians(lat2)
	deltaLat := toRadians(lat2 - lat1)
	deltaLon := toRadians(lon2 - lon1)

	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusMeters * c
}

// ============================================================
// FIND NEAREST OFFICE
// ============================================================

func (w *WebRTCManager) findNearestOffice(lat, lon float64) *LocationMatch {
	offices := w.locationConfig.GetOffices()

	if len(offices) == 0 {
		return nil
	}

	var bestMatch *LocationMatch

	for _, office := range offices {
		distance := calculateDistance(
			office.Latitude,
			office.Longitude,
			lat,
			lon,
		)

		match := &LocationMatch{
			Office:   office,
			Distance: distance,
			IsValid:  distance <= office.RadiusMeters,
		}

		if bestMatch == nil || distance < bestMatch.Distance {
			bestMatch = match
		}
	}

	return bestMatch
}

// ============================================================
// VALIDATE LOCATION
// ============================================================

func (w *WebRTCManager) validateLocation(lat, lon float64) bool {
	if !w.locationConfig.Enabled {
		log.Println("⚠️  Location validation disabled")
		return true
	}

	if lat == 0 && lon == 0 {
		log.Println("❌ Invalid coordinates: (0, 0)")
		return false
	}

	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		log.Printf("❌ Invalid coordinates range: (%.6f, %.6f)", lat, lon)
		return false
	}

	match := w.findNearestOffice(lat, lon)
	if match == nil {
		log.Println("❌ No offices configured")
		return false
	}

	log.Printf("📍 Location validation:")
	log.Printf("   User location: (%.6f, %.6f)", lat, lon)
	log.Printf("   Nearest office: %s", match.Office.Name)
	log.Printf("   Office location: (%.6f, %.6f)", match.Office.Latitude, match.Office.Longitude)
	log.Printf("   Distance: %.2f meters", match.Distance)
	log.Printf("   Max allowed: %.2f meters", match.Office.RadiusMeters)

	if match.IsValid {
		log.Printf("   ✅ Location is VALID (within %s radius)", match.Office.Name)
	} else {
		log.Printf("   ❌ Location is INVALID (%.2fm > %.2fm from %s)",
			match.Distance, match.Office.RadiusMeters, match.Office.Name)

		offices := w.locationConfig.GetOffices()
		if len(offices) > 1 {
			log.Printf("   Other offices:")
			for _, office := range offices {
				if office.ID != match.Office.ID {
					dist := calculateDistance(office.Latitude, office.Longitude, lat, lon)
					log.Printf("      - %s: %.2fm away", office.Name, dist)
				}
			}
		}
	}

	return match.IsValid
}

// ============================================================
// LOCATION MESSAGE HANDLER
// ============================================================

func (w *WebRTCManager) SetupLocationHandler() {
	log.Println("🎧 Setting up location message handler...")

	w.client.On("location_message_received", func(data interface{}) {
		w.handleLocationMessageEvent(data)
	})

	log.Println("✅ Location handler setup complete")
}

func (w *WebRTCManager) handleLocationMessageEvent(data interface{}) {
	eventMap, ok := data.(map[string]interface{})
	if !ok {
		log.Printf("❌ Invalid location event data type")
		return
	}

	userID, _ := eventMap["user_id"].(int64)
	channelID, _ := eventMap["channel_id"].(int64)
	displayName, _ := eventMap["display_name"].(string)
	latitude, latOk := eventMap["latitude"].(float64)
	longitude, lonOk := eventMap["longitude"].(float64)

	if !latOk || !lonOk {
		log.Printf("❌ Missing or invalid coordinates in event")
		return
	}

	if userID == 0 || channelID == 0 {
		log.Printf("❌ Missing user_id or channel_id in event")
		return
	}

	log.Printf("📍 Processing location from %s (%d)", displayName, userID)
	log.Printf("   Coordinates: (%.6f, %.6f)", latitude, longitude)

	if err := w.HandleLocationReply(userID, channelID, latitude, longitude); err != nil {
		log.Printf("❌ Failed to handle location reply: %v", err)
	}
}

// ============================================================
// HANDLE LOCATION REPLY
// ============================================================

func (w *WebRTCManager) HandleLocationReply(userID int64, channelID int64, latitude, longitude float64) error {
	w.confirmationMu.Lock()
	state, exists := w.pendingConfirmations[userID]
	if !exists {
		w.confirmationMu.Unlock()
		log.Printf("⚠️  No pending confirmation for user %d", userID)
		return fmt.Errorf("no pending confirmation")
	}

	state.mu.Lock()
	state.confirmed = true
	state.mu.Unlock()

	state.cancelOnce.Do(func() {
		if state.timer != nil {
			state.timer.Stop()
		}
	})

	delete(w.pendingConfirmations, userID)
	w.confirmationMu.Unlock()

	log.Printf("✅ Location confirmed from user %d: (%.6f, %.6f)", userID, latitude, longitude)

	isValidLocation := w.validateLocation(latitude, longitude)

	if !isValidLocation {
		log.Printf("❌ Invalid location for user %d", userID)
		if err := w.SendCheckinFailed(channelID, userID, "Vị trí không hợp lệ"); err != nil {
			log.Printf("❌ Failed to send invalid location message: %v", err)
		}

		w.mu.RLock()
		_, connExists := w.connections[userID]
		w.mu.RUnlock()

		if connExists {
			w.playCheckinFailAudio(userID)
			go w.endCallAfterDelay(userID, "invalid_location", 2*time.Second)
		}
		return fmt.Errorf("invalid location")
	}

	// Call API to update status
	reqBody := models.UpdateStatus{
		UserId: userID,
		Status: "APPROVED",
	}

	body, statusCode, err := w.apiClient.SendRequest(reqBody, models.APIUpdateStatus)
	if err != nil {
		log.Printf("❌ API request failed: %v", err)
		return err
	}

	w.apiClient.LogResponse(body, statusCode)

	if !w.apiClient.IsSuccessStatusCode(statusCode) {
		if len(body) > 0 && len(body) < 500 {
			log.Printf("   Error: %s", string(body))
		}
		if err := w.SendCheckinFailed(channelID, userID, "Vị trí không hợp lệ"); err != nil {
			log.Printf("❌ Failed to send invalid location message: %v", err)
		}
		return fmt.Errorf("API returned status %d", statusCode)
	}

	if err := w.SendCheckinSuccess(channelID, userID, ""); err != nil {
		log.Printf("❌ Failed to send success message: %v", err)
		return err
	}

	return nil
}

// ============================================================
// CONFIRMATION TIMEOUT
// ============================================================

func (w *WebRTCManager) startConfirmationTimeout(userID, channelID int64) {
	w.confirmationMu.Lock()

	// Cancel old confirmation if exists
	if oldState, exists := w.pendingConfirmations[userID]; exists {
		oldState.cancelOnce.Do(func() {
			if oldState.timer != nil {
				oldState.timer.Stop()
			}
		})
	}

	timer := time.AfterFunc(60*time.Second, func() {
		w.handleConfirmationTimeout(userID, channelID)
	})

	w.pendingConfirmations[userID] = &confirmationState{
		userID:    userID,
		channelID: channelID,
		timer:     timer,
		confirmed: false,
	}

	w.confirmationMu.Unlock()

	log.Printf("⏰ Started 60s confirmation timer for user %d", userID)
}

func (w *WebRTCManager) handleConfirmationTimeout(userID int64, channelID int64) {
	w.confirmationMu.Lock()
	state, exists := w.pendingConfirmations[userID]
	if !exists {
		w.confirmationMu.Unlock()
		return
	}

	state.mu.Lock()
	alreadyConfirmed := state.confirmed
	state.mu.Unlock()

	if alreadyConfirmed {
		delete(w.pendingConfirmations, userID)
		w.confirmationMu.Unlock()
		log.Printf("✅ User %d already confirmed, skipping timeout", userID)
		return
	}

	delete(w.pendingConfirmations, userID)
	w.confirmationMu.Unlock()

	log.Printf("⏱️ Confirmation timeout for user %d - no location received", userID)

	if err := w.SendCheckinFailed(channelID, userID, "Hết thời gian xác nhận vị trí"); err != nil {
		log.Printf("❌ Failed to send timeout message: %v", err)
	}

	w.mu.RLock()
	_, connExists := w.connections[userID]
	w.mu.RUnlock()

	if connExists {
		w.playCheckinFailAudio(userID)
		go w.endCallAfterDelay(userID, "confirmation_timeout", 2*time.Second)
	}
}
