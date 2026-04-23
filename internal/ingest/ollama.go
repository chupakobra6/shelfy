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

type ollamaReviewResponse struct {
	CleanedInput string `json:"cleaned_input"`
	ReasonCode   string `json:"reason_code"`
}

var ollamaHTTPClient = &http.Client{Timeout: 90 * time.Second}

func (s *Service) callOllamaTextCleaner(ctx context.Context, normalizedText string) (cleanerOutput, error) {
	prompt := buildTextCleanerPrompt(normalizedText)
	parsed, err := s.callOllamaCleaner(ctx, "text_cleaner", prompt)
	if err != nil {
		return cleanerOutput{}, err
	}
	return parsed, nil
}

func (s *Service) callOllamaVoiceCleaner(ctx context.Context, normalizedTranscript string) (cleanerOutput, error) {
	prompt := buildVoiceCleanerPrompt(normalizedTranscript)
	parsed, err := s.callOllamaCleaner(ctx, "voice_cleaner", prompt)
	if err != nil {
		return cleanerOutput{}, err
	}
	return parsed, nil
}

func (s *Service) callOllamaCleaner(ctx context.Context, mode, prompt string) (cleanerOutput, error) {
	if strings.TrimSpace(s.ollamaBaseURL) == "" {
		return cleanerOutput{}, fmt.Errorf("ollama disabled")
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
		return cleanerOutput{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.ollamaBaseURL+"/api/generate", bytes.NewReader(encoded))
	if err != nil {
		return cleanerOutput{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := ollamaHTTPClient.Do(req)
	if err != nil {
		return cleanerOutput{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return cleanerOutput{}, fmt.Errorf("ollama cleaner returned %s: %s", resp.Status, string(body))
	}
	var response struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return cleanerOutput{}, err
	}
	var parsed ollamaReviewResponse
	if err := json.Unmarshal([]byte(response.Response), &parsed); err != nil {
		return cleanerOutput{}, err
	}
	result := cleanerOutput{
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
	return "You clean a normalized Russian tracking input. Return compact JSON with keys cleaned_input and reason_code.\n\n" +
		"Goal:\n" +
		"- Return the shortest useful entity from the input.\n" +
		"- Keep the real date phrase when it is present.\n" +
		"- Prefer the first concrete entity when the input contains multiple things.\n" +
		"- Remove request words, filler, delivery/store chatter, brands when optional, quantities, package sizes, fatness, and similar noise.\n" +
		"- Normalize the entity into a natural product form when obvious.\n" +
		"- Keep fixed plural product names such as \"пельмени\".\n" +
		"- Do not invent products or dates.\n\n" +
		"Source=" + source + "\n\n" +
		"Examples:\n" +
		`1) input="по фарш до послезавтра"
output={"cleaned_input":"фарш до послезавтра","reason_code":"filler_cleanup"}

2) input="так у меня тут бананы до завтра"
output={"cleaned_input":"бананы до завтра","reason_code":"filler_cleanup"}

3) input="меня хлеб срок годности через неделю"
output={"cleaned_input":"хлеб через неделю","reason_code":"filler_cleanup"}

4) input="так так так молоко тут двадцать девятого"
output={"cleaned_input":"молоко двадцать девятого","reason_code":"filler_cleanup"}

5) input="закажи на дом молоко простоквашино два с половиной литра и жирностью один процент"
output={"cleaned_input":"молоко","reason_code":"entity_shortening"}

6) input="ложкарев полкилограмма пельмени"
output={"cleaned_input":"пельмени","reason_code":"entity_shortening"}

7) input="так у меня тут колбасы до завтра"
output={"cleaned_input":"колбаса до завтра","reason_code":"entity_shortening"}

8) input="закажи пожалуйста фэри для посуды"
output={"cleaned_input":"фэри","reason_code":"entity_shortening"}

9) input="нужно купить молоко домик в деревне и черный чай принцесса нури до пятницы"
output={"cleaned_input":"молоко до пятницы","reason_code":"first_entity"}

10) input="с доставкой продукты на дом килограмм яблок бананы два килограмма десяток яиц"
output={"cleaned_input":"яблоки","reason_code":"first_entity"}

11) input="закажи мне пожалуйста из магазина монетка две булки черного уральского хлеба"
output={"cleaned_input":"черный уральский хлеб","reason_code":"entity_shortening"}` + "\n"
}
