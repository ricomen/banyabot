package telegram

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"banyabot/internal/banya"
	"banyabot/internal/config"
	"banyabot/internal/db"
	"banyabot/internal/pictures"
)

//go:embed banya.jpg
var banyaPhoto []byte

type Bot struct {
	API     *bot.Bot
	Cfg     config.Config
	Store   *db.Store
	Ritual  *banya.Ritual
	Sched   Rescheduler
	pending map[int64]string
	pendMu  sync.Mutex
}

func New(cfg config.Config, store *db.Store, ritual *banya.Ritual) (*Bot, error) {
	b := &Bot{Cfg: cfg, Store: store, Ritual: ritual}
	api, err := bot.New(cfg.BotToken, bot.WithDefaultHandler(b.defaultHandler))
	if err != nil {
		return nil, err
	}
	b.API = api
	ritual.Sender = b
	api.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.PollAnswer != nil
	}, b.onPollAnswer)
	api.RegisterHandler(bot.HandlerTypeMessageText, "/help", bot.MatchTypePrefix, b.onHelp)
	api.RegisterHandler(bot.HandlerTypeMessageText, "/stats", bot.MatchTypePrefix, b.onStats)
	api.RegisterHandler(bot.HandlerTypeMessageText, "/banya", bot.MatchTypePrefix, b.onBanya)
	api.RegisterHandler(bot.HandlerTypeMessageText, "/gryaznuli", bot.MatchTypePrefix, b.onDirty)
	api.RegisterHandler(bot.HandlerTypeMessageText, "/fact", bot.MatchTypePrefix, b.onFact)
	api.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypePrefix, b.onStart)
	api.RegisterHandler(bot.HandlerTypeCallbackQueryData, "setup:", bot.MatchTypePrefix, b.onSetupCallback)
	return b, nil
}

func (b *Bot) Start(ctx context.Context) {
	b.API.Start(ctx)
}

const shameGifID = "CgACAgIAAxkBAANIauGPioUwlSrUvbG0P8pX0VD88nsAAsoLAAIVBNFIZUEiJQor21I9BA"

func (b *Bot) defaultHandler(ctx context.Context, api *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}
	b.remember(update.Message.Chat.ID, update.Message.From)
	b.handleSetupText(ctx, api, update.Message)
}

func (b *Bot) remember(chatID int64, user *models.User) {
	if chatID != b.Cfg.BanyaChatID || user == nil || user.IsBot {
		return
	}
	_ = b.Store.UpsertMember(db.Member{
		ChatID: chatID,
		UserID: user.ID,
		Name:   displayName(user),
	})
}

func (b *Bot) SendFact(ctx context.Context, chatID int64, text string) error {
	photo := b.factPhoto(ctx, text)
	_, err := b.API.SendPhoto(ctx, &bot.SendPhotoParams{
		ChatID: chatID,
		Photo: &models.InputFileUpload{
			Filename: photoName(photo),
			Data:     bytes.NewReader(photo),
		},
		Caption: text,
	})
	return err
}

func (b *Bot) factPhoto(ctx context.Context, text string) []byte {
	client := &pictures.Client{}
	img, err := client.Yandex(ctx, text)
	if err != nil || len(img) == 0 {
		slog.Warn("картинка с яндекса не нашлась", "err", err)
		img, err = client.Find(ctx, text)
	}
	if err != nil || len(img) == 0 {
		slog.Warn("картинка с commons не нашлась", "err", err)
		return banyaPhoto
	}
	return img
}

func photoName(img []byte) string {
	if len(img) > 4 && img[0] == 0x89 && img[1] == 'P' {
		return "banya.png"
	}
	return "banya.jpg"
}

func (b *Bot) SendShame(ctx context.Context, chatID int64, text string) error {
	_, err := b.API.SendAnimation(ctx, &bot.SendAnimationParams{
		ChatID: chatID,
		Animation: &models.InputFileString{
			Data: shameGifID,
		},
		Caption: text,
	})
	return err
}

func (b *Bot) SendText(ctx context.Context, chatID int64, text string) error {
	_, err := b.API.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	})
	return err
}

func (b *Bot) SendPoll(ctx context.Context, chatID int64, question string, options []string) (string, int, error) {
	opts := make([]models.InputPollOption, 0, len(options))
	for _, o := range options {
		opts = append(opts, models.InputPollOption{Text: o})
	}
	msg, err := b.API.SendPoll(ctx, &bot.SendPollParams{
		ChatID:      chatID,
		Question:    question,
		Options:     opts,
		IsAnonymous: bot.False(),
	})
	if err != nil {
		return "", 0, err
	}
	if msg.Poll == nil {
		return "", 0, fmt.Errorf("telegram returned message without poll")
	}
	return msg.Poll.ID, msg.ID, nil
}

func (b *Bot) onPollAnswer(ctx context.Context, api *bot.Bot, update *models.Update) {
	ans := update.PollAnswer
	if ans == nil || ans.User == nil {
		return
	}
	week, err := b.Store.GetWeekByPollID(ans.PollID)
	if err != nil {
		return
	}
	if week.ChatID != b.Cfg.BanyaChatID {
		return
	}
	b.remember(week.ChatID, ans.User)
	if len(ans.OptionIDs) == 0 {
		_ = b.Store.DeleteVote(week.WeekID, ans.User.ID)
		return
	}
	optID := ans.OptionIDs[0]
	_ = b.Store.UpsertVote(db.Vote{
		WeekID:     week.WeekID,
		UserID:     ans.User.ID,
		Username:   displayName(ans.User),
		OptionID:   optID,
		OptionText: banya.OptionText(optID),
		UpdatedAt:  time.Now().In(b.Cfg.Location).Format(time.RFC3339),
	})
}

