package utils

import (
	"fmt"
	"strings"
)

func PatchSDPForQuality(sdp string, asKBPS int, minKbps int, maxKbps int) string {
	lines := strings.Split(sdp, "\n")
	var out []string
	inVideo := false
	videoPayload := ""
	insertedFmtp := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		out = append(out, line)

		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "m=video") {
			inVideo = true
			insertedFmtp = false
			if asKBPS > 0 {
				out = append(out, fmt.Sprintf("b=AS:%d", asKBPS))
			}
			continue
		}

		if inVideo {
			if strings.HasPrefix(trim, "a=rtpmap:") && strings.Contains(trim, "VP8/90000") {
				parts := strings.SplitN(strings.TrimPrefix(trim, "a=rtpmap:"), " ", 2)
				if len(parts) >= 1 {
					videoPayload = strings.TrimSpace(parts[0])
				}
				if videoPayload != "" && !insertedFmtp && minKbps > 0 && maxKbps > 0 {
					startBitrate := (minKbps + maxKbps) / 2
					out = append(out, fmt.Sprintf("a=fmtp:%s x-google-min-bitrate=%d;x-google-max-bitrate=%d;x-google-start-bitrate=%d;max-fr=30;max-fs=3600",
						videoPayload, minKbps, maxKbps, startBitrate))
					insertedFmtp = true
				}
			}

			if strings.HasPrefix(trim, "m=") && !strings.HasPrefix(trim, "m=video") {
				inVideo = false
			}
		}
	}

	return strings.Join(out, "\n")
}
