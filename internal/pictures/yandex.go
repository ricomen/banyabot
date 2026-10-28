package pictures

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var yandexSearch = "https://yandex.ru/images/search"

var imagesAppState = regexp.MustCompile(`id="ImagesApp-[^"]*"\s+data-state="([^"]+)"`)

// Yandex downloads one JPEG or PNG from the first Yandex Images result page.
func (c *Client) Yandex(ctx context.Context, fact string) ([]byte, error) {
	page, err := c.yandexGet(ctx, yandexSearch+"?"+url.Values{"text": {query(fact)}}.Encode())
	if err != nil {
		return nil, err
	}
	urls := yandexImageURLs(page)
	if len(urls) == 0 {
		return nil, fmt.Errorf("yandex: no images")
	}
	c.rng().Shuffle(len(urls), func(i, j int) { urls[i], urls[j] = urls[j], urls[i] })
	var last error
	for _, rawURL := range urls {
		data, err := c.yandexGet(ctx, rawURL)
		if err != nil {
			last = err
			continue
		}
		if isJPEG(data) || isPNG(data) {
			return data, nil
		}
		last = fmt.Errorf("yandex: not a jpeg or png")
	}
	if last == nil {
		last = fmt.Errorf("yandex: no images")
	}
	return nil, last
}

func (c *Client) yandexGet(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "ru")
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
		return nil, fmt.Errorf("yandex %s", resp.Status)
	}
	return data, nil
}

func yandexImageURLs(page []byte) []string {
	m := imagesAppState.FindSubmatch(page)
	if m == nil {
		return nil
	}
	var state struct {
		InitialState struct {
			SerpList struct {
				Items struct {
					Entities map[string]struct {
						Image      string `json:"image"`
						ViewerData struct {
							ImgHref string `json:"img_href"`
						} `json:"viewerData"`
					} `json:"entities"`
				} `json:"items"`
			} `json:"serpList"`
		} `json:"initialState"`
	}
	if err := json.Unmarshal([]byte(html.UnescapeString(string(m[1]))), &state); err != nil {
		return nil
	}
	var out []string
	for _, item := range state.InitialState.SerpList.Items.Entities {
		if href := httpURL(item.ViewerData.ImgHref); href != "" {
			out = append(out, href)
			continue
		}
		if thumb := httpURL(item.Image); thumb != "" {
			out = append(out, thumb)
		}
	}
	return out
}

func httpURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return u.String()
}

func isJPEG(b []byte) bool {
	return len(b) > 3 && b[0] == 0xff && b[1] == 0xd8 && b[2] == 0xff
}

func isPNG(b []byte) bool {
	return len(b) > 8 && b[0] == 0x89 && b[1] == 'P' && b[2] == 'N' && b[3] == 'G'
}
