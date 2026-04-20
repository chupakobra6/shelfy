package telegram

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data,omitempty"`
}

type Message struct {
	MessageID       int64     `json:"message_id"`
	MessageThreadID int64     `json:"message_thread_id,omitempty"`
	From            *User     `json:"from,omitempty"`
	Chat            Chat      `json:"chat"`
	Text            string    `json:"text,omitempty"`
	Caption         string    `json:"caption,omitempty"`
	Photo           []Photo   `json:"photo,omitempty"`
	Voice           *Voice    `json:"voice,omitempty"`
	Audio           *Audio    `json:"audio,omitempty"`
	Sticker         *Sticker  `json:"sticker,omitempty"`
	Document        *Document `json:"document,omitempty"`
}

type User struct {
	ID       int64  `json:"id"`
	IsBot    bool   `json:"is_bot,omitempty"`
	Username string `json:"username,omitempty"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type Photo struct {
	FileID   string `json:"file_id"`
	FileSize int    `json:"file_size,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
}

type Voice struct {
	FileID string `json:"file_id"`
}

type Audio struct {
	FileID string `json:"file_id"`
}

type Sticker struct{}

type Document struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
}

type SendMessageRequest struct {
	ChatID      int64                 `json:"chat_id"`
	Text        string                `json:"text"`
	ParseMode   string                `json:"parse_mode,omitempty"`
	ReplyMarkup *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

type EditMessageTextRequest struct {
	ChatID      int64                 `json:"chat_id"`
	MessageID   int64                 `json:"message_id"`
	Text        string                `json:"text"`
	ParseMode   string                `json:"parse_mode,omitempty"`
	ReplyMarkup *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

type AnswerCallbackQueryRequest struct {
	CallbackQueryID string `json:"callback_query_id"`
	Text            string `json:"text,omitempty"`
	ShowAlert       bool   `json:"show_alert,omitempty"`
}

type GetUpdatesResponse struct {
	OK     bool     `json:"ok"`
	Result []Update `json:"result"`
}

type SendMessageResponse struct {
	OK     bool    `json:"ok"`
	Result Message `json:"result"`
}

type BaseResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
}

type FileResponse struct {
	OK     bool `json:"ok"`
	Result struct {
		FilePath string `json:"file_path"`
	} `json:"result"`
}
