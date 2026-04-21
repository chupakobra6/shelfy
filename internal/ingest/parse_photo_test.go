package ingest

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHasMeaningfulOCRSignal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "empty", input: "", want: false},
		{name: "punctuation only", input: "-- ...", want: false},
		{name: "short latin noise", input: "ee", want: false},
		{name: "short latin noise with date", input: "ee 21.04.2026", want: false},
		{name: "russian name with relative date", input: "сметана завтра", want: true},
		{name: "latin label with numeric date", input: "MILK 21.04.2026", want: true},
		{name: "date context words", input: "срок годен", want: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := hasMeaningfulOCRSignal(cleanOCRText(tc.input)); got != tc.want {
				t.Fatalf("hasMeaningfulOCRSignal(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestParsePhotoDraftRejectsLowSignalOCRAndSkipsModels(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		ocrText string
	}{
		{name: "blank", ocrText: ""},
		{name: "noise", ocrText: "ee"},
		{name: "punctuation", ocrText: "-- ..."},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stub := newOllamaTestServer(t, `{"name":"Milk","raw_deadline_phrase":"tomorrow"}`, `{"name":"Milk","raw_deadline_phrase":"tomorrow"}`)
			defer stub.Close()

			service := newPhotoTestService(stub.URL())
			imagePath := writePhotoTestImage(t)
			_, err := service.parsePhotoDraft(context.Background(), "", tc.ocrText, now, imagePath)
			if err == nil {
				t.Fatalf("expected parsePhotoDraft to fail for %q", tc.ocrText)
			}
			if stub.TextCalls() != 0 {
				t.Fatalf("expected no text model calls, got %d", stub.TextCalls())
			}
			if stub.VisionCalls() != 0 {
				t.Fatalf("expected no vision model calls, got %d", stub.VisionCalls())
			}
		})
	}
}

func TestParsePhotoDraftAcceptsMeaningfulRussianOCRWithoutVision(t *testing.T) {
	t.Parallel()

	service := newPhotoTestService("")
	now := time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC)

	draft, err := service.parsePhotoDraft(context.Background(), "", "сметана завтра", now, "")
	if err != nil {
		t.Fatalf("parsePhotoDraft returned error: %v", err)
	}
	if draft.Name != "сметана" {
		t.Fatalf("expected name сметана, got %q", draft.Name)
	}
	if draft.ExpiresOn == nil || draft.ExpiresOn.Format("2006-01-02") != "2026-04-22" {
		t.Fatalf("expected expiry 2026-04-22, got %#v", draft.ExpiresOn)
	}
}

func TestParsePhotoDraftRejectsUnsupportedEnglishVisionGuess(t *testing.T) {
	t.Parallel()

	stub := newOllamaTestServer(t, `{"name":"","raw_deadline_phrase":""}`, `{"name":"Milk","raw_deadline_phrase":"soon"}`)
	defer stub.Close()

	service := newPhotoTestService(stub.URL())
	now := time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC)
	imagePath := writePhotoTestImage(t)

	_, err := service.parsePhotoDraft(context.Background(), "", "срок годен", now, imagePath)
	if err == nil {
		t.Fatalf("expected unsupported english vision guess to be rejected")
	}
	if stub.TextCalls() != 1 {
		t.Fatalf("expected one text call, got %d", stub.TextCalls())
	}
	if stub.VisionCalls() != 1 {
		t.Fatalf("expected one vision call, got %d", stub.VisionCalls())
	}
}

func TestParsePhotoDraftAcceptsEnglishVisionNameWithOCROverlap(t *testing.T) {
	t.Parallel()

	stub := newOllamaTestServer(t, `{"name":"","raw_deadline_phrase":""}`, `{"name":"Milk","raw_deadline_phrase":"21.04.2026"}`)
	defer stub.Close()

	service := newPhotoTestService(stub.URL())
	now := time.Date(2026, time.April, 20, 12, 0, 0, 0, time.UTC)
	imagePath := writePhotoTestImage(t)

	draft, err := service.parsePhotoDraft(context.Background(), "", "MILK 21.04.2026", now, imagePath)
	if err != nil {
		t.Fatalf("parsePhotoDraft returned error: %v", err)
	}
	if !strings.EqualFold(draft.Name, "Milk") {
		t.Fatalf("expected OCR-supported english name, got %q", draft.Name)
	}
	if draft.ExpiresOn == nil || draft.ExpiresOn.Format("2006-01-02") != "2026-04-21" {
		t.Fatalf("expected expiry 2026-04-21, got %#v", draft.ExpiresOn)
	}
}

