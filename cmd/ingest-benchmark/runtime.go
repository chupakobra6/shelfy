package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type benchRuntime struct {
	repoRoot            string
	corpusDir           string
	cacheRoot           string
	assetCacheDir       string
	voiceCacheDir       string
	runtimeBaseImage    string
	pipelineWorkerImage string
	hostModelsDir       string
	hostVoskBinaryPath  string
	containerModelPath  string
	httpClient          *http.Client
}

func newBenchRuntime(repoRoot, corpusDir string, cfg runConfig) (*benchRuntime, error) {
	runtime := &benchRuntime{
		repoRoot:            repoRoot,
		corpusDir:           corpusDir,
		cacheRoot:           cfg.cacheDir,
		assetCacheDir:       filepath.Join(cfg.cacheDir, "assets"),
		voiceCacheDir:       filepath.Join(cfg.cacheDir, "voice"),
		runtimeBaseImage:    cfg.runtimeBaseImage,
		pipelineWorkerImage: cfg.pipelineWorkerImage,
		hostModelsDir:       cfg.modelsDir,
		hostVoskBinaryPath:  cfg.voskBinaryHostPath,
		containerModelPath:  cfg.voskModelPath,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute,
		},
	}
	for _, dir := range []string{
		runtime.cacheRoot,
		runtime.assetCacheDir,
		runtime.voiceCacheDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return runtime, nil
}

func (r *benchRuntime) prefetchAssets(ctx context.Context, suite corpus) error {
	for _, vc := range suite.Voice {
		if _, _, err := r.resolveVoicePaths(ctx, vc); err != nil {
			return err
		}
	}
	return nil
}

func (r *benchRuntime) resolveVoicePaths(ctx context.Context, vc voiceCase) (string, string, error) {
	originalPath := ""
	var err error
	switch {
	case strings.TrimSpace(vc.AssetPath) != "":
		originalPath, err = r.ensureLocalAsset(vc.AssetPath, vc.SHA256)
	case strings.TrimSpace(vc.DownloadURL) != "":
		originalPath, err = r.ensureDirectAsset(ctx, "voice", vc.DownloadURL, vc.SHA256)
	case strings.TrimSpace(vc.Dataset) != "":
		originalPath, err = r.ensureDatasetAsset(ctx, vc)
	default:
		err = fmt.Errorf("voice %s has no download_url or asset_path", vc.ID)
	}
	if err != nil {
		return "", "", err
	}
	key := shortHash(vc.DownloadURL + "|" + vc.SHA256)
	if strings.TrimSpace(vc.AssetPath) != "" {
		key = shortHash(vc.AssetPath + "|" + vc.SHA256)
	}
	wavPath := filepath.Join(r.voiceCacheDir, "wav", key+".wav")
	if fileExists(wavPath) {
		return originalPath, wavPath, nil
	}
	if err := os.MkdirAll(filepath.Dir(wavPath), 0o755); err != nil {
		return "", "", err
	}
	tempPath := strings.TrimSuffix(wavPath, ".wav") + ".part.wav"
	defer os.Remove(tempPath)
	args := []string{"-y", "-i", originalPath, "-ac", "1", "-ar", "16000", "-sample_fmt", "s16", "-f", "wav", tempPath}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("ffmpeg voice convert: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := os.Rename(tempPath, wavPath); err != nil {
		return "", "", err
	}
	return originalPath, wavPath, nil
}

func (r *benchRuntime) ensureDatasetAsset(ctx context.Context, vc voiceCase) (string, error) {
	switch strings.TrimSpace(vc.Dataset) {
	case "golos_crowd_commands":
		return r.ensureGolosAsset(ctx, vc)
	default:
		return "", fmt.Errorf("voice %s uses unsupported dataset resolver %q", vc.ID, vc.Dataset)
	}
}

