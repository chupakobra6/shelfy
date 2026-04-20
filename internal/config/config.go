package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	BotToken           string
	DatabaseURL        string
	LogLevel           string
	Env                string
	CopyRuntimePath    string
	DefaultTimezone    string
	DigestLocalTime    string
	PollTimeoutSeconds int
	JobPollInterval    time.Duration
	SchedulerInterval  time.Duration
	TmpDir             string
	OllamaBaseURL      string
	OllamaModel        string
	TesseractCommand   string
	WhisperCommand     string
	WhisperModelPath   string
	EnableDevControl   bool
	DevControlAddr     string
}

func Load() (Config, error) {
	cfg := Config{
		BotToken:           os.Getenv("SHELFY_BOT_TOKEN"),
		DatabaseURL:        os.Getenv("SHELFY_DATABASE_URL"),
		LogLevel:           defaultString(os.Getenv("SHELFY_LOG_LEVEL"), "INFO"),
		Env:                defaultString(os.Getenv("SHELFY_ENV"), "development"),
		CopyRuntimePath:    defaultString(os.Getenv("SHELFY_COPY_RUNTIME_PATH"), "copy/runtime.ru.yaml"),
		DefaultTimezone:    defaultString(os.Getenv("SHELFY_DEFAULT_TIMEZONE"), "Europe/Moscow"),
		DigestLocalTime:    defaultString(os.Getenv("SHELFY_DIGEST_LOCAL_TIME"), "09:00"),
		PollTimeoutSeconds: defaultInt(os.Getenv("SHELFY_POLL_TIMEOUT_SECONDS"), 30),
		JobPollInterval:    defaultDuration(os.Getenv("SHELFY_JOB_POLL_INTERVAL"), 2*time.Second),
		SchedulerInterval:  defaultDuration(os.Getenv("SHELFY_SCHEDULER_INTERVAL"), 30*time.Second),
		TmpDir:             defaultString(os.Getenv("SHELFY_TMP_DIR"), "/tmp/shelfy"),
		OllamaBaseURL:      strings.TrimRight(defaultString(os.Getenv("SHELFY_OLLAMA_BASE_URL"), "http://127.0.0.1:11434"), "/"),
		OllamaModel:        defaultString(os.Getenv("SHELFY_OLLAMA_MODEL"), "gemma3:4b"),
		TesseractCommand:   defaultString(os.Getenv("SHELFY_TESSERACT_COMMAND"), "tesseract"),
		WhisperCommand:     defaultString(os.Getenv("SHELFY_WHISPER_COMMAND"), "whisper-cli"),
		WhisperModelPath:   defaultString(os.Getenv("SHELFY_WHISPER_MODEL_PATH"), ""),
		EnableDevControl:   defaultBool(os.Getenv("SHELFY_ENABLE_DEV_CONTROL_API"), true),
		DevControlAddr:     defaultString(os.Getenv("SHELFY_DEV_CONTROL_LISTEN_ADDR"), ":8081"),
	}

	if cfg.BotToken == "" {
		return Config{}, fmt.Errorf("SHELFY_BOT_TOKEN is required")
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("SHELFY_DATABASE_URL is required")
	}
	return cfg, nil
}

func (c Config) IsProduction() bool {
	return strings.EqualFold(c.Env, "production")
}

func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func defaultInt(v string, fallback int) int {
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func defaultBool(v string, fallback bool) bool {
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func defaultDuration(v string, fallback time.Duration) time.Duration {
	if v == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return parsed
}
