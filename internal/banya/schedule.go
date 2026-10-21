package banya

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

var weekdays = map[string]int{
	"воскресенье": 0,
	"понедельник": 1,
	"вторник":     2,
	"среда":       3,
	"четверг":     4,
	"пятница":     5,
	"суббота":     6,
}

var weekdayNames = []string{
	"воскресенье",
	"понедельник",
	"вторник",
	"среда",
	"четверг",
	"пятница",
	"суббота",
}

// FormatSchedule turns a simple weekly cron "0 18 * * 2" into "вторник 18:00".
func FormatSchedule(spec string) (string, bool) {
	fields := strings.Fields(spec)
	if len(fields) != 5 || fields[2] != "*" || fields[3] != "*" {
		return "", false
	}
	minute, err1 := strconv.Atoi(fields[0])
	hour, err2 := strconv.Atoi(fields[1])
	dow, err3 := strconv.Atoi(fields[4])
	if err1 != nil || err2 != nil || err3 != nil || minute < 0 || minute > 59 || hour < 0 || hour > 23 || dow < 0 || dow > 6 {
		return "", false
	}
	return fmt.Sprintf("%s %02d:%02d", weekdayNames[dow], hour, minute), true
}

// ParseSchedule reads "вторник 15:00" into a 5-field cron spec and the same label.
func ParseSchedule(raw string) (spec, label string, err error) {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(raw)))
	if len(fields) != 2 {
		return "", "", fmt.Errorf("нужно так: вторник 15:00")
	}
	dow, ok := weekdays[fields[0]]
	if !ok {
		return "", "", fmt.Errorf("день недели не узнан: %s", fields[0])
	}
	hour, minute, err := parseClock(fields[1])
	if err != nil {
		return "", "", err
	}
	spec = fmt.Sprintf("%d %d * * %d", minute, hour, dow)
	label = fields[0] + " " + fmt.Sprintf("%02d:%02d", hour, minute)
	return spec, label, nil
}

func parseClock(raw string) (hour, minute int, err error) {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("время нужно как 15:00")
	}
	hour, err = strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("час от 0 до 23")
	}
	minute, err = strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("минуты от 00 до 59")
	}
	return hour, minute, nil
}

// ChatRef is a public username or a numeric chat id taken from a link.
type ChatRef struct {
	Username string
	ID       int64
}

// ParseChatLink accepts t.me/name, @name, or a t.me/c/… message link.
// Invite links (t.me/+…) cannot be resolved.
func ParseChatLink(raw string) (ChatRef, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	if strings.HasPrefix(s, "@") {
		name := strings.TrimPrefix(s, "@")
		if !validUsername(name) {
			return ChatRef{}, fmt.Errorf("это не имя группы")
		}
		return ChatRef{Username: name}, nil
	}
	s = strings.TrimPrefix(s, "t.me/")
	s = strings.TrimPrefix(s, "telegram.me/")
	s = strings.Trim(s, "/")
	if s == "" || strings.HasPrefix(s, "+") || strings.HasPrefix(s, "joinchat/") {
		return ChatRef{}, fmt.Errorf("приглашение t.me/+… бот не раскрывает. Добавь его в группу и пришли публичную ссылку t.me/имя или ссылку на сообщение t.me/c/…")
	}
	if strings.HasPrefix(s, "c/") {
		rest := strings.TrimPrefix(s, "c/")
		idPart := strings.Split(rest, "/")[0]
		n, err := strconv.ParseInt("-100"+idPart, 10, 64)
		if err != nil || n >= 0 {
			return ChatRef{}, fmt.Errorf("в ссылке t.me/c/… не вижу id группы")
		}
		return ChatRef{ID: n}, nil
	}
	name := strings.Split(s, "/")[0]
	if !validUsername(name) {
		return ChatRef{}, fmt.Errorf("это не ссылка на группу")
	}
	return ChatRef{Username: name}, nil
}

func validUsername(name string) bool {
	if len(name) < 4 {
		return false
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			continue
		}
		return false
	}
	return true
}
