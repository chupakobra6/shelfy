package ingest

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/igor/shelfy/internal/observability"
)

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

func (s *Service) runVosk(ctx context.Context, wavPath string) (string, error) {
	if strings.TrimSpace(s.voskModelPath) == "" {
		return "", fmt.Errorf("vosk model path is empty")
	}
	if strings.TrimSpace(s.voskCommand) == "" {
		return "", fmt.Errorf("vosk command is empty")
	}
	startedAt := time.Now()
	args := []string{s.voskModelPath, wavPath}
	if grammarPath := strings.TrimSpace(s.voskGrammarPath); grammarPath != "" {
		args = append(args, grammarPath)
	}
	cmd := exec.CommandContext(ctx, s.voskCommand, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("vosk: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	text := strings.TrimSpace(stdout.String())
	s.logger.InfoContext(ctx, "vosk_completed", observability.LogAttrs(ctx,
		"duration_ms", time.Since(startedAt).Milliseconds(),
		"text_len", len(text),
		"model_path", s.voskModelPath,
		"grammar_path", s.voskGrammarPath,
		"text_excerpt", excerptForLog(text, 320),
		"stderr_excerpt", excerptForLog(strings.TrimSpace(stderr.String()), 320),
	)...)
	return text, nil
}
