package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"banyabot/internal/banya"
	"banyabot/internal/db"
)

const (
	waitSchedule = "schedule"
	waitChat     = "chat"
	waitGif      = "gif"
	waitCaption  = "caption"
	waitPoll     = "poll"
	waitOffense  = "offense:"
)

type Rescheduler interface {
	Set(spec string) error
	Clear()
}

func (b *Bot) rememberWait(userID int64, kind string) {
	b.pendMu.Lock()
	defer b.pendMu.Unlock()
	if b.pending == nil {
		b.pending = map[int64]string{}
	}
	b.pending[userID] = kind
}

func (b *Bot) peekWait(userID int64) string {
	b.pendMu.Lock()
	defer b.pendMu.Unlock()
	return b.pending[userID]
}

func (b *Bot) takeWait(userID int64) string {
	b.pendMu.Lock()
	defer b.pendMu.Unlock()
	kind := b.pending[userID]
	delete(b.pending, userID)
	return kind
}

func (b *Bot) publishMenu(ctx context.Context) {
	private := []models.BotCommand{
		{Command: "schedule", Description: "День и время опроса"},
		{Command: "noschedule", Description: "Выключить еженедельный опрос"},
		{Command: "group", Description: "Группа по ссылке"},
		{Command: "offense", Description: "Новая пара: гиф и текст"},
		{Command: "offenses", Description: "Все отповеди"},
		{Command: "question", Description: "Текст вопроса опроса"},
		{Command: "gryaznuli", Description: "Постыдить тех, кто не голосовал"},
	}
	if _, err := b.API.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: private,
		Scope:    &models.BotCommandScopeAllPrivateChats{},
	}); err != nil {
		slog.Warn("меню в личке", "err", err)
	}
	if _, err := b.API.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: []models.BotCommand{},
		Scope:    &models.BotCommandScopeAllGroupChats{},
	}); err != nil {
		slog.Warn("меню в группе", "err", err)
	}
}

func (b *Bot) privateAdmin(msg *models.Message) bool {
	return msg != nil && msg.From != nil && msg.Chat.Type == models.ChatTypePrivate && b.Cfg.IsAdmin(msg.From.ID)
}

func (b *Bot) onSchedule(ctx context.Context, api *bot.Bot, update *models.Update) {
	msg := update.Message
	if !b.privateAdmin(msg) {
		return
	}
	b.askSchedule(ctx, api, msg.Chat.ID, msg.From.ID)
}

func (b *Bot) onNoSchedule(ctx context.Context, api *bot.Bot, update *models.Update) {
	msg := update.Message
	if !b.privateAdmin(msg) {
		return
	}
	b.clearSchedule(ctx, api, msg.Chat.ID)
}

func (b *Bot) onGroup(ctx context.Context, api *bot.Bot, update *models.Update) {
	msg := update.Message
	if !b.privateAdmin(msg) {
		return
	}
	b.rememberWait(msg.From.ID, waitChat)
	_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "Пришли ссылку на группу: https://t.me/имя или ссылку на сообщение https://t.me/c/…\nПриглашение t.me/+… не подойдёт.",
	})
}

func (b *Bot) onOffense(ctx context.Context, api *bot.Bot, update *models.Update) {
	msg := update.Message
	if !b.privateAdmin(msg) {
		return
	}
	b.rememberWait(msg.From.ID, waitGif)
	_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "Пришли гиф. Файлом не надо, именно гифкой."})
}

func (b *Bot) onOffenses(ctx context.Context, api *bot.Bot, update *models.Update) {
	msg := update.Message
	if !b.privateAdmin(msg) {
		return
	}
	b.showOffenses(ctx, api, msg.Chat.ID)
}

func (b *Bot) onQuestion(ctx context.Context, api *bot.Bot, update *models.Update) {
	msg := update.Message
	if !b.privateAdmin(msg) {
		return
	}
	b.rememberWait(msg.From.ID, waitPoll)
	_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "Сейчас в опросе: " + b.Ritual.Question() + "\n\nНапиши новый вопрос. Чтобы вернуть фразу по умолчанию, пришли минус.",
	})
}

func (b *Bot) onStart(ctx context.Context, api *bot.Bot, update *models.Update) {
	msg := update.Message
	if msg == nil || msg.From == nil || msg.Chat.Type != models.ChatTypePrivate {
		return
	}
	if !b.Cfg.IsAdmin(msg.From.ID) {
		return
	}
	_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      msg.Chat.ID,
		Text:        b.setupText(ctx, api),
		ReplyMarkup: setupKeyboard(),
	})
}

