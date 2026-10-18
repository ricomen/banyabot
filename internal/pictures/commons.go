package pictures

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const userAgent = "instabot/1.0 (telegram banya facts; educational)"

type Client struct {
	HTTP *http.Client
	RNG  *rand.Rand
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (c *Client) rng() *rand.Rand {
	if c.RNG != nil {
		return c.RNG
	}
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}

// Find downloads one JPEG or PNG from Wikimedia Commons that matches the fact.
func (c *Client) Find(ctx context.Context, fact string) ([]byte, error) {
	urls, err := c.search(ctx, query(fact))
	if err != nil {
		return nil, err
	}
	if len(urls) == 0 {
		urls, err = c.search(ctx, "русская баня")
		if err != nil {
			return nil, err
		}
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("commons: no images")
	}
	pick := urls[c.rng().Intn(len(urls))]
	return c.get(ctx, pick)
}

func (c *Client) search(ctx context.Context, text string) ([]string, error) {
	q := url.Values{}
	q.Set("action", "query")
	q.Set("format", "json")
	q.Set("generator", "search")
	q.Set("gsrsearch", text)
	q.Set("gsrnamespace", "6")
	q.Set("gsrlimit", "12")
	q.Set("prop", "imageinfo")
	q.Set("iiprop", "url|mime")
	q.Set("iiurlwidth", "1024")
	body, err := c.get(ctx, "https://commons.wikimedia.org/w/api.php?"+q.Encode())
	if err != nil {
		return nil, err
	}
	return imageURLs(body), nil
}

func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("commons %s", resp.Status)
	}
	return data, nil
}

func query(fact string) string {
	fields := strings.Fields(fact)
	if len(fields) > 8 {
		fields = fields[:8]
	}
	q := strings.Join(fields, " ")
	if q == "" {
		return "русская баня"
	}
	return "баня " + q
}

func imageURLs(raw []byte) []string {
	var parsed struct {
		Query struct {
			Pages map[string]struct {
				ImageInfo []struct {
					Mime     string `json:"mime"`
					ThumbURL string `json:"thumburl"`
					URL      string `json:"url"`
				} `json:"imageinfo"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil
	}
	var out []string
	for _, page := range parsed.Query.Pages {
		for _, info := range page.ImageInfo {
			if info.Mime != "image/jpeg" && info.Mime != "image/png" {
				continue
			}
			if info.ThumbURL != "" {
				out = append(out, info.ThumbURL)
				continue
			}
			if info.URL != "" {
				out = append(out, info.URL)
			}
		}
	}
	return out
}
