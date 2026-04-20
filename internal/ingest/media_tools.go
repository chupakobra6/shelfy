package ingest

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/igor/shelfy/internal/observability"
)

func (s *Service) runTesseract(ctx context.Context, imagePath string) (string, error) {
	startedAt := time.Now()
	cmd := exec.CommandContext(ctx, s.tesseractCommand, imagePath, "stdout", "-l", "rus+eng", "--psm", "6")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	stdoutText := strings.TrimSpace(stdout.String())
	stderrText := strings.TrimSpace(stderr.String())
	cleaned := cleanOCRText(stdoutText)
	s.logger.InfoContext(ctx, "tesseract_completed", observability.LogAttrs(ctx,
		"duration_ms", time.Since(startedAt).Milliseconds(),
		"stdout_len", len(stdoutText),
		"stderr_len", len(stderrText),
		"text_len", len(cleaned),
		"text_excerpt", excerptForLog(cleaned, 320),
		"stderr_excerpt", excerptForLog(stderrText, 320),
	)...)
	if err != nil {
		return cleaned, fmt.Errorf("tesseract: %w: %s", err, stderrText)
	}
	if cleaned == "" && stderrText != "" {
		return "", fmt.Errorf("tesseract produced no OCR text: %s", stderrText)
	}
	return cleaned, nil
}

func (s *Service) runFFmpeg(ctx context.Context, inputPath, outputPath string) error {
	startedAt := time.Now()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", inputPath, outputPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg: %w: %s", err, string(output))
	}
	s.logger.InfoContext(ctx, "ffmpeg_completed", observability.LogAttrs(ctx, "duration_ms", time.Since(startedAt).Milliseconds())...)
	return nil
}

func (s *Service) runWhisper(ctx context.Context, wavPath string) (string, error) {
	if strings.TrimSpace(s.whisperModelPath) == "" {
		return "", fmt.Errorf("whisper model path is empty")
	}
	startedAt := time.Now()
	outputPrefix := strings.TrimSuffix(wavPath, filepath.Ext(wavPath)) + "-transcript"
	cmd := exec.CommandContext(ctx, s.whisperCommand, "-m", s.whisperModelPath, "-f", wavPath, "-otxt", "-of", outputPrefix)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("whisper: %w: %s", err, string(output))
	}
	body, err := os.ReadFile(outputPrefix + ".txt")
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(body))
	s.logger.InfoContext(ctx, "whisper_completed", observability.LogAttrs(ctx,
		"duration_ms", time.Since(startedAt).Milliseconds(),
		"text_len", len(text),
		"model_path", s.whisperModelPath,
		"text_excerpt", excerptForLog(text, 320),
	)...)
	return text, nil
}