func (b *Bot) onSetupCallback(ctx context.Context, api *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil || q.From.ID == 0 || !b.Cfg.IsAdmin(q.From.ID) {
		return
	}
	_, _ = api.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID})
	chatID := q.From.ID
	if q.Message.Message != nil {
		chatID = q.Message.Message.Chat.ID
	}
	switch {
	case q.Data == "setup:when":
		b.askSchedule(ctx, api, chatID, q.From.ID)
	case strings.HasPrefix(q.Data, "setup:dow:"):
		dow, err := strconv.Atoi(strings.TrimPrefix(q.Data, "setup:dow:"))
		if err != nil {
			return
		}
		b.askHour(ctx, api, chatID, dow)
	case strings.HasPrefix(q.Data, "setup:hr:"):
		dow, hour, ok := splitTwo(strings.TrimPrefix(q.Data, "setup:hr:"))
		if !ok {
			return
		}
		b.askMinute(ctx, api, chatID, dow, hour)
	case strings.HasPrefix(q.Data, "setup:mi:"):
		dow, hour, minute, ok := splitThree(strings.TrimPrefix(q.Data, "setup:mi:"))
		if !ok {
			return
		}
		spec, label, err := banya.ScheduleFrom(dow, hour, minute)
		if err != nil {
			_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: err.Error()})
			return
		}
		b.saveSchedule(ctx, api, chatID, q.From.ID, spec, label)
	case q.Data == "setup:clear":
		b.clearSchedule(ctx, api, chatID)
	case q.Data == "setup:chat":
		b.rememberWait(q.From.ID, waitChat)
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "Пришли ссылку на группу: https://t.me/имя или ссылку на сообщение https://t.me/c/…\nПриглашение t.me/+… не подойдёт.",
		})
	case q.Data == "setup:offense":
		b.rememberWait(q.From.ID, waitGif)
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "Пришли гиф. Файлом не надо, именно гифкой.",
		})
	case q.Data == "setup:offenses":
		b.showOffenses(ctx, api, chatID)
	case q.Data == "setup:poll":
		b.rememberWait(q.From.ID, waitPoll)
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "Сейчас в опросе: " + b.Ritual.Question() + "\n\nНапиши новый вопрос. Чтобы вернуть фразу по умолчанию, пришли минус.",
		})
	case q.Data == "setup:shame":
		b.fireShame(ctx, api, chatID, b.Cfg.BanyaChatID)
	case strings.HasPrefix(q.Data, "setup:off:edit:"):
		id, err := strconv.ParseInt(strings.TrimPrefix(q.Data, "setup:off:edit:"), 10, 64)
		if err != nil {
			return
		}
		b.rememberWait(q.From.ID, waitOffense+strconv.FormatInt(id, 10))
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "Новый текст для этой отповеди. Гифка останется прежней.",
		})
	case strings.HasPrefix(q.Data, "setup:off:del:"):
		id, err := strconv.ParseInt(strings.TrimPrefix(q.Data, "setup:off:del:"), 10, 64)
		if err != nil {
			return
		}
		if err := b.Store.DeleteOffense(id); err != nil {
			_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: "Удалить не вышло."})
			return
		}
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: "Отповедь выкинул."})
		b.showOffenses(ctx, api, chatID)
	}
}

func (b *Bot) handleSetupText(ctx context.Context, api *bot.Bot, msg *models.Message) bool {
	if msg == nil || msg.From == nil || msg.Chat.Type != models.ChatTypePrivate || !b.Cfg.IsAdmin(msg.From.ID) {
		return false
	}
	kind := b.peekWait(msg.From.ID)
	if kind == "" {
		return false
	}
	switch {
	case kind == waitSchedule:
		b.takeWait(msg.From.ID)
		b.applySchedule(ctx, api, msg)
	case kind == waitChat:
		b.takeWait(msg.From.ID)
		b.applyChat(ctx, api, msg)
	case kind == waitGif:
		b.applyOffenseGif(ctx, api, msg)
	case kind == waitCaption:
		b.takeWait(msg.From.ID)
		b.applyOffenseCaption(ctx, api, msg)
	case kind == waitPoll:
		b.takeWait(msg.From.ID)
		b.applyPoll(ctx, api, msg)
	case strings.HasPrefix(kind, waitOffense):
		b.takeWait(msg.From.ID)
		b.applyOffenseEdit(ctx, api, msg, strings.TrimPrefix(kind, waitOffense))
	default:
		b.takeWait(msg.From.ID)
	}
	return true
}