func (r *benchRuntime) ensureLocalAsset(assetPath, expectedSHA string) (string, error) {
	resolved := assetPath
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(r.repoRoot, resolved)
	}
	if !fileExists(resolved) {
		return "", fmt.Errorf("asset path does not exist: %s", resolved)
	}
	return resolved, verifySHA256(resolved, expectedSHA)
}

func (r *benchRuntime) ensureDirectAsset(ctx context.Context, family, downloadURL, expectedSHA string) (string, error) {
	ext := normalizedExt(downloadURL)
	destPath := filepath.Join(r.assetCacheDir, family, shortHash(downloadURL)+ext)
	if fileExists(destPath) {
		return destPath, verifySHA256(destPath, expectedSHA)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return "", err
	}
	if err := r.downloadFile(ctx, downloadURL, destPath); err != nil {
		return "", err
	}
	return destPath, verifySHA256(destPath, expectedSHA)
}

func (r *benchRuntime) ensureGolosAsset(ctx context.Context, vc voiceCase) (string, error) {
	split, offset, err := parseDatasetRowLocator(vc.SourceID)
	if err != nil {
		return "", err
	}
	destPath := filepath.Join(r.assetCacheDir, "voice", "golos", fmt.Sprintf("%s-%06d.wav", split, offset))
	if fileExists(destPath) {
		return destPath, verifySHA256(destPath, vc.SHA256)
	}
	row, err := r.fetchGolosRow(ctx, split, offset)
	if err != nil {
		return "", err
	}
	if ref := strings.TrimSpace(vc.TranscriptRef); ref != "" && normalizedSpeechText(row.Transcription) != normalizedSpeechText(ref) {
		return "", fmt.Errorf("golos row %s transcript mismatch: got %q want %q", vc.SourceID, row.Transcription, ref)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return "", err
	}
	if err := r.downloadFile(ctx, row.AudioSrc, destPath); err != nil {
		return "", err
	}
	return destPath, verifySHA256(destPath, vc.SHA256)
}

type golosRow struct {
	Transcription string
	AudioSrc      string
}

func (r *benchRuntime) fetchGolosRow(ctx context.Context, split string, offset int) (golosRow, error) {
	url := fmt.Sprintf("https://datasets-server.huggingface.co/rows?dataset=bond005%%2Fsberdevices_golos_10h_crowd&config=default&split=%s&offset=%d&length=1", split, offset)
	backoff := time.Second
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return golosRow{}, err
		}
		req.Header.Set("User-Agent", "Shelfy-Ingest-Benchmark/1.0")
		resp, err := r.httpClient.Do(req)
		if err != nil {
			lastErr = err
		} else {
			row, retryable, err := decodeGolosRow(split, offset, resp)
			if err == nil {
				return row, nil
			}
			lastErr = err
			if !retryable {
				backoff = 0
			}
		}
		if attempt < 4 && backoff > 0 {
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return golosRow{}, ctx.Err()
			}
			backoff *= 2
		}
	}
	return golosRow{}, lastErr
}

