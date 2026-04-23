package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/igor/shelfy/internal/observability"
)

type ollamaDraft struct {
	Name              string `json:"name"`
	RawDeadlinePhrase string `json:"raw_deadline_phrase"`
}

type ollamaReviewResponse struct {
	CleanedInput string `json:"cleaned_input"`
	ReasonCode   string `json:"reason_code"`
}

var ollamaHTTPClient = &http.Client{Timeout: 90 * time.Second}

func (s *Service) callOllamaText(ctx context.Context, text string) (ollamaDraft, error) {
	if strings.TrimSpace(s.ollamaBaseURL) == "" {
		return ollamaDraft{}, fmt.Errorf("ollama disabled")
	}
	if strings.TrimSpace(text) == "" {
		return ollamaDraft{}, fmt.Errorf("ollama text input is empty")
	}
	prompt := "Extract at most one food product name and one raw deadline phrase from the user text. The input is usually Russian, so keep Russian product names and Russian date phrases in Russian when the text is Russian. Ignore wake words, request verbs, quantities, package sizes, fat percentages, delivery/logistics details, and filler words. If the text mentions multiple different products or is not a food product tracking request, return empty strings for both fields. Put into raw_deadline_phrase only the smallest actual date-like or expiry-like phrase from the text. raw_deadline_phrase must contain only a date or expiry phrase, never a product name, quantity, package size, delivery/logistics text, or any other non-date fragment. Keep the raw deadline phrase exactly as written when it is present. Return compact JSON with keys name and raw_deadline_phrase. Do not invent facts."
	return s.callOllama(ctx, "text", prompt, text)
}

func (s *Service) callOllamaTextCleaner(ctx context.Context, normalizedText string) (reviewCleaner, error) {
	prompt := buildTextCleanerPrompt(normalizedText)
	parsed, err := s.callOllamaCleaner(ctx, "text_cleaner", prompt)
	if err != nil {
		return reviewCleaner{}, err
	}
	return parsed, nil
}

func (s *Service) callOllamaVoiceCleaner(ctx context.Context, normalizedTranscript string) (reviewCleaner, error) {
	prompt := buildVoiceCleanerPrompt(normalizedTranscript)
	parsed, err := s.callOllamaCleaner(ctx, "voice_cleaner", prompt)
	if err != nil {
		return reviewCleaner{}, err
	}
	return parsed, nil
}

