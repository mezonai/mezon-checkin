package webrtc

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"os/exec"
	"time"

	"gocv.io/x/gocv"
)

// ============================================================
// VP8 KEYFRAME DETECTION
// ============================================================

func isVP8Keyframe(frame []byte) bool {
	if len(frame) < 10 {
		return false
	}
	frameTag := uint32(frame[0]) | (uint32(frame[1]) << 8) | (uint32(frame[2]) << 16)
	isKey := (frameTag & 0x1) == 0
	if !isKey {
		return false
	}
	if len(frame) < 7 {
		return false
	}
	if frame[3] != 0x9d || frame[4] != 0x01 || frame[5] != 0x2a {
		return false
	}
	return true
}

// ============================================================
// VP8 DIMENSION EXTRACTION
// ============================================================

func getVP8KeyframeDims(frame []byte) (int, int, error) {
	if len(frame) < 10 {
		return 0, 0, fmt.Errorf("frame too small: %d bytes", len(frame))
	}

	frameTag := uint32(frame[0]) | (uint32(frame[1]) << 8) | (uint32(frame[2]) << 16)
	if (frameTag & 0x1) != 0 {
		return 0, 0, fmt.Errorf("not a keyframe (tag: 0x%x)", frameTag)
	}

	if frame[3] != 0x9d || frame[4] != 0x01 || frame[5] != 0x2a {
		return 0, 0, fmt.Errorf("invalid start code: %02x %02x %02x", frame[3], frame[4], frame[5])
	}

	width := (int(frame[6]) | (int(frame[7]) << 8)) & 0x3FFF
	height := (int(frame[8]) | (int(frame[9]) << 8)) & 0x3FFF

	if width == 0 || height == 0 {
		return 0, 0, fmt.Errorf("zero dimension: %dx%d", width, height)
	}

	if width > 3840 || height > 2160 {
		return 0, 0, fmt.Errorf("dimension too large: %dx%d", width, height)
	}

	return width, height, nil
}

// ============================================================
// OPTIMAL DECODE SIZE
// ============================================================

func (w *WebRTCManager) getOptimalDecodeSize(origWidth, origHeight int) (int, int) {
	maxW := w.dimensionConfig.MaxDecodeWidth
	maxH := w.dimensionConfig.MaxDecodeHeight

	if origWidth <= maxW && origHeight <= maxH {
		return origWidth, origHeight
	}

	scaleW := float64(maxW) / float64(origWidth)
	scaleH := float64(maxH) / float64(origHeight)

	scale := scaleW
	if scaleH < scaleW {
		scale = scaleH
	}

	newWidth := int(float64(origWidth) * scale)
	newHeight := int(float64(origHeight) * scale)

	// Round to even numbers
	newWidth = (newWidth / 2) * 2
	newHeight = (newHeight / 2) * 2

	if newWidth < 2 {
		newWidth = 2
	}
	if newHeight < 2 {
		newHeight = 2
	}

	return newWidth, newHeight
}

// ============================================================
// IVF DATA CREATION
// ============================================================

