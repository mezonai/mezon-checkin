package models

import (
	"mezon-checkin-bot/mezon-protobuf/go/api"
	"mezon-checkin-bot/mezon-protobuf/go/rtapi"
)

// ============================================================
// WEBSOCKET MESSAGES
// ============================================================

type WebsocketMessage struct {
	CID                string                    `json:"cid,omitempty"`
	WebrtcSignalingFwd *rtapi.WebrtcSignalingFwd `json:"webrtc_signaling_fwd,omitempty"`
	UserChannelAdded   *rtapi.UserChannelAdded   `json:"user_channel_added_event,omitempty"`
	Ping               *rtapi.Ping               `json:"ping,omitempty"`
	Pong               *rtapi.Pong               `json:"pong,omitempty"`

	ChannelMessage *api.ChannelMessage `json:"channel_message,omitempty"`
}
