package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const defaultModel = "gemini-3.5-flash-lite"

type Client struct {
	APIKey string
	Model  string
	HTTP   *http.Client
	URL    string
}

func (c *Client) model() string {
	if c.Model != "" {
		return c.Model
	}
	return defaultModel
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 45 * time.Second}
}

func (c *Client) Fact(ctx context.Context, recent []string) (string, error) {
	if c.APIKey == "" {
		return "", fmt.Errorf("gemini api key is empty")
	}
	user := "Дай один новый факт про русскую баню."
	if len(recent) > 0 {
		user += " Не повторяй уже отправленное:\n- " + strings.Join(recent, "\n- ")
	}
	body, err := json.Marshal(map[string]any{
		"model": c.model(),
		"system_instruction": "Пишешь один короткий факт про русскую баню для Telegram. " +
			"Одно или два предложения, разговорный тон злобного баньщика. " +
			"Без списков, заголовков и ссылок. Не выдумывай точные цифры.",
		"input": user,
	})
	if err != nil {
		return "", err
	}

	endpoint := c.URL
	if endpoint == "" {
		endpoint = "https://generativelanguage.googleapis.com/v1beta/interactions"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.APIKey)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini %s: %s", resp.Status, truncate(string(raw), 300))
	}

	var parsed struct {
		OutputText string `json:"output_text"`
		Steps      []struct {
			Type    string `json:"type"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	var b strings.Builder
	for _, step := range parsed.Steps {
		if step.Type != "model_output" {
			continue
		}
		for _, block := range step.Content {
			b.WriteString(block.Text)
		}
	}
	if b.Len() == 0 {
		b.WriteString(parsed.OutputText)
	}
	text := cleanFact(b.String())
	if text == "" {
		return "", fmt.Errorf("gemini returned empty fact")
	}
	return text, nil
}

func cleanFact(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'«»`)
	if i := strings.Index(s, "\n\n"); i >= 0 {
		s = s[:i]
	}
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	const max = 500
	if utf8.RuneCountInString(s) > max {
		runes := []rune(s)
		s = strings.TrimSpace(string(runes[:max]))
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
