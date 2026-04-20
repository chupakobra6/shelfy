package copy

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"text/template"
)

type Catalog struct {
	Messages map[string]string `yaml:"messages"`
	Labels   map[string]string `yaml:"labels"`
}

type Loader struct {
	catalog Catalog
}

func Load(path string) (*Loader, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read copy catalog: %w", err)
	}
	var catalog Catalog
	if err := yamlUnmarshal(body, &catalog); err != nil {
		return nil, fmt.Errorf("parse copy catalog: %w", err)
	}
	return &Loader{catalog: catalog}, nil
}

func (l *Loader) Render(messageID string, data map[string]any) (string, error) {
	entry, ok := l.catalog.Messages[messageID]
	if !ok {
		return "", fmt.Errorf("message %s not found in catalog", messageID)
	}
	tmpl, err := template.New(messageID).Option("missingkey=zero").Parse(normalizeTemplate(entry))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (l *Loader) Label(labelID string) (string, error) {
	value, ok := l.catalog.Labels[labelID]
	if !ok {
		return "", fmt.Errorf("label %s not found in catalog", labelID)
	}
	return value, nil
}

var simplePlaceholderPattern = regexp.MustCompile(`{{\s*([a-zA-Z0-9_]+)\s*}}`)

func normalizeTemplate(input string) string {
	return simplePlaceholderPattern.ReplaceAllString(input, "{{.$1}}")
}
