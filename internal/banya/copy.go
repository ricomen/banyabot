package banya

import (
	"fmt"
	"strings"
	"time"
)

const (
	OptionGoing    = "Иду"
	OptionNotGoing = "Не иду"
)

func Options() []string {
	return []string{OptionGoing, OptionNotGoing}
}

func OptionText(id int) string {
	opts := Options()
	if id < 0 || id >= len(opts) {
		return fmt.Sprintf("вариант %d", id)
	}
	return opts[id]
}

func NobodyVoted() string {
	return "Сутки прошли, а в опросе тишина. Веник сам себя не погладит."
}

func DirtyList(names []string) string {
	return ShameText("", names)
}

func ShameText(caption string, names []string) string {
	var b strings.Builder
	caption = strings.TrimSpace(caption)
	if caption == "" {
		b.WriteString("Вот эти грязнули на этой неделе не ходили в баню, помогу им отмыться:")
	} else {
		b.WriteString(caption)
	}
	for _, name := range names {
		b.WriteString("\n— ")
		b.WriteString(name)
	}
	return b.String()
}

func PollQuestion() string {
	return "ЗаеБанька на этой неделе?"
}

func HelpText() string {
	return "Я банщик.\n" +
		"По вторникам кидаю факт и опрос: Иду / Не иду.\n\n" +
		"/stats — кто как ответил на этой неделе\n" +
		"/stats last — прошлая неделя\n" +
		"/gryaznuli — «Грязнули»\n" +
		"/banya — админский повтор ритуала\n" +
		"/fact — админский факт без опроса\n" +
		"/start — настройки в личке админа"
}

func AlreadyPosted(weekID string) string {
	return "Опрос на " + WeekLabel(weekID) + " уже висит. Не жадничай."
}

func StatsHeader(weekID string) string {
	return WeekLabel(weekID) + ". Баньщик считает головы:\n"
}

func WeekLabel(weekID string) string {
	var year, week int
	if _, err := fmt.Sscanf(weekID, "%d-W%d", &year, &week); err != nil || week < 1 || week > 53 {
		return weekID
	}
	jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
	wd := int(jan4.Weekday())
	if wd == 0 {
		wd = 7
	}
	start := jan4.AddDate(0, 0, -(wd-1)+(week-1)*7)
	end := start.AddDate(0, 0, 6)
	return fmt.Sprintf("%d %s — %d %s", start.Day(), monthGenitive(start.Month()), end.Day(), monthGenitive(end.Month()))
}

func monthGenitive(m time.Month) string {
	names := [...]string{
		"", "января", "февраля", "марта", "апреля", "мая", "июня",
		"июля", "августа", "сентября", "октября", "ноября", "декабря",
	}
	if m < time.January || m > time.December {
		return ""
	}
	return names[m]
}
