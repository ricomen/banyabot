package banya

import "testing"

func TestFormatScheduleDefault(t *testing.T) {
	got, ok := FormatSchedule("0 18 * * 2")
	if !ok || got != "вторник 18:00" {
		t.Fatalf("%q %v", got, ok)
	}
}

func TestParseScheduleTuesday(t *testing.T) {
	spec, label, err := ParseSchedule("Вторник 15:00")
	if err != nil {
		t.Fatal(err)
	}
	if spec != "0 15 * * 2" || label != "вторник 15:00" {
		t.Fatalf("spec %q label %q", spec, label)
	}
}

func TestParseChatLink(t *testing.T) {
	ref, err := ParseChatLink("https://t.me/banya_chat")
	if err != nil || ref.Username != "banya_chat" {
		t.Fatalf("%+v %v", ref, err)
	}
	ref, err = ParseChatLink("https://t.me/c/1234567890/42")
	if err != nil || ref.ID != -1001234567890 {
		t.Fatalf("%+v %v", ref, err)
	}
	if _, err := ParseChatLink("https://t.me/+AbCd"); err == nil {
		t.Fatal("invite link should fail")
	}
}