func (w *WebRTCManager) createIVFData(frameData []byte, width, height int) []byte {
	// IVF File Header (32 bytes)
	ivfHeader := make([]byte, 32)

	copy(ivfHeader[0:4], []byte{'D', 'K', 'I', 'F'})
	ivfHeader[4] = 0
	ivfHeader[5] = 0
	ivfHeader[6] = 32
	ivfHeader[7] = 0
	copy(ivfHeader[8:12], []byte{'V', 'P', '8', '0'})
	ivfHeader[12] = byte(width & 0xff)
	ivfHeader[13] = byte((width >> 8) & 0xff)
	ivfHeader[14] = byte(height & 0xff)
	ivfHeader[15] = byte((height >> 8) & 0xff)
	ivfHeader[16] = 30
	ivfHeader[17] = 0
	ivfHeader[18] = 0
	ivfHeader[19] = 0
	ivfHeader[20] = 1
	ivfHeader[21] = 0
	ivfHeader[22] = 0
	ivfHeader[23] = 0
	ivfHeader[24] = 1
	ivfHeader[25] = 0
	ivfHeader[26] = 0
	ivfHeader[27] = 0

	// IVF Frame Header (12 bytes)
	frameSize := uint32(len(frameData))
	frameHeader := make([]byte, 12)

	frameHeader[0] = byte(frameSize & 0xff)
	frameHeader[1] = byte((frameSize >> 8) & 0xff)
	frameHeader[2] = byte((frameSize >> 16) & 0xff)
	frameHeader[3] = byte((frameSize >> 24) & 0xff)

	totalSize := 32 + 12 + len(frameData)
	result := make([]byte, 0, totalSize)
	result = append(result, ivfHeader...)
	result = append(result, frameHeader...)
	result = append(result, frameData...)

	return result
}

// ============================================================
// VP8 TO GOCV MAT
// ============================================================

func (w *WebRTCManager) vp8FrameToGoCV(frameData []byte) (*gocv.Mat, error) {
	origWidth, origHeight, err := getVP8KeyframeDims(frameData)
	if err != nil {
		return nil, fmt.Errorf("parse dims: %w", err)
	}

	if origWidth <= 0 || origHeight <= 0 {
		return nil, fmt.Errorf("invalid dims: %dx%d", origWidth, origHeight)
	}

	decodeWidth, decodeHeight := w.getOptimalDecodeSize(origWidth, origHeight)
	ivfData := w.createIVFData(frameData, origWidth, origHeight)

	// Build ffmpeg args
	args := []string{
		"-loglevel", "error",
		"-nostdin",
		"-f", "ivf",
		"-i", "pipe:0",
	}

	if decodeWidth != origWidth || decodeHeight != origHeight {
		args = append(args,
			"-vf", fmt.Sprintf("scale=%d:%d:flags=fast_bilinear", decodeWidth, decodeHeight),
		)
	}

	args = append(args,
		"-frames:v", "1",
		"-f", "rawvideo",
		"-pix_fmt", "bgr24",
		"-threads", "1",
		"pipe:1",
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	// Get buffer from pool
	buf := w.bufferPool.Get()
	defer func() {
		// Don't pool huge buffers
		if buf.Cap() > maxPooledBufferSize {
			return
		}
		w.bufferPool.Put(buf)
	}()

	var stderrBuf bytes.Buffer
	cmd.Stdout = buf
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ffmpeg start: %w", err)
	}

	// Write IVF data
	writeErr := make(chan error, 1)
	go func() {
		defer stdin.Close()
		if _, err := stdin.Write(ivfData); err != nil {
			writeErr <- fmt.Errorf("write: %w", err)
			return
		}
		writeErr <- nil
	}()

	// Wait for command
	cmdErr := cmd.Wait()

	if err := <-writeErr; err != nil {
		return nil, err
	}

	if cmdErr != nil {
		stderr := stderrBuf.String()
		if len(stderr) > 200 {
			stderr = stderr[:200] + "..."
		}
		return nil, fmt.Errorf("decode: %w (%s)", cmdErr, stderr)
	}

	expectedSize := decodeWidth * decodeHeight * 3
	if buf.Len() < expectedSize {
		return nil, fmt.Errorf("short frame: %d < %d", buf.Len(), expectedSize)
	}

	// Copy data for buffer reuse
	frameBytes := make([]byte, expectedSize)
	copy(frameBytes, buf.Bytes()[:expectedSize])

	mat, err := gocv.NewMatFromBytes(
		decodeHeight,
		decodeWidth,
		gocv.MatTypeCV8UC3,
		frameBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("NewMatFromBytes: %w", err)
	}

	if mat.Empty() {
		mat.Close()
		return nil, fmt.Errorf("empty mat")
	}

	return &mat, nil
}

