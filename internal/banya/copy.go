package banya

import (
	"fmt"
	"strings"
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
	var b strings.Builder
	b.WriteString("Вот эти грязнули на этой неделе не ходили в баню, помогу им отмыться:\n")
	for _, name := range names {
		b.WriteString("— ")
		b.WriteString(name)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func PollQuestion() string {
	return "Кто идёт париться на этой неделе, а кто трусит?"
}

func HelpText() string {
	return "Я злобный баньщик.\n" +
		"По вторникам кидаю факт и опрос: Иду / Не иду.\n\n" +
		"/stats — кто как ответил на этой неделе\n" +
		"/stats last — прошлая неделя\n" +
		"/gryaznuli — админская проверка: гиф и кто не голосовал\n" +
		"/banya — админский повтор ритуала\n" +
		"/fact — админский факт без опроса\n" +
		"/start — настройки в личке админа"
}

func AlreadyPosted(weekID string) string {
	return fmt.Sprintf("На неделю %s опрос уже висит. Не жадничай.", weekID)
}

func StatsHeader(weekID string) string {
	return fmt.Sprintf("Неделя %s. Баньщик считает головы:\n", weekID)
}
