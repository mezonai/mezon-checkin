package webrtc

import (
	"bytes"
	"sync"
	"time"
)

// ============================================================
// DEFAULT CONFIGURATIONS
// ============================================================

func DefaultLocationConfig() LocationConfig {
	return LocationConfig{
		Enabled:         true,
		OfficesFilePath: "config/offices.json",
	}
}

func DefaultDimensionConfig() DimensionConfig {
	return DimensionConfig{
		MaxDecodeWidth:      640,
		MaxDecodeHeight:     480,
		DetectionWidth:      320,
		SkipDetectionResize: false,
		MinFaceSize:         80,
		ExpandRatio:         0.2,
	}
}

func DefaultCaptureConfig() CaptureConfig {
	return CaptureConfig{
		CaptureTimeout:  90 * time.Second,
		PLITimeout:      10 * time.Second,
		InitialRTPCount: 100,
		CaptureInterval: 1 * time.Second,
		MaxAttempts:     5,
		SampleBufferMax: 128,
	}
}

// ============================================================
// BUFFER POOL IMPLEMENTATION
// ============================================================

const (
	maxPooledBufferSize = 10 * 1024 * 1024 // 10MB
	initialBufferCap    = 512 * 1024       // 512KB
)

func newBufferPool() *bufferPool {
	return &bufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				buf := new(bytes.Buffer)
				buf.Grow(initialBufferCap)
				return buf
			},
		},
	}
}

func (p *bufferPool) Get() *bytes.Buffer {
	buf := p.pool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

func (p *bufferPool) Put(buf *bytes.Buffer) {
	if buf == nil {
		return
	}
	// Only pool buffers < 10MB to prevent memory bloat
	if buf.Cap() < maxPooledBufferSize {
		p.pool.Put(buf)
	}
}