func decodeGolosRow(split string, offset int, resp *http.Response) (golosRow, bool, error) {
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return golosRow{}, shouldRetryHTTPStatus(resp.StatusCode), fmt.Errorf("fetch golos row %s:%d returned %s: %s", split, offset, resp.Status, strings.TrimSpace(string(body)))
	}
	var payload struct {
		Rows []struct {
			Row struct {
				Audio []struct {
					Src string `json:"src"`
				} `json:"audio"`
				Transcription string `json:"transcription"`
			} `json:"row"`
		} `json:"rows"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return golosRow{}, false, err
	}
	if len(payload.Rows) != 1 || len(payload.Rows[0].Row.Audio) == 0 || strings.TrimSpace(payload.Rows[0].Row.Audio[0].Src) == "" {
		return golosRow{}, false, fmt.Errorf("golos row %s:%d returned no audio", split, offset)
	}
	return golosRow{
		Transcription: strings.TrimSpace(payload.Rows[0].Row.Transcription),
		AudioSrc:      strings.TrimSpace(payload.Rows[0].Row.Audio[0].Src),
	}, false, nil
}

func parseDatasetRowLocator(value string) (string, int, error) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid dataset row locator %q", value)
	}
	offset, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid dataset row locator %q: %w", value, err)
	}
	return parts[0], offset, nil
}

func normalizedSpeechText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "ё", "е")
	replacer := strings.NewReplacer(
		".", " ",
		",", " ",
		";", " ",
		":", " ",
		"!", " ",
		"?", " ",
		"(", " ",
		")", " ",
		"\"", " ",
		"'", " ",
		"-", " ",
	)
	value = replacer.Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

func (r *benchRuntime) downloadFile(ctx context.Context, downloadURL, destPath string) error {
	tempPath := destPath + ".part"
	defer os.Remove(tempPath)
	backoff := time.Second
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "Shelfy-Ingest-Benchmark/1.0")
		resp, err := r.httpClient.Do(req)
		if err != nil {
			lastErr = err
		} else {
			func() {
				defer resp.Body.Close()
				if resp.StatusCode >= 300 {
					body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
					lastErr = fmt.Errorf("download %s returned %s: %s", downloadURL, resp.Status, strings.TrimSpace(string(body)))
					return
				}
				file, err := os.Create(tempPath)
				if err != nil {
					lastErr = err
					return
				}
				if _, err := io.Copy(file, resp.Body); err != nil {
					file.Close()
					lastErr = err
					return
				}
				if err := file.Close(); err != nil {
					lastErr = err
					return
				}
				lastErr = nil
			}()
		}
		if lastErr == nil {
			return os.Rename(tempPath, destPath)
		}
		if attempt < 3 {
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
			backoff *= 2
		}
	}
	return lastErr
}

func shouldRetryHTTPStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

func (r *benchRuntime) runDockerVosk(ctx context.Context, wavPath, grammarPath string) (string, error) {
	if !fileExists(r.hostModelsDir) {
		return "", fmt.Errorf("models dir does not exist: %s", r.hostModelsDir)
	}
	args := []string{
		"run", "--rm",
		"-v", filepath.Dir(wavPath) + ":/bench:ro",
		"-v", r.hostModelsDir + ":/models:ro",
	}
	entrypoint := "/usr/local/bin/vosk-transcribe"
	image := r.pipelineWorkerImage
	if r.hostVoskBinaryPath != "" {
		args = append(args, "-v", r.hostVoskBinaryPath+":/bench-bin/vosk-transcribe:ro")
		entrypoint = "/bench-bin/vosk-transcribe"
		image = r.runtimeBaseImage
	}
	if strings.TrimSpace(grammarPath) != "" {
		args = append(args, "-v", filepath.Dir(grammarPath)+":/grammar:ro")
	}
	args = append(args, "--entrypoint", entrypoint, image, r.containerModelPath, "/bench/"+filepath.Base(wavPath))
	if strings.TrimSpace(grammarPath) != "" {
		args = append(args, "/grammar/"+filepath.Base(grammarPath))
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker vosk: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func verifySHA256(path, expected string) error {
	expected = strings.ToLower(strings.TrimSpace(expected))
	if expected == "" {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != expected {
		return fmt.Errorf("sha256 mismatch for %s: got %s want %s", path, actual, expected)
	}
	return nil
}

func shortHash(raw string) string {
	sum := sha1.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])[:12]
}

func normalizedExt(rawURL string) string {
	parsed, err := neturl.Parse(rawURL)
	if err != nil {
		return ".bin"
	}
	ext := strings.ToLower(filepath.Ext(parsed.Path))
	if ext == "" || len(ext) > 8 {
		return ".bin"
	}
	return ext
}

func copySeedFile(srcPath, destPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	tempPath := destPath + ".part"
	defer os.Remove(tempPath)

	dst, err := os.Create(tempPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, destPath)
}

func ensureLocalDockerImage(ctx context.Context, image string) error {
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", image)
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("docker image %q is missing: %s", image, msg)
	}
	return nil
}
