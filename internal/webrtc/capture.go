package webrtc

import (
	"context"
	"image"
	"log"
	"mezon-checkin-bot/models"
	"strings"
	"time"

	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/samplebuilder"
	"gocv.io/x/gocv"
)

// ============================================================
// REALTIME FACE DETECTION CAPTURE
// ============================================================

func (w *WebRTCManager) realtimeFaceDetectionCapture(userID int64, track *webrtc.TrackRemote, ctx context.Context) {
	log.Printf("📸 Starting face detection for %d...", userID)

	defer func() {
		log.Printf("   🧹 Face detection cleanup for %d", userID)
	}()

	sampleBuilder := samplebuilder.New(
		w.captureConfig.SampleBufferMax,
		&codecs.VP8Packet{},
		track.Codec().ClockRate,
	)

	captureState := &captureState{
		lastCaptureTime:       time.Now(),
		totalAttempts:         0,
		successCount:          0,
		rtpCount:              0,
		firstKeyframeReceived: false,
	}

	sampleChan := make(chan *media.Sample, 10)

	// RTP reader with context cancellation
	rtpCtx, rtpCancel := context.WithCancel(ctx)
	defer rtpCancel()

	go func() {
		defer close(sampleChan)
		for {
			select {
			case <-rtpCtx.Done():
				log.Println("   🛑 RTP reader stopped")
				return
			default:
				pkt, _, err := track.ReadRTP()
				if err != nil {
					if !strings.Contains(err.Error(), "closed") {
						log.Printf("   ⚠️  RTP error: %v", err)
					}
					return
				}

				sampleBuilder.Push(pkt)
				if sample := sampleBuilder.Pop(); sample != nil {
					select {
					case sampleChan <- sample:
					case <-rtpCtx.Done():
						return
					}
				}
			}
		}
	}()

	log.Println("   ⏳ Scanning for faces...")

	captureTimeout := time.After(w.captureConfig.CaptureTimeout)
	pliTimeout := time.After(w.captureConfig.PLITimeout)

	// Get connection state
	w.mu.RLock()
	state, exists := w.connections[userID]
	w.mu.RUnlock()

	if !exists {
		log.Printf("   ❌ Connection not found")
		return
	}

	// Main loop
	for {
		select {
		case <-ctx.Done():
			log.Printf("   🛑 Context cancelled")
			return

		case <-captureTimeout:
			log.Printf("   ⏱️  Timeout after %v", w.captureConfig.CaptureTimeout)
			w.handleCaptureFailure(userID, state, "timeout")
			return

		case <-pliTimeout:
			if !captureState.firstKeyframeReceived {
				log.Println("   ❌ PLI timeout")
				w.handleCaptureFailure(userID, state, "pli_timeout")
				return
			}

		case sample, ok := <-sampleChan:
			if !ok {
				log.Println("   📡 Stream ended")
				return
			}

			// Check max attempts
			if captureState.totalAttempts >= w.captureConfig.MaxAttempts {
				log.Printf("   ❌ Max attempts: %d/%d",
					captureState.successCount, captureState.totalAttempts)
				w.handleCaptureFailure(userID, state, "max_attempts")
				return
			}

			captureState.rtpCount++
			if captureState.rtpCount == w.captureConfig.InitialRTPCount {
				log.Printf("   📦 Video stream active")
			}

			// Process keyframes only
			if !isVP8Keyframe(sample.Data) {
				continue
			}

			if !captureState.firstKeyframeReceived {
				captureState.firstKeyframeReceived = true
				log.Println("   ✅ Keyframe received!")
			}

			// Rate limiting
			if time.Since(captureState.lastCaptureTime) < w.captureConfig.CaptureInterval {
				continue
			}

			// Decode frame
			img, err := w.vp8FrameToGoCV(sample.Data)
			if err != nil {
				continue
			}

			// Detect face
			hasFace, response := w.detectAndSendFullImage(*img, userID, captureState.totalAttempts+1)
			img.Close() // CRITICAL: Close immediately

			captureState.totalAttempts++

			// Handle success
			if hasFace && response != nil {
				captureState.lastCaptureTime = time.Now()
				captureState.successCount++

				if captureState.successCount > 0 {
					log.Printf("   ✅ RECOGNITION SUCCESS!")
					w.handleCaptureSuccess(userID, state, response)
					return
				}
			}
		}
	}
}

// ============================================================
// CAPTURE RESULT HANDLERS
// ============================================================

func (w *WebRTCManager) handleCaptureSuccess(userID int64, state *connectionState, response *models.FaceRecognitionResponse) {
	log.Println("   🎯 Processing successful checkin...")

	// Send confirmation message with timeout guarantee
	if response != nil && !response.IsWFH {
		done := make(chan error, 1)
		go func() {
			log.Println("   📧 Sending confirmation...")
			err := w.SendCheckinConfirmation(state.channelID, userID, response.GetFullName())
			done <- err
		}()

		// Wait with timeout
		select {
		case err := <-done:
			if err != nil {
				log.Printf("   ❌ Failed to send confirmation: %v", err)
			} else {
				log.Println("   ✅ Confirmation sent")
			}
		case <-time.After(5 * time.Second):
			log.Println("   ⚠️  Confirmation timeout")
		}
	}

	if response != nil && response.IsWFH {
		if err := w.SendCheckinSuccess(state.channelID, userID, ""); err != nil {
			log.Printf("❌ Failed to send success message: %v", err)
		}
	}

	// Play audio (non-blocking)
	log.Println("   🎵 Playing success audio...")
	go w.endCallAfterDelay(userID, "checkin_success_complete", 500*time.Millisecond)

	// Wait for audio to stream
	time.Sleep(500 * time.Millisecond)

	// Stop media pipeline
	log.Println("   🛑 Stopping media pipeline...")
	if state.cancelFunc != nil {
		state.cancelFunc()
	}

	log.Println("   ✅ Success handling complete!")
}