func (b *Bot) clearSchedule(ctx context.Context, api *bot.Bot, chatID int64) {
	if b.Sched == nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: "Расписание ещё не запущено."})
		return
	}
	b.Sched.Clear()
	if err := b.Store.SetSetting("cron_off", "1"); err != nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: "Записать не вышло."})
		return
	}
	_ = b.Store.DeleteSetting("cron_spec")
	_ = b.Store.DeleteSetting("schedule_label")
	b.Cfg.CronSpec = ""
	_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   "Еженедельный опрос выключен. Уже висящий опрос через сутки после публикации один раз напомнит молчунам.",
	})
}

func (b *Bot) applyOffenseGif(ctx context.Context, api *bot.Bot, msg *models.Message) {
	if msg.Animation == nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "Это не гиф. Пришли гифкой."})
		return
	}
	b.pendMu.Lock()
	if b.pendingGif == nil {
		b.pendingGif = map[int64]string{}
	}
	b.pendingGif[msg.From.ID] = msg.Animation.FileID
	b.pendMu.Unlock()
	b.rememberWait(msg.From.ID, waitCaption)
	_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "Теперь текст. Список тех, кто не голосовал, допишу сам.",
	})
}

func (b *Bot) applyOffenseCaption(ctx context.Context, api *bot.Bot, msg *models.Message) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		b.rememberWait(msg.From.ID, waitCaption)
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "Текст пустой. Напиши оскорбление."})
		return
	}
	b.pendMu.Lock()
	fileID := b.pendingGif[msg.From.ID]
	delete(b.pendingGif, msg.From.ID)
	b.pendMu.Unlock()
	if fileID == "" {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "Гиф потерялась. Начни заново с кнопки."})
		return
	}
	if err := b.Store.InsertOffense(fileID, text); err != nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "Записать не вышло."})
		return
	}
	_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "Запомнил. В следующий стыд выберу эту или другую пару."})
}

func (b *Bot) applyOffenseEdit(ctx context.Context, api *bot.Bot, msg *models.Message, rawID string) {
	text := strings.TrimSpace(msg.Text)
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || text == "" {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "Текст пустой, ничего не менял."})
		return
	}
	if err := b.Store.UpdateOffenseCaption(id, text); err != nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "Такой отповеди уже нет."})
		return
	}
	_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "Текст заменил."})
}

func (b *Bot) applyPoll(ctx context.Context, api *bot.Bot, msg *models.Message) {
	text := strings.TrimSpace(msg.Text)
	if text == "-" {
		if err := b.Store.DeleteSetting("poll_question"); err != nil {
			_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "Стереть не вышло."})
			return
		}
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "Вернул вопрос по умолчанию: " + banya.PollQuestion()})
		return
	}
	if utf8.RuneCountInString(text) > 300 {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "В опрос влезает 300 знаков. Сократи."})
		return
	}
	if text == "" {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "Пусто. Напиши вопрос или минус, чтобы вернуть старый."})
		return
	}
	if err := b.Store.SetSetting("poll_question", text); err != nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "Записать не вышло."})
		return
	}
	_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "Вопрос опроса теперь такой: " + text})
}

func (b *Bot) showOffenses(ctx context.Context, api *bot.Bot, chatID int64) {
	list, err := b.Store.Offenses()
	if err != nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: "Список не открылся."})
		return
	}
	if len(list) == 0 {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: "Отповедей нет. Добавь пару гифка и текст."})
		return
	}
	var text strings.Builder
	text.WriteString("Отповеди:\n")
	var rows [][]models.InlineKeyboardButton
	for i, o := range list {
		preview := o.Caption
		if utf8.RuneCountInString(preview) > 60 {
			preview = string([]rune(preview)[:60]) + "…"
		}
		fmt.Fprintf(&text, "\n%d. %s", i+1, preview)
		id := strconv.FormatInt(o.ID, 10)
		n := strconv.Itoa(i + 1)
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: n + " править", CallbackData: "setup:off:edit:" + id},
			{Text: n + " удалить", CallbackData: "setup:off:del:" + id},
		})
	}
	_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        text.String(),
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: rows},
	})
}

