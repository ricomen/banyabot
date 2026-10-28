package pictures

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestYandexImageURLs(t *testing.T) {
	page := `<div id="ImagesApp-1" data-state="{&quot;initialState&quot;:{&quot;serpList&quot;:{&quot;items&quot;:{&quot;entities&quot;:{&quot;a&quot;:{&quot;image&quot;:&quot;//avatars.mds.yandex.net/i?id=1&quot;,&quot;viewerData&quot;:{&quot;img_href&quot;:&quot;https://cdn.example/a.jpg&quot;}},&quot;b&quot;:{&quot;image&quot;:&quot;//avatars.mds.yandex.net/i?id=2&quot;,&quot;viewerData&quot;:{}}}}}}}" data-hydrate-priority="1">`
	got := yandexImageURLs([]byte(page))
	want := map[string]bool{
		"https://cdn.example/a.jpg":             true,
		"https://avatars.mds.yandex.net/i?id=2": true,
	}
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for _, u := range got {
		if !want[u] {
			t.Fatalf("%v", got)
		}
	}
}

func TestYandexPicksOnePhoto(t *testing.T) {
	jpeg := []byte{0xff, 0xd8, 0xff, 0x00}
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if strings.Contains(r.URL.Path, "search") {
			w.Write([]byte(`<div id="ImagesApp-1" data-state="{&quot;initialState&quot;:{&quot;serpList&quot;:{&quot;items&quot;:{&quot;entities&quot;:{&quot;a&quot;:{&quot;viewerData&quot;:{&quot;img_href&quot;:&quot;` + srvURL(r) + `/a.jpg&quot;}}}}}}}" >`))
			return
		}
		w.Write(jpeg)
	}))
	defer srv.Close()

	old := yandexSearch
	yandexSearch = srv.URL + "/search"
	t.Cleanup(func() { yandexSearch = old })

	got, err := (&Client{HTTP: srv.Client()}).Yandex(context.Background(), "баня")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(jpeg) || hits < 2 {
		t.Fatalf("hits=%d body=%v", hits, got)
	}
}

func srvURL(r *http.Request) string {
	return "http://" + r.Host
}
