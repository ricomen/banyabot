package banya

import "fmt"

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

func PollQuestion() string {
	return "Кто идёт париться на этой неделе, а кто трусит?"
}

func HelpText() string {
	return "Я злобный баньщик.\n" +
		"По вторникам кидаю факт и опрос: Иду / Не иду.\n\n" +
		"/stats — кто как ответил на этой неделе\n" +
		"/stats last — прошлая неделя\n" +
		"/banya — админский повтор ритуала\n" +
		"/fact — админский факт без опроса"
}

func AlreadyPosted(weekID string) string {
	return fmt.Sprintf("На неделю %s опрос уже висит. Не жадничай.", weekID)
}

func StatsHeader(weekID string) string {
	return fmt.Sprintf("Неделя %s. Баньщик считает головы:\n", weekID)
}