func (b *Bot) fireShame(ctx context.Context, api *bot.Bot, replyChat, dest int64) {
	weekID := db.WeekID(time.Now().In(b.Cfg.Location))
	names, err := b.Ritual.Absent(b.Cfg.BanyaChatID, weekID)
	if err != nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: replyChat, Text: "Список грязнуль не собрался: " + err.Error()})
		return
	}
	if len(names) == 0 {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: replyChat,
			Text:   "Все, кого баньщик видел, уже проголосовали. Остальных он не знает.",
		})
		return
	}
	fileID, caption := b.Ritual.PickOffense()
	if err := b.SendShame(ctx, dest, fileID, banya.ShameText(caption, names)); err != nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: replyChat, Text: "Гиф не ушла: " + err.Error()})
		return
	}
	if replyChat != dest {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: replyChat, Text: "Отправил в группу."})
	}
}

func (b *Bot) applySchedule(ctx context.Context, api *bot.Bot, msg *models.Message) {
	spec, label, err := banya.ParseSchedule(msg.Text)
	if err != nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: err.Error()})
		return
	}
	b.saveSchedule(ctx, api, msg.Chat.ID, msg.From.ID, spec, label)
}

func (b *Bot) saveSchedule(ctx context.Context, api *bot.Bot, chatID, userID int64, spec, label string) {
	b.takeWait(userID)
	if b.Sched == nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: "Расписание ещё не запущено."})
		return
	}
	if err := b.Sched.Set(spec); err != nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: "Не вышло поставить расписание: " + err.Error()})
		return
	}
	if err := b.Store.SetSetting("cron_spec", spec); err != nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: "Записать расписание не вышло."})
		return
	}
	_ = b.Store.SetSetting("schedule_label", label)
	_ = b.Store.DeleteSetting("cron_off")
	b.Cfg.CronSpec = spec
	_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   "Опрос буду кидать: " + label + ".",
	})
}

func (b *Bot) applyChat(ctx context.Context, api *bot.Bot, msg *models.Message) {
	ref, err := banya.ParseChatLink(msg.Text)
	if err != nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: err.Error()})
		return
	}
	chatID := ref.ID
	title := ""
	if ref.Username != "" {
		info, err := api.GetChat(ctx, &bot.GetChatParams{ChatID: "@" + ref.Username})
		if err != nil {
			_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: msg.Chat.ID,
				Text:   "Группу по этой ссылке не вижу. Бот должен уже состоять в ней, и ссылка должна быть публичной.",
			})
			return
		}
		if info.Type != models.ChatTypeGroup && info.Type != models.ChatTypeSupergroup {
			_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "Это не группа."})
			return
		}
		chatID = info.ID
		title = info.Title
	}
	if title == "" && chatID != 0 {
		if info, err := api.GetChat(ctx, &bot.GetChatParams{ChatID: chatID}); err == nil {
			title = info.Title
		}
	}
	if err := b.Store.SetSetting("chat_id", strconv.FormatInt(chatID, 10)); err != nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "Записать группу не вышло."})
		return
	}
	b.Cfg.BanyaChatID = chatID
	if b.Ritual != nil {
		b.Ritual.ChatID = chatID
	}
	if title != "" {
		_ = b.Store.SetSetting("chat_title", title)
	}
	_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "Группа сохранена: " + formatGroup(title, chatID) + ".",
	})
}

func (b *Bot) askSchedule(ctx context.Context, api *bot.Bot, chatID, userID int64) {
	b.rememberWait(userID, waitSchedule)
	_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        "Сейчас: " + b.scheduleWhen() + ".\nВыбери день. Или напиши, например: вторник 15:00",
		ReplyMarkup: dayKeyboard(),
	})
}

func (b *Bot) askHour(ctx context.Context, api *bot.Bot, chatID int64, dow int) {
	_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        "Час:",
		ReplyMarkup: hourKeyboard(dow),
	})
}

func (b *Bot) askMinute(ctx context.Context, api *bot.Bot, chatID int64, dow, hour int) {
	_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        "Минуты:",
		ReplyMarkup: minuteKeyboard(dow, hour),
	})
}

