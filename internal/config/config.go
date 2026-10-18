package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	BotToken      string
	BanyaChatID   int64
	AdminUserIDs  map[int64]struct{}
	Location      *time.Location
	CronSpec      string
	DBPath        string
	FactsPath     string
}

func Load() (Config, error) {
	token := strings.TrimSpace(os.Getenv("BOT_TOKEN"))
	if token == "" {
		return Config{}, fmt.Errorf("BOT_TOKEN is required")
	}

	chatRaw := strings.TrimSpace(os.Getenv("BANYA_CHAT_ID"))
	if chatRaw == "" {
		return Config{}, fmt.Errorf("BANYA_CHAT_ID is required")
	}
	chatID, err := strconv.ParseInt(chatRaw, 10, 64)
	if err != nil {
		return Config{}, fmt.Errorf("BANYA_CHAT_ID: %w", err)
	}

	admins := make(map[int64]struct{})
	for _, part := range strings.Split(os.Getenv("ADMIN_USER_IDS"), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("ADMIN_USER_IDS: %w", err)
		}
		admins[id] = struct{}{}
	}

	tzName := strings.TrimSpace(os.Getenv("TZ"))
	if tzName == "" {
		tzName = "Asia/Almaty"
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return Config{}, fmt.Errorf("TZ: %w", err)
	}

	cronSpec := strings.TrimSpace(os.Getenv("CRON"))
	if cronSpec == "" {
		cronSpec = "0 18 * * 2"
	}

	dbPath := strings.TrimSpace(os.Getenv("DB_PATH"))
	if dbPath == "" {
		dbPath = "./data/banya.db"
	}

	return Config{
		BotToken:     token,
		BanyaChatID:  chatID,
		AdminUserIDs: admins,
		Location:     loc,
		CronSpec:     cronSpec,
		DBPath:       dbPath,
		FactsPath:    strings.TrimSpace(os.Getenv("FACTS_PATH")),
	}, nil
}

func (c Config) IsAdmin(userID int64) bool {
	_, ok := c.AdminUserIDs[userID]
	return ok
}
