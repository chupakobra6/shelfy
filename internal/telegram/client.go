package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/igor/shelfy/internal/observability"
)

type Client struct {
	token  string
	http   *http.Client
	logger *slog.Logger
}

func NewClient(token string, logger *slog.Logger) *Client {
	return &Client{
		token: token,
		http: &http.Client{
			Timeout: 75 * time.Second,
		},
		logger: logger,
	}
}

func (c *Client) PollUpdates(ctx context.Context, offset int64, timeoutSeconds int) ([]Update, error) {
	startedAt := time.Now()
	body := map[string]any{
		"offset":          offset,
		"timeout":         timeoutSeconds,
		"allowed_updates": []string{"message", "callback_query"},
	}
	var response GetUpdatesResponse
	if err := c.callJSON(ctx, "getUpdates", body, &response); err != nil {
		return nil, err
	}
	c.logger.DebugContext(ctx, "telegram_poll_completed", observability.LogAttrs(ctx,
		"offset", offset,
		"duration_ms", time.Since(startedAt).Milliseconds(),
		"update_count", len(response.Result),
	)...)
	return response.Result, nil
}

func (c *Client) SendMessage(ctx context.Context, request SendMessageRequest) (Message, error) {
	startedAt := time.Now()
	var response SendMessageResponse
	if err := c.callJSON(ctx, "sendMessage", request, &response); err != nil {
		return Message{}, err
	}
	c.logger.InfoContext(ctx, "telegram_send_message_completed", observability.LogAttrs(ctx,
		"chat_id", request.ChatID,
		"message_id", response.Result.MessageID,
		"duration_ms", time.Since(startedAt).Milliseconds(),
	)...)
	return response.Result, nil
}

func (c *Client) EditMessageText(ctx context.Context, request EditMessageTextRequest) error {
	startedAt := time.Now()
	var response BaseResponse
	if err := c.callJSON(ctx, "editMessageText", request, &response); err != nil {
		if isTelegramNotModifiedError(err) {
			c.logger.DebugContext(ctx, "telegram_edit_message_skipped_not_modified", observability.LogAttrs(ctx,
				"chat_id", request.ChatID,
				"message_id", request.MessageID,
				"duration_ms", time.Since(startedAt).Milliseconds(),
			)...)
			return nil
		}
		return err
	}
	c.logger.InfoContext(ctx, "telegram_edit_message_completed", observability.LogAttrs(ctx,
		"chat_id", request.ChatID,
		"message_id", request.MessageID,
		"duration_ms", time.Since(startedAt).Milliseconds(),
	)...)
	return nil
}

func (c *Client) DeleteMessage(ctx context.Context, chatID, messageID int64) error {
	startedAt := time.Now()
	var response BaseResponse
	if err := c.callJSON(ctx, "deleteMessage", map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
	}, &response); err != nil {
		if isTelegramMissingDeleteTarget(err) {
			c.logger.DebugContext(ctx, "telegram_delete_message_skipped_missing", observability.LogAttrs(ctx,
				"chat_id", chatID,
				"message_id", messageID,
				"duration_ms", time.Since(startedAt).Milliseconds(),
			)...)
			return nil
		}
		return err
	}
	c.logger.InfoContext(ctx, "telegram_delete_message_completed", observability.LogAttrs(ctx,
		"chat_id", chatID,
		"message_id", messageID,
		"duration_ms", time.Since(startedAt).Milliseconds(),
	)...)
	return nil
}

func (c *Client) PinMessage(ctx context.Context, chatID, messageID int64) error {
	startedAt := time.Now()
	var response BaseResponse
	if err := c.callJSON(ctx, "pinChatMessage", map[string]any{
		"chat_id":              chatID,
		"message_id":           messageID,
		"disable_notification": true,
	}, &response); err != nil {
		return err
	}
	c.logger.InfoContext(ctx, "telegram_pin_message_completed", observability.LogAttrs(ctx,
		"chat_id", chatID,
		"message_id", messageID,
		"duration_ms", time.Since(startedAt).Milliseconds(),
	)...)
	return nil
}

