package pictures

import "testing"

func TestImageURLsSkipsSVG(t *testing.T) {
	raw := []byte(`{"query":{"pages":{
		"1":{"imageinfo":[{"mime":"image/svg+xml","thumburl":"https://example/a.svg"}]},
		"2":{"imageinfo":[{"mime":"image/jpeg","thumburl":"https://example/b.jpg"}]}
	}}}`)
	got := imageURLs(raw)
	if len(got) != 1 || got[0] != "https://example/b.jpg" {
		t.Fatalf("%v", got)
	}
}

func TestQueryKeepsFactWords(t *testing.T) {
	q := query("Берёзовый веник мягче дубового и пахнет летом после дождя совсем")
	if q != "баня Берёзовый веник мягче дубового и пахнет летом после" {
		t.Fatalf("%q", q)
	}
}
