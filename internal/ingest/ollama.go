package ingest

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/igor/shelfy/internal/observability"
)

type ollamaDraft struct {
	Name              string `json:"name"`
	RawDeadlinePhrase string `json:"raw_deadline_phrase"`
}

type photoVisionMode string

const (
	photoVisionModeStrictExtract      photoVisionMode = "strict_extract"
	photoVisionModeAnchoredNameAssist photoVisionMode = "anchored_name_assist"
)

func (s *Service) callOllamaText(ctx context.Context, text string) (ollamaDraft, error) {
	if strings.TrimSpace(s.ollamaBaseURL) == "" {
		return ollamaDraft{}, fmt.Errorf("ollama disabled")
	}
	if strings.TrimSpace(text) == "" {
		return ollamaDraft{}, fmt.Errorf("ollama text input is empty")
	}
	prompt := "Extract a food product name and a raw deadline phrase from the user text. Return compact JSON with keys name and raw_deadline_phrase. Do not invent facts."
	return s.callOllama(ctx, "text", prompt, text, nil)
}

func (s *Service) callOllamaVision(ctx context.Context, imagePath string, ocrHint string, mode photoVisionMode, anchorHint string) (ollamaDraft, error) {
	if strings.TrimSpace(s.ollamaBaseURL) == "" {
		return ollamaDraft{}, fmt.Errorf("ollama disabled")
	}
	prompt := "Inspect the product package image and return compact JSON with keys name and raw_deadline_phrase. Extract only text that is actually readable on the image. If there is no clearly readable product name or expiry wording/date, return empty strings for both fields. Never guess from packaging, colors, brand style, shape, object type, or context."
	if mode == photoVisionModeAnchoredNameAssist {
		prompt = "Inspect the product package image and return compact JSON with keys name and raw_deadline_phrase. A reliable deadline hint is already known from external text, so your job is to identify only the product name if it is clearly visible on the package. Leave raw_deadline_phrase empty unless the exact same deadline text is clearly readable on the image. If the product name is not clearly visible, return empty strings. Never guess from packaging colors, brand style, shape, object type, or context."
		if strings.TrimSpace(anchorHint) != "" {
			prompt += "\nExternal deadline hint:\n" + anchorHint
		}
	}
	if strings.TrimSpace(ocrHint) != "" {
		prompt += "\nUse this OCR text only as a noisy hint. Do not treat it as permission to invent missing fields. If both the OCR and image are unclear, return empty strings:\n" + ocrHint
	}
	raw, err := os.ReadFile(imagePath)
	if err != nil {
		return ollamaDraft{}, err
	}
	image := base64.StdEncoding.EncodeToString(raw)
	return s.callOllama(ctx, "vision", prompt, "Image-based product extraction.", []string{image})
}

func (s *Service) callOllama(ctx context.Context, mode, prompt, input string, images []string) (ollamaDraft, error) {
	startedAt := time.Now()
	s.logger.InfoContext(ctx, "ollama_call_started", observability.LogAttrs(ctx,
		"mode", mode,
		"model", s.ollamaModel,
		"has_images", len(images) > 0,
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
	if len(images) > 0 {
		body["images"] = images
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
	resp, err := http.DefaultClient.Do(req)
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
		"has_images", len(images) > 0,
		"has_name", strings.TrimSpace(draft.Name) != "",
		"has_deadline_phrase", strings.TrimSpace(draft.RawDeadlinePhrase) != "",
		"response_excerpt", excerptForLog(response.Response, 280),
	)...)
	return draft, nil
}