// ============================================================
// IMAGE PROCESSING
// ============================================================

func (w *WebRTCManager) makeSquare(mat gocv.Mat) gocv.Mat {
	cropWidth := mat.Cols()
	cropHeight := mat.Rows()

	if cropWidth == cropHeight {
		return mat.Clone()
	}

	maxSize := cropWidth
	if cropHeight > maxSize {
		maxSize = cropHeight
	}

	finalSquare := gocv.NewMatWithSize(maxSize, maxSize, mat.Type())
	offsetX := (maxSize - cropWidth) / 2
	offsetY := (maxSize - cropHeight) / 2

	roi := finalSquare.Region(image.Rect(offsetX, offsetY, offsetX+cropWidth, offsetY+cropHeight))
	mat.CopyTo(&roi)
	roi.Close()

	return finalSquare
}

func (w *WebRTCManager) expandAndCenterFace(face image.Rectangle, imgWidth, imgHeight int) image.Rectangle {
	expandRatio := w.dimensionConfig.ExpandRatio
	faceWidth := face.Dx()
	faceHeight := face.Dy()

	expandX := int(float64(faceWidth) * expandRatio)
	expandY := int(float64(faceHeight) * expandRatio)

	x1 := face.Min.X - expandX
	y1 := face.Min.Y - expandY
	x2 := face.Max.X + expandX
	y2 := face.Max.Y + expandY

	if x1 < 0 {
		x1 = 0
	}
	if y1 < 0 {
		y1 = 0
	}
	if x2 > imgWidth {
		x2 = imgWidth
	}
	if y2 > imgHeight {
		y2 = imgHeight
	}

	expandedWidth := x2 - x1
	expandedHeight := y2 - y1
	squareSize := expandedWidth
	if expandedHeight > squareSize {
		squareSize = expandedHeight
	}

	centerX := x1 + expandedWidth/2
	centerY := y1 + expandedHeight/2

	squareX1 := centerX - squareSize/2
	squareY1 := centerY - squareSize/2
	squareX2 := squareX1 + squareSize
	squareY2 := squareY1 + squareSize

	if squareX1 < 0 {
		squareX1 = 0
		squareX2 = squareSize
		if squareX2 > imgWidth {
			squareX2 = imgWidth
		}
	}
	if squareY1 < 0 {
		squareY1 = 0
		squareY2 = squareSize
		if squareY2 > imgHeight {
			squareY2 = imgHeight
		}
	}
	if squareX2 > imgWidth {
		squareX2 = imgWidth
		squareX1 = imgWidth - squareSize
		if squareX1 < 0 {
			squareX1 = 0
		}
	}
	if squareY2 > imgHeight {
		squareY2 = imgHeight
		squareY1 = imgHeight - squareSize
		if squareY1 < 0 {
			squareY1 = 0
		}
	}

	return image.Rect(squareX1, squareY1, squareX2, squareY2)
}

func (w *WebRTCManager) encodeImageToBase64(mat gocv.Mat) (string, error) {
	imgGo, err := mat.ToImage()
	if err != nil {
		buf, err := gocv.IMEncode(gocv.JPEGFileExt, mat)
		if err != nil {
			return "", fmt.Errorf("IMEncode failed: %w", err)
		}
		defer buf.Close()
		return base64.StdEncoding.EncodeToString(buf.GetBytes()), nil
	}

	buf := w.bufferPool.Get()
	defer w.bufferPool.Put(buf)

	err = jpeg.Encode(buf, imgGo, &jpeg.Options{Quality: w.faceDetector.Config.JPEGQuality})
	if err != nil {
		return "", fmt.Errorf("jpeg encode failed: %w", err)
	}

	log.Printf("   📦 Image size: %.1fKB (quality: %d)",
		float64(buf.Len())/1024.0, w.faceDetector.Config.JPEGQuality)

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
