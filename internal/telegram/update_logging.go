package telegram

import (
	"context"
	"strings"

	"github.com/igor/shelfy/internal/observability"
)

func (c *Client) LogIncomingUpdate(ctx context.Context, update Update) {
	switch {
	case update.Message != nil:
		c.logIncomingMessage(ctx, *update.Message)
	case update.CallbackQuery != nil:
		c.logIncomingCallback(ctx, *update.CallbackQuery)
	default:
		c.logger.DebugContext(ctx, "telegram.incoming_update_ignored", observability.LogAttrs(ctx)...)
	}
}

func (c *Client) logIncomingMessage(ctx context.Context, message Message) {
	contentType := incomingContentType(message)
	c.logger.InfoContext(ctx, "telegram.incoming_message", observability.LogAttrs(ctx,
		"chat_id", message.Chat.ID,
		"chat_type", message.Chat.Type,
		"message_id", message.MessageID,
		"thread_id", zeroToNil(message.MessageThreadID),
		"user_id", userIDOrZero(message.From),
		"username", usernameOrEmpty(message.From),
		"text", truncateLogString(message.Text, 2000),
		"caption", truncateLogString(message.Caption, 2000),
		"content_type", contentType,
		"photo_count", len(message.Photo),
		"voice_file_id", voiceFileID(message.Voice),
		"audio_file_id", audioFileID(message.Audio),
		"document_file_id", documentFileID(message.Document),
		"document_name", documentFileName(message.Document),
		"document_mime_type", documentMimeType(message.Document),
	)...)
}

func (c *Client) logIncomingCallback(ctx context.Context, callback CallbackQuery) {
	var chatID any
	var chatType any
	var messageID any
	var threadID any
	if callback.Message != nil {
		chatID = callback.Message.Chat.ID
		chatType = callback.Message.Chat.Type
		messageID = callback.Message.MessageID
		threadID = zeroToNil(callback.Message.MessageThreadID)
	}
	c.logger.InfoContext(ctx, "telegram.incoming_callback", observability.LogAttrs(ctx,
		"chat_id", chatID,
		"chat_type", chatType,
		"message_id", messageID,
		"thread_id", threadID,
		"user_id", callback.From.ID,
		"username", callback.From.Username,
		"data", truncateLogString(callback.Data, 1000),
	)...)
}

func incomingContentType(message Message) string {
	switch {
	case strings.TrimSpace(message.Text) != "":
		return "text"
	case strings.TrimSpace(message.Caption) != "":
		switch {
		case len(message.Photo) > 0:
			return "photo_caption"
		case message.Document != nil:
			return "document_caption"
		default:
			return "caption_only"
		}
	case len(message.Photo) > 0:
		return "photo"
	case message.Voice != nil:
		return "voice"
	case message.Audio != nil:
		return "audio"
	case message.Document != nil:
		return "document"
	case message.Sticker != nil:
		return "sticker"
	default:
		return "unknown"
	}
}

func truncateLogString(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}

func userIDOrZero(user *User) int64 {
	if user == nil {
		return 0
	}
	return user.ID
}

func usernameOrEmpty(user *User) string {
	if user == nil {
		return ""
	}
	return user.Username
}

func voiceFileID(voice *Voice) string {
	if voice == nil {
		return ""
	}
	return voice.FileID
}

func audioFileID(audio *Audio) string {
	if audio == nil {
		return ""
	}
	return audio.FileID
}

func documentFileID(document *Document) string {
	if document == nil {
		return ""
	}
	return document.FileID
}

func documentFileName(document *Document) string {
	if document == nil {
		return ""
	}
	return document.FileName
}

func documentMimeType(document *Document) string {
	if document == nil {
		return ""
	}
	return document.MimeType
}

func zeroToNil(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}