func dayKeyboard() models.InlineKeyboardMarkup {
	days := []struct {
		name string
		dow  int
	}{
		{"Понедельник", 1},
		{"Вторник", 2},
		{"Среда", 3},
		{"Четверг", 4},
		{"Пятница", 5},
		{"Суббота", 6},
		{"Воскресенье", 0},
	}
	rows := make([][]models.InlineKeyboardButton, 0, len(days))
	for _, d := range days {
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         d.name,
			CallbackData: "setup:dow:" + strconv.Itoa(d.dow),
		}})
	}
	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func hourKeyboard(dow int) models.InlineKeyboardMarkup {
	var rows [][]models.InlineKeyboardButton
	var row []models.InlineKeyboardButton
	for hour := 0; hour < 24; hour++ {
		row = append(row, models.InlineKeyboardButton{
			Text:         fmt.Sprintf("%02d", hour),
			CallbackData: fmt.Sprintf("setup:hr:%d:%d", dow, hour),
		})
		if len(row) == 6 {
			rows = append(rows, row)
			row = nil
		}
	}
	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func minuteKeyboard(dow, hour int) models.InlineKeyboardMarkup {
	var row []models.InlineKeyboardButton
	for _, minute := range []int{0, 15, 30, 45} {
		row = append(row, models.InlineKeyboardButton{
			Text:         fmt.Sprintf(":%02d", minute),
			CallbackData: fmt.Sprintf("setup:mi:%d:%d:%d", dow, hour, minute),
		})
	}
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{row}}
}

func splitTwo(raw string) (int, int, bool) {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	a, err1 := strconv.Atoi(parts[0])
	b, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return a, b, true
}

func splitThree(raw string) (int, int, int, bool) {
	parts := strings.Split(raw, ":")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	a, err1 := strconv.Atoi(parts[0])
	b, err2 := strconv.Atoi(parts[1])
	c, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, false
	}
	return a, b, c, true
}

func (b *Bot) scheduleWhen() string {
	when := "ещё не выбрано"
	if label, ok, err := b.Store.Setting("schedule_label"); err == nil && ok && label != "" {
		when = label
	} else if pretty, ok := banya.FormatSchedule(b.Cfg.CronSpec); ok {
		when = pretty + " (по умолчанию)"
	} else if b.Cfg.CronSpec != "" {
		when = b.Cfg.CronSpec + " (по умолчанию)"
	}
	if off, ok, err := b.Store.Setting("cron_off"); err == nil && ok && off == "1" {
		when = "выключено"
	}
	return when
}

func (b *Bot) setupText(ctx context.Context, api *bot.Bot) string {
	when := b.scheduleWhen()
	poll := b.Ritual.Question()
	n := 0
	if list, err := b.Store.Offenses(); err == nil {
		n = len(list)
	}
	return "Настройка банщика.\n\nРасписание: " + when +
		"\nГруппа: " + b.groupLabel(ctx, api) +
		"\nОпрос: " + poll +
		"\nОтповедей: " + strconv.Itoa(n)
}

func (b *Bot) groupLabel(ctx context.Context, api *bot.Bot) string {
	id := b.Cfg.BanyaChatID
	if raw, ok, err := b.Store.Setting("chat_id"); err == nil && ok && raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			id = n
		}
	}
	if id == 0 {
		return "ещё не выбрана"
	}
	title, ok, err := b.Store.Setting("chat_title")
	if err == nil && (!ok || title == "") {
		if info, err := api.GetChat(ctx, &bot.GetChatParams{ChatID: id}); err == nil {
			title = info.Title
			if title != "" {
				_ = b.Store.SetSetting("chat_title", title)
			}
		}
	}
	return formatGroup(title, id)
}

func formatGroup(title string, id int64) string {
	if title == "" {
		return strconv.FormatInt(id, 10)
	}
	return fmt.Sprintf("%s (%d)", title, id)
}

func setupKeyboard() models.InlineKeyboardMarkup {
	return models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "Расписание", CallbackData: "setup:when"}},
			{{Text: "Убрать расписание", CallbackData: "setup:clear"}},
			{{Text: "Группа", CallbackData: "setup:chat"}},
			{{Text: "Новая отповедь", CallbackData: "setup:offense"}},
			{{Text: "Отповеди", CallbackData: "setup:offenses"}},
			{{Text: "Текст опроса", CallbackData: "setup:poll"}},
			{{Text: "Грязнули", CallbackData: "setup:shame"}},
		},
	}
}