func (s *Service) callOllama(ctx context.Context, mode, prompt, input string) (ollamaDraft, error) {
	startedAt := time.Now()
	s.logger.InfoContext(ctx, "ollama_call_started", observability.LogAttrs(ctx,
		"mode", mode,
		"model", s.ollamaModel,
		"input_len", len(strings.TrimSpace(input)),
		"input_excerpt", excerptForLog(input, 320),
	)...)
	body := map[string]any{
		"model":  s.ollamaModel,
		"prompt": prompt + "\n\nInput:\n" + input,
		"stream": false,
		"format": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":                map[string]any{"type": "string"},
				"raw_deadline_phrase": map[string]any{"type": "string"},
			},
			"required": []string{"name", "raw_deadline_phrase"},
		},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return ollamaDraft{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.ollamaBaseURL+"/api/generate", bytes.NewReader(encoded))
	if err != nil {
		return ollamaDraft{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := ollamaHTTPClient.Do(req)
	if err != nil {
		return ollamaDraft{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return ollamaDraft{}, fmt.Errorf("ollama returned %s: %s", resp.Status, string(body))
	}
	var response struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return ollamaDraft{}, err
	}
	var draft ollamaDraft
	if err := json.Unmarshal([]byte(response.Response), &draft); err != nil {
		return ollamaDraft{}, err
	}
	s.logger.InfoContext(ctx, "ollama_call_completed", observability.LogAttrs(ctx,
		"mode", mode,
		"duration_ms", time.Since(startedAt).Milliseconds(),
		"model", s.ollamaModel,
		"has_name", strings.TrimSpace(draft.Name) != "",
		"has_deadline_phrase", strings.TrimSpace(draft.RawDeadlinePhrase) != "",
		"response_excerpt", excerptForLog(response.Response, 280),
	)...)
	return draft, nil
}

func (s *Service) callOllamaCleaner(ctx context.Context, mode, prompt string) (reviewCleaner, error) {
	if strings.TrimSpace(s.ollamaBaseURL) == "" {
		return reviewCleaner{}, fmt.Errorf("ollama disabled")
	}
	startedAt := time.Now()
	s.logger.InfoContext(ctx, "ollama_call_started", observability.LogAttrs(ctx,
		"mode", mode,
		"model", s.ollamaModel,
		"input_len", len(strings.TrimSpace(prompt)),
		"input_excerpt", excerptForLog(prompt, 320),
	)...)
	body := map[string]any{
		"model":  s.ollamaModel,
		"prompt": prompt,
		"stream": false,
		"format": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cleaned_input": map[string]any{"type": "string"},
				"reason_code":   map[string]any{"type": "string"},
			},
			"required": []string{"cleaned_input", "reason_code"},
		},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return reviewCleaner{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.ollamaBaseURL+"/api/generate", bytes.NewReader(encoded))
	if err != nil {
		return reviewCleaner{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := ollamaHTTPClient.Do(req)
	if err != nil {
		return reviewCleaner{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return reviewCleaner{}, fmt.Errorf("ollama cleaner returned %s: %s", resp.Status, string(body))
	}
	var response struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return reviewCleaner{}, err
	}
	var parsed ollamaReviewResponse
	if err := json.Unmarshal([]byte(response.Response), &parsed); err != nil {
		return reviewCleaner{}, err
	}
	result := reviewCleaner{
		CleanedInput: strings.TrimSpace(parsed.CleanedInput),
		ReasonCode:   strings.TrimSpace(parsed.ReasonCode),
	}
	s.logger.InfoContext(ctx, "ollama_call_completed", observability.LogAttrs(ctx,
		"mode", mode,
		"duration_ms", time.Since(startedAt).Milliseconds(),
		"model", s.ollamaModel,
		"cleaned_excerpt", excerptForLog(result.CleanedInput, 240),
		"reason_code", result.ReasonCode,
		"response_excerpt", excerptForLog(response.Response, 280),
	)...)
	return result, nil
}

func buildTextCleanerPrompt(normalizedText string) string {
	return cleanerPromptHeader("text") +
		"\nInput:\n" + strings.TrimSpace(normalizedText)
}

func buildVoiceCleanerPrompt(normalizedTranscript string) string {
	return cleanerPromptHeader("voice") +
		"\nInput:\n" + strings.TrimSpace(normalizedTranscript)
}

func cleanerPromptHeader(source string) string {
	return "You clean a Russian grocery tracking input. Return compact JSON with keys cleaned_input and reason_code.\n\n" +
		"Rules:\n" +
		"- Return only a cleaned version of the input. Do not extract fields.\n" +
		"- The input is already normalized text. Clean only that text.\n" +
		"- Remove obvious filler at the beginning or end: wake words, request verbs, discourse fillers, store/delivery chatter, quantities, package sizes, fatness, and similar noise.\n" +
		"- Keep the same single product and the same real date phrase when they are present.\n" +
		"- Do not invent products, brands, or dates.\n" +
		"- If the input is already clean, return it almost unchanged.\n\n" +
		"Source=" + source + "\n\n" +
		"Examples:\n" +
		`1) input="по фарш до послезавтра"
output={"cleaned_input":"фарш до послезавтра","reason_code":"filler_cleanup"}

2) input="так у меня один банан до завтра"
output={"cleaned_input":"банан до завтра","reason_code":"filler_cleanup"}

3) input="меня хлеб срок годности через неделю"
output={"cleaned_input":"хлеб через неделю","reason_code":"filler_cleanup"}

4) input="так так так молоко тут двадцать девятого"
output={"cleaned_input":"молоко двадцать девятого","reason_code":"filler_cleanup"}

5) input="ложкарев полкилограмма пельмени"
output={"cleaned_input":"пельмени ложкарев","reason_code":"quantity_cleanup"}

6) input="бананы тридцать"
output={"cleaned_input":"бананы","reason_code":"quantity_cleanup"}` + "\n"
}

func (s *Service) callOllamaTranscriptRepair(ctx context.Context, transcript string) (string, error) {
	if strings.TrimSpace(s.ollamaBaseURL) == "" {
		return "", fmt.Errorf("ollama disabled")
	}
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		return "", fmt.Errorf("transcript repair input is empty")
	}
	body := map[string]any{
		"model":  s.ollamaModel,
		"prompt": "Repair a noisy Russian ASR transcript conservatively. Keep only words that are strongly supported by the transcript. Return compact JSON with a single key transcript. Do not add explanations.\n\nInput:\n" + transcript,
		"stream": false,
		"format": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"transcript": map[string]any{"type": "string"},
			},
			"required": []string{"transcript"},
		},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.ollamaBaseURL+"/api/generate", bytes.NewReader(encoded))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := ollamaHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("ollama transcript repair returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var response struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", err
	}
	var parsed struct {
		Transcript string `json:"transcript"`
	}
	if err := json.Unmarshal([]byte(response.Response), &parsed); err != nil {
		return "", err
	}
	return strings.TrimSpace(parsed.Transcript), nil
}