func (w *WebRTCManager) handleCaptureFailure(userID int64, state *connectionState, reason string) {
	log.Printf("   ❌ Capture failed: %s", reason)

	// Cancel context first
	if state.cancelFunc != nil {
		state.cancelFunc()
	}

	// Map reason to message
	failureMessages := map[string]string{
		"timeout":      "Hết thời gian chờ",
		"pli_timeout":  "Không nhận được video",
		"max_attempts": "Không xác định được danh tính",
	}

	failureMessage := failureMessages[reason]
	if failureMessage == "" {
		failureMessage = "Lỗi không xác định"
	}

	// Send failure message
	if err := w.SendCheckinFailed(state.channelID, userID, failureMessage); err != nil {
		log.Printf("   ❌ Failed to send message: %v", err)
	}
	go w.endCallAfterDelay(userID, "checkin_fail_no_audio_config", 500*time.Millisecond)

	// Play fail audio
	// w.playCheckinFailAudio(userID)
}

// ============================================================
// FACE DETECTION & SUBMISSION
// ============================================================

func (w *WebRTCManager) detectAndSendFullImage(img gocv.Mat, userId int64, attemptNum int) (bool, *models.FaceRecognitionResponse) {
	if !w.faceDetector.Config.Enabled || img.Empty() {
		return false, nil
	}

	origW := img.Cols()
	origH := img.Rows()

	if origW == 0 || origH == 0 {
		log.Printf("   ⚠️  Invalid image dimensions: %dx%d", origW, origH)
		return false, nil
	}

	var detectionImg gocv.Mat
	var scale float64 = 1.0
	needResize := true

	maxDetectionWidth := (w.dimensionConfig.DetectionWidth * 3) / 2
	if w.dimensionConfig.SkipDetectionResize && origW <= maxDetectionWidth {
		detectionImg = img
		needResize = false
		log.Printf("   📐 Detection: using decoded size %dx%d (resize skipped)", origW, origH)
	} else {
		targetW := w.dimensionConfig.DetectionWidth
		scale = float64(targetW) / float64(origW)
		targetH := int(float64(origH) * scale)

		detectionImg = gocv.NewMat()
		defer detectionImg.Close()
		gocv.Resize(img, &detectionImg, image.Pt(targetW, targetH), 0, 0, gocv.InterpolationLinear)

		log.Printf("   📐 Detection: %dx%d → %dx%d (scale=%.2f)",
			origW, origH, targetW, targetH, scale)
	}

	graySmall := gocv.NewMat()
	defer graySmall.Close()
	gocv.CvtColor(detectionImg, &graySmall, gocv.ColorBGRToGray)

	rectsSmall := w.faceDetector.Classifier.DetectMultiScale(graySmall)

	if len(rectsSmall) == 0 {
		return false, nil
	}

	var candidateRects []image.Rectangle
	if needResize {
		for _, r := range rectsSmall {
			x1 := int(float64(r.Min.X) / scale)
			y1 := int(float64(r.Min.Y) / scale)
			x2 := int(float64(r.Max.X) / scale)
			y2 := int(float64(r.Max.Y) / scale)
			candidateRects = append(candidateRects, image.Rect(x1, y1, x2, y2))
		}
	} else {
		candidateRects = rectsSmall
	}

	largestFace, found := w.findLargestValidFace(candidateRects)
	if !found {
		log.Printf("   ⚠️  All faces too small (min: %dpx)", w.faceDetector.Config.MinFaceSize)
		return false, nil
	}

	log.Printf("   👤 [Attempt %d/%d] Detected %d face(s), chosen area=%d",
		attemptNum, w.captureConfig.MaxAttempts, len(candidateRects), largestFace.Dx()*largestFace.Dy())

	expandedFace := w.expandAndCenterFace(largestFace, origW, origH)
	croppedFace := img.Region(expandedFace)
	defer croppedFace.Close()

	finalSquare := w.makeSquare(croppedFace)
	defer finalSquare.Close()

	base64Img, err := w.encodeImageToBase64(finalSquare)
	if err != nil {
		log.Printf("   ⚠️  Encode failed: %v", err)
		return true, nil
	}

	response, _ := w.faceDetector.SubmitSingleImageToAPI(base64Img, userId, attemptNum)
	return true, response
}

func (w *WebRTCManager) findLargestValidFace(rects []image.Rectangle) (image.Rectangle, bool) {
	var largestFace image.Rectangle
	maxArea := 0

	for _, rect := range rects {
		area := rect.Dx() * rect.Dy()
		if area > maxArea &&
			rect.Dx() >= w.faceDetector.Config.MinFaceSize &&
			rect.Dy() >= w.faceDetector.Config.MinFaceSize {
			maxArea = area
			largestFace = rect
		}
	}

	return largestFace, maxArea > 0
}
