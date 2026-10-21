package telegram

import (
	"context"
	"fmt"
	"strconv"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"instabot/internal/banya"
)

const (
	waitSchedule = "schedule"
	waitChat     = "chat"
)

type Rescheduler interface {
	Set(spec string) error
}

func (b *Bot) rememberWait(userID int64, kind string) {
	b.pendMu.Lock()
	defer b.pendMu.Unlock()
	if b.pending == nil {
		b.pending = map[int64]string{}
	}
	b.pending[userID] = kind
}

func (b *Bot) takeWait(userID int64) string {
	b.pendMu.Lock()
	defer b.pendMu.Unlock()
	kind := b.pending[userID]
	delete(b.pending, userID)
	return kind
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
	switch q.Data {
	case "setup:when":
		b.rememberWait(q.From.ID, waitSchedule)
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "Напиши день и время, например: вторник 15:00",
		})
	case "setup:chat":
		b.rememberWait(q.From.ID, waitChat)
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "Пришли ссылку на группу: https://t.me/имя или ссылку на сообщение https://t.me/c/…\nПриглашение t.me/+… не подойдёт.",
		})
	}
}

func (b *Bot) handleSetupText(ctx context.Context, api *bot.Bot, msg *models.Message) bool {
	if msg == nil || msg.From == nil || msg.Chat.Type != models.ChatTypePrivate || !b.Cfg.IsAdmin(msg.From.ID) {
		return false
	}
	kind := b.takeWait(msg.From.ID)
	if kind == "" {
		return false
	}
	switch kind {
	case waitSchedule:
		b.applySchedule(ctx, api, msg)
	case waitChat:
		b.applyChat(ctx, api, msg)
	}
	return true
}

func (b *Bot) applySchedule(ctx context.Context, api *bot.Bot, msg *models.Message) {
	spec, label, err := banya.ParseSchedule(msg.Text)
	if err != nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: err.Error()})
		return
	}
	if b.Sched == nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "Расписание ещё не запущено."})
		return
	}
	if err := b.Sched.Set(spec); err != nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "Не вышло поставить расписание: " + err.Error()})
		return
	}
	if err := b.Store.SetSetting("cron_spec", spec); err != nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "Записать расписание не вышло."})
		return
	}
	_ = b.Store.SetSetting("schedule_label", label)
	b.Cfg.CronSpec = spec
	_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
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

func (b *Bot) setupText(ctx context.Context, api *bot.Bot) string {
	when := "ещё не выбрано"
	if label, ok, err := b.Store.Setting("schedule_label"); err == nil && ok && label != "" {
		when = label
	} else if pretty, ok := banya.FormatSchedule(b.Cfg.CronSpec); ok {
		when = pretty + " (по умолчанию)"
	} else if b.Cfg.CronSpec != "" {
		when = b.Cfg.CronSpec + " (по умолчанию)"
	}
	return "Настройка банщика.\n\nРасписание: " + when + "\nГруппа: " + b.groupLabel(ctx, api)
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
			{{Text: "Расписание (вторник 15:00)", CallbackData: "setup:when"}},
			{{Text: "Группа (ссылка t.me/…)", CallbackData: "setup:chat"}},
		},
	}
}