func TestParsePhotoDraftAllowsCaptionOnlyFullDraft(t *testing.T) {
	t.Parallel()

	stub := newOllamaTestServer(t, `{"name":"","raw_deadline_phrase":""}`, `{"name":"","raw_deadline_phrase":""}`)
	defer stub.Close()

	service := newPhotoTestService(stub.URL())
	now := time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC)
	imagePath := writePhotoTestImage(t)

	draft, err := service.parsePhotoDraft(context.Background(), "молоко до завтра", "", now, imagePath)
	if err != nil {
		t.Fatalf("parsePhotoDraft returned error: %v", err)
	}
	if draft.Name != "молоко" {
		t.Fatalf("expected caption-derived name, got %q", draft.Name)
	}
	if draft.ExpiresOn == nil || draft.ExpiresOn.Format("2006-01-02") != "2026-04-22" {
		t.Fatalf("expected expiry 2026-04-22, got %#v", draft.ExpiresOn)
	}
	if stub.TextCalls() != 0 || stub.VisionCalls() != 0 {
		t.Fatalf("expected caption-only draft to skip model calls, got text=%d vision=%d", stub.TextCalls(), stub.VisionCalls())
	}
}

func TestParsePhotoDraftUsesCaptionDateAsVisionAnchor(t *testing.T) {
	t.Parallel()

	stub := newOllamaTestServer(t, `{"name":"","raw_deadline_phrase":""}`, `{"name":"Молоко","raw_deadline_phrase":""}`)
	defer stub.Close()

	service := newPhotoTestService(stub.URL())
	now := time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC)
	imagePath := writePhotoTestImage(t)

	draft, err := service.parsePhotoDraft(context.Background(), "до завтра", "", now, imagePath)
	if err != nil {
		t.Fatalf("parsePhotoDraft returned error: %v", err)
	}
	if draft.Name != "Молоко" {
		t.Fatalf("expected anchored vision name, got %q", draft.Name)
	}
	if draft.ExpiresOn == nil || draft.ExpiresOn.Format("2006-01-02") != "2026-04-22" {
		t.Fatalf("expected expiry 2026-04-22, got %#v", draft.ExpiresOn)
	}
	if stub.TextCalls() != 0 {
		t.Fatalf("expected no text model calls, got %d", stub.TextCalls())
	}
	if stub.VisionCalls() != 1 {
		t.Fatalf("expected one anchored vision call, got %d", stub.VisionCalls())
	}
}

func TestParsePhotoDraftCaptionBeatsConflictingOCR(t *testing.T) {
	t.Parallel()

	service := newPhotoTestService("")
	now := time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC)

	draft, err := service.parsePhotoDraft(context.Background(), "молоко до завтра", "кефир 25.04.2026", now, "")
	if err != nil {
		t.Fatalf("parsePhotoDraft returned error: %v", err)
	}
	if draft.Name != "молоко" {
		t.Fatalf("expected caption name to win, got %q", draft.Name)
	}
	if draft.ExpiresOn == nil || draft.ExpiresOn.Format("2006-01-02") != "2026-04-22" {
		t.Fatalf("expected caption date to win, got %#v", draft.ExpiresOn)
	}
}

func TestParsePhotoDraftReceiptDateWithCaptionName(t *testing.T) {
	t.Parallel()

	service := newPhotoTestService("")
	now := time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC)
	ocr := "КАССОВЫЙ ЧЕК\nкефир 1\nгоден до 22.04.2026\nитог 120.00"

	draft, err := service.parsePhotoDraft(context.Background(), "кефир", ocr, now, "")
	if err != nil {
		t.Fatalf("parsePhotoDraft returned error: %v", err)
	}
	if draft.Name != "кефир" {
		t.Fatalf("expected caption name, got %q", draft.Name)
	}
	if draft.ExpiresOn == nil || draft.ExpiresOn.Format("2006-01-02") != "2026-04-22" {
		t.Fatalf("expected receipt-derived expiry 2026-04-22, got %#v", draft.ExpiresOn)
	}
}