func (b *Bot) onHelp(ctx context.Context, api *bot.Bot, update *models.Update) {
	msg := update.Message
	if msg == nil || !b.allowedChat(msg.Chat.ID, msg.From) {
		return
	}
	_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   banya.HelpText(),
	})
}

func (b *Bot) onStats(ctx context.Context, api *bot.Bot, update *models.Update) {
	msg := update.Message
	if msg == nil || !b.allowedChat(msg.Chat.ID, msg.From) {
		return
	}
	text := strings.TrimSpace(msg.Text)
	wantLast := strings.Contains(strings.ToLower(text), "last")
	now := time.Now().In(b.Cfg.Location)
	weekID := db.WeekID(now)

	var (
		week db.Week
		err  error
	)
	if wantLast {
		week, err = b.Store.LatestWeekBefore(b.Cfg.BanyaChatID, weekID)
		if errors.Is(err, sql.ErrNoRows) {
			_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: msg.Chat.ID,
				Text:   "Прошлой недели в журнале нет. Баньщик ещё не вёл счёт.",
			})
			return
		}
	} else {
		week, err = b.Store.GetWeek(weekID, b.Cfg.BanyaChatID)
		if errors.Is(err, sql.ErrNoRows) {
			_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: msg.Chat.ID,
				Text:   "На эту неделю опроса ещё нет. Жди вторника или зови админа с /banya.",
			})
			return
		}
	}
	if err != nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "Журнал бани молчит. Попробуй позже."})
		return
	}

	votes, err := b.Store.VotesForWeek(week.WeekID)
	if err != nil {
		return
	}
	_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   formatStats(week.WeekID, votes),
	})
}

func (b *Bot) onFact(ctx context.Context, api *bot.Bot, update *models.Update) {
	msg := update.Message
	if msg == nil || msg.From == nil {
		return
	}
	if !b.Cfg.IsAdmin(msg.From.ID) || !b.allowedChat(msg.Chat.ID, msg.From) {
		return
	}
	if err := b.Ritual.PostFact(ctx); err != nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "Факт не вышел: " + err.Error(),
		})
	}
}

func (b *Bot) onDirty(ctx context.Context, api *bot.Bot, update *models.Update) {
	msg := update.Message
	if msg == nil || msg.From == nil {
		return
	}
	if !b.Cfg.IsAdmin(msg.From.ID) || !b.allowedChat(msg.Chat.ID, msg.From) {
		return
	}
	b.remember(b.Cfg.BanyaChatID, msg.From)
	weekID := db.WeekID(time.Now().In(b.Cfg.Location))
	names, err := b.Ritual.Absent(b.Cfg.BanyaChatID, weekID)
	if err != nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "Список грязнуль не собрался: " + err.Error(),
		})
		return
	}
	if len(names) == 0 {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "Все, кого баньщик видел, уже проголосовали. Остальных он не знает.",
		})
		return
	}
	if err := b.SendShame(ctx, msg.Chat.ID, banya.DirtyList(names)); err != nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "Гиф не ушла: " + err.Error(),
		})
	}
}

func (b *Bot) onBanya(ctx context.Context, api *bot.Bot, update *models.Update) {
	msg := update.Message
	if msg == nil || msg.From == nil {
		return
	}
	if !b.Cfg.IsAdmin(msg.From.ID) {
		return
	}
	if !b.allowedChat(msg.Chat.ID, msg.From) {
		return
	}
	err := b.Ritual.Run(ctx)
	if errors.Is(err, banya.ErrAlreadyPosted) {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   banya.AlreadyPosted(db.WeekID(time.Now().In(b.Cfg.Location))),
		})
		return
	}
	if err != nil {
		_, _ = api.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "Ритуал сорвался: " + err.Error(),
		})
	}
}

func (b *Bot) allowedChat(chatID int64, from *models.User) bool {
	if chatID == b.Cfg.BanyaChatID {
		return true
	}
	return from != nil && b.Cfg.IsAdmin(from.ID)
}

func displayName(u *models.User) string {
	if u.Username != "" {
		return "@" + u.Username
	}
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if name == "" {
		return fmt.Sprintf("id:%d", u.ID)
	}
	return name
}

func formatStats(weekID string, votes []db.Vote) string {
	var going, notGoing []string
	for _, v := range votes {
		label := v.Username
		if label == "" {
			label = fmt.Sprintf("id:%d", v.UserID)
		}
		switch v.OptionText {
		case banya.OptionGoing:
			going = append(going, label)
		default:
			notGoing = append(notGoing, label)
		}
	}
	var b strings.Builder
	b.WriteString(banya.StatsHeader(weekID))
	b.WriteString("\nИду (" + fmt.Sprintf("%d", len(going)) + "):\n")
	if len(going) == 0 {
		b.WriteString("никто. Стыд.\n")
	} else {
		for _, n := range going {
			b.WriteString("• " + n + "\n")
		}
	}
	b.WriteString("\nНе иду (" + fmt.Sprintf("%d", len(notGoing)) + "):\n")
	if len(notGoing) == 0 {
		b.WriteString("пока пусто.\n")
	} else {
		for _, n := range notGoing {
			b.WriteString("• " + n + "\n")
		}
	}
	return b.String()
}
