package models

// ============================================================
// MESSAGE CONTENT STRUCTURES
// ============================================================

type ChannelMessageContent struct {
	T             string                    `json:"t,omitempty"`
	ContentThread string                    `json:"contentThread,omitempty"`
	Embed         []InteractiveMessageEmbed `json:"embed,omitempty"`
	Components    []MessageComponent        `json:"components,omitempty"`
	// Add other fields as needed: hg, ej, lk, mk, vk
}

type InteractiveMessageEmbed struct {
	Color       string       `json:"color,omitempty"`
	Title       string       `json:"title,omitempty"`
	URL         string       `json:"url,omitempty"`
	Author      *EmbedAuthor `json:"author,omitempty"`
	Description string       `json:"description,omitempty"`
	Thumbnail   *EmbedImage  `json:"thumbnail,omitempty"`
	Fields      []EmbedField `json:"fields,omitempty"`
	Image       *EmbedImage  `json:"image,omitempty"`
	Timestamp   string       `json:"timestamp,omitempty"`
	Footer      *EmbedFooter `json:"footer,omitempty"`
}

type EmbedAuthor struct {
	Name    string `json:"name"`
	IconURL string `json:"icon_url,omitempty"`
	URL     string `json:"url,omitempty"`
}

type EmbedImage struct {
	URL    string `json:"url"`
	Width  string `json:"width,omitempty"`
	Height string `json:"height,omitempty"`
}

type EmbedField struct {
	Name       string        `json:"name"`
	Value      string        `json:"value"`
	Inline     bool          `json:"inline,omitempty"`
	Options    []interface{} `json:"options,omitempty"`
	Inputs     interface{}   `json:"inputs,omitempty"`
	MaxOptions int           `json:"max_options,omitempty"`
}

type EmbedFooter struct {
	Text    string `json:"text"`
	IconURL string `json:"icon_url,omitempty"`
}

type MessageComponent struct {
	ID        string           `json:"id"`
	Type      int              `json:"type"` // 1 = Button, 2 = Select, etc.
	Component ComponentDetails `json:"component"`
}

type ComponentDetails struct {
	Label       string   `json:"label,omitempty"`
	Style       int      `json:"style,omitempty"` // 1=Primary, 2=Secondary, 3=Success, 4=Danger
	CustomID    string   `json:"custom_id,omitempty"`
	Disabled    bool     `json:"disabled,omitempty"`
	Emoji       string   `json:"emoji,omitempty"`
	URL         string   `json:"url,omitempty"`
	Options     []string `json:"options,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
}

// ============================================================
// DM MESSAGE REQUEST
// ============================================================

type SendDMRequest struct {
	ClanID      string                `json:"clan_id"`
	ChannelID   string                `json:"channel_id"`
	Mode        int                   `json:"mode"` // 2 = DM mode
	IsPublic    bool                  `json:"is_public"`
	Content     ChannelMessageContent `json:"content"`
	Attachments []interface{}         `json:"attachments,omitempty"`
	Code        int                   `json:"code,omitempty"`
}