func (c *Client) AnswerCallbackQuery(ctx context.Context, request AnswerCallbackQueryRequest) error {
	startedAt := time.Now()
	var response BaseResponse
	if err := c.callJSON(ctx, "answerCallbackQuery", request, &response); err != nil {
		return err
	}
	c.logger.DebugContext(ctx, "telegram_answer_callback_completed", observability.LogAttrs(ctx,
		"duration_ms", time.Since(startedAt).Milliseconds(),
	)...)
	return nil
}

func (c *Client) GetFilePath(ctx context.Context, fileID string) (string, error) {
	startedAt := time.Now()
	var response FileResponse
	if err := c.callJSON(ctx, "getFile", map[string]any{"file_id": fileID}, &response); err != nil {
		return "", err
	}
	if response.Result.FilePath == "" {
		return "", fmt.Errorf("telegram file path is empty")
	}
	c.logger.InfoContext(ctx, "telegram_get_file_completed", observability.LogAttrs(ctx,
		"file_id", fileID,
		"duration_ms", time.Since(startedAt).Milliseconds(),
	)...)
	return response.Result.FilePath, nil
}

func (c *Client) DownloadFile(ctx context.Context, fileID, targetPath string) error {
	startedAt := time.Now()
	filePath, err := c.GetFilePath(ctx, fileID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.fileURL(filePath), nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return c.redactError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram file download failed: %s", resp.Status)
	}
	file, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer file.Close()
	written, err := io.Copy(file, resp.Body)
	if err != nil {
		return err
	}
	c.logger.InfoContext(ctx, "telegram_download_file_completed", observability.LogAttrs(ctx,
		"file_id", fileID,
		"target_path", targetPath,
		"bytes", written,
		"duration_ms", time.Since(startedAt).Milliseconds(),
	)...)
	return nil
}

func (c *Client) callJSON(ctx context.Context, method string, requestBody any, dest any) error {
	requestCtx, cancel := context.WithTimeout(ctx, c.requestTimeout(method))
	defer cancel()
	encoded, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, c.apiURL(method), bytes.NewReader(encoded))
	if err != nil {
		return c.redactError(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return c.redactError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("telegram %s failed: %s (%s)", method, resp.Status, string(body))
	}
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return err
	}
	switch typed := dest.(type) {
	case *BaseResponse:
		if !typed.OK {
			return fmt.Errorf("telegram %s not ok: %s", method, typed.Description)
		}
	case *GetUpdatesResponse:
		if !typed.OK {
			return fmt.Errorf("telegram %s not ok", method)
		}
	case *SendMessageResponse:
		if !typed.OK {
			return fmt.Errorf("telegram %s not ok", method)
		}
	case *FileResponse:
		if !typed.OK {
			return fmt.Errorf("telegram %s not ok", method)
		}
	}
	return nil
}

func (c *Client) requestTimeout(method string) time.Duration {
	switch method {
	case "getUpdates":
		return 70 * time.Second
	case "sendMessage", "editMessageText", "deleteMessage", "pinChatMessage", "answerCallbackQuery", "getFile":
		return 15 * time.Second
	default:
		return 30 * time.Second
	}
}

func (c *Client) apiURL(method string) string {
	return fmt.Sprintf("https://api.telegram.org/bot%s/%s", c.token, method)
}

func (c *Client) fileURL(path string) string {
	return fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", c.token, path)
}

func (c *Client) redactError(err error) error {
	if err == nil {
		return nil
	}
	return &redactedError{
		public: strings.ReplaceAll(err.Error(), c.token, "<redacted>"),
		cause:  err,
	}
}

func IsTransientPollError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "context deadline exceeded") ||
		strings.Contains(message, "client.timeout exceeded") ||
		strings.Contains(message, "unexpected eof") ||
		strings.Contains(message, "connection reset by peer") ||
		strings.Contains(message, "tls handshake timeout")
}

type redactedError struct {
	public string
	cause  error
}

func (e *redactedError) Error() string {
	return e.public
}

func (e *redactedError) Unwrap() error {
	return e.cause
}

func isTelegramNotModifiedError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "message is not modified")
}

func isTelegramMissingDeleteTarget(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "message to delete not found")
}