func TestParsePhotoDraftReceiptWithoutCaptionDisambiguationFails(t *testing.T) {
	t.Parallel()

	service := newPhotoTestService("")
	now := time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC)
	ocr := "КАССОВЫЙ ЧЕК\nкефир 1\nмолоко 1\nгоден до 22.04.2026\nитог 220.00"

	if _, err := service.parsePhotoDraft(context.Background(), "", ocr, now, ""); err == nil {
		t.Fatalf("expected receipt without disambiguating caption to fail")
	}
}

func TestCallOllamaVisionPromptForbidsGuessing(t *testing.T) {
	t.Parallel()

	stub := newOllamaTestServer(t, `{"name":"","raw_deadline_phrase":""}`, `{"name":"","raw_deadline_phrase":""}`)
	defer stub.Close()

	service := newPhotoTestService(stub.URL())
	imagePath := writePhotoTestImage(t)

	if _, err := service.callOllamaVision(context.Background(), imagePath, "weak OCR", photoVisionModeStrictExtract, ""); err != nil {
		t.Fatalf("callOllamaVision returned error: %v", err)
	}

	prompt := strings.ToLower(stub.LastVisionPrompt())
	for _, want := range []string{
		"return empty strings",
		"never guess",
		"do not treat it as permission to invent missing fields",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q, got %q", want, prompt)
		}
	}
}

func TestCallOllamaVisionAnchoredAssistPromptMentionsExternalHint(t *testing.T) {
	t.Parallel()

	stub := newOllamaTestServer(t, `{"name":"","raw_deadline_phrase":""}`, `{"name":"","raw_deadline_phrase":""}`)
	defer stub.Close()

	service := newPhotoTestService(stub.URL())
	imagePath := writePhotoTestImage(t)

	if _, err := service.callOllamaVision(context.Background(), imagePath, "", photoVisionModeAnchoredNameAssist, "до завтра"); err != nil {
		t.Fatalf("callOllamaVision returned error: %v", err)
	}

	prompt := strings.ToLower(stub.LastVisionPrompt())
	for _, want := range []string{
		"already known from external text",
		"identify only the product name",
		"external deadline hint",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected anchored assist prompt to contain %q, got %q", want, prompt)
		}
	}
}

type ollamaTestServer struct {
	server           *httptest.Server
	textResponse     string
	visionResponse   string
	textCalls        int
	visionCalls      int
	lastVisionPrompt string
}

func newOllamaTestServer(t *testing.T, textResponse, visionResponse string) *ollamaTestServer {
	t.Helper()

	stub := &ollamaTestServer{
		textResponse:   textResponse,
		visionResponse: visionResponse,
	}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Prompt string   `json:"prompt"`
			Images []string `json:"images"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		response := stub.textResponse
		if len(req.Images) > 0 || strings.Contains(req.Prompt, "Inspect the product package image") {
			stub.visionCalls++
			stub.lastVisionPrompt = req.Prompt
			response = stub.visionResponse
		} else {
			stub.textCalls++
		}
		if strings.TrimSpace(response) == "" {
			response = `{"name":"","raw_deadline_phrase":""}`
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"response": response}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	return stub
}

func (s *ollamaTestServer) Close() {
	s.server.Close()
}

func (s *ollamaTestServer) URL() string {
	return s.server.URL
}

func (s *ollamaTestServer) TextCalls() int {
	return s.textCalls
}

func (s *ollamaTestServer) VisionCalls() int {
	return s.visionCalls
}

func (s *ollamaTestServer) LastVisionPrompt() string {
	return s.lastVisionPrompt
}

func newPhotoTestService(ollamaBaseURL string) *Service {
	return &Service{
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		ollamaBaseURL: ollamaBaseURL,
		ollamaModel:   "test-model",
	}
}

func writePhotoTestImage(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "input.jpg")
	if err := os.WriteFile(path, []byte("fake-image"), 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}
	return path
}
