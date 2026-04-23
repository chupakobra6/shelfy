//go:build linux && cgo

package main

/*
#cgo CFLAGS: -I/usr/local/include
#cgo LDFLAGS: -L/usr/local/lib -lvosk -ldl -lpthread
#include <stdlib.h>
#include <vosk_api.h>
*/
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unsafe"
)

type recognitionResult struct {
	Text string `json:"text"`
}

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) != 3 && len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: vosk-transcribe <model-dir> <wav-path> [grammar-json-path]")
		return 2
	}
	grammarPath := ""
	if len(os.Args) == 4 {
		grammarPath = os.Args[3]
	}
	text, err := transcribe(os.Args[1], os.Args[2], grammarPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println(text)
	return 0
}

func transcribe(modelDir, wavPath, grammarPath string) (string, error) {
	reader, err := openPCM16MonoWAV(wavPath)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	C.vosk_set_log_level(C.int(-1))
	cModelDir := C.CString(modelDir)
	defer C.free(unsafe.Pointer(cModelDir))
	model := C.vosk_model_new(cModelDir)
	if model == nil {
		return "", fmt.Errorf("failed to load vosk model: %s", modelDir)
	}
	defer C.vosk_model_free(model)

	var recognizer *C.VoskRecognizer
	if strings.TrimSpace(grammarPath) != "" {
		grammarBytes, err := os.ReadFile(grammarPath)
		if err != nil {
			return "", fmt.Errorf("read grammar: %w", err)
		}
		grammar := strings.TrimSpace(string(grammarBytes))
		if grammar == "" {
			return "", fmt.Errorf("vosk grammar file is empty: %s", grammarPath)
		}
		cGrammar := C.CString(grammar)
		defer C.free(unsafe.Pointer(cGrammar))
		recognizer = C.vosk_recognizer_new_grm(model, C.float(reader.SampleRate()), cGrammar)
	} else {
		recognizer = C.vosk_recognizer_new(model, C.float(reader.SampleRate()))
	}
	if recognizer == nil {
		return "", errors.New("failed to create vosk recognizer")
	}
	defer C.vosk_recognizer_free(recognizer)

	buf := make([]byte, 8000)
	parts := make([]string, 0, 8)
	for {
		n, err := reader.ReadChunk(buf)
		if n > 0 {
			accepted := C.vosk_recognizer_accept_waveform(
				recognizer,
				(*C.char)(unsafe.Pointer(&buf[0])),
				C.int(n),
			)
			if accepted != 0 {
				parts = append(parts, extractTranscript(C.vosk_recognizer_result(recognizer)))
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
	}
	parts = append(parts, extractTranscript(C.vosk_recognizer_final_result(recognizer)))
	return joinTranscript(parts), nil
}

func extractTranscript(raw *C.char) string {
	payload := strings.TrimSpace(C.GoString(raw))
	if payload == "" {
		return ""
	}
	var result recognitionResult
	if err := json.Unmarshal([]byte(payload), &result); err != nil {
		return ""
	}
	return strings.TrimSpace(result.Text)
}

func joinTranscript(parts []string) string {
	filtered := parts[:0]
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	return strings.Join(filtered, " ")
}
