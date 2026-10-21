# AGENTS.md

Telegram-бот «злобный баньщик»: один Go-бинарник, SQLite, long polling, cron внутри процесса.

## Layout / entrypoint

- `cmd/bot/main.go` — вся сборка приложения: конфиг → SQLite → факты → Telegram → cron.
- `internal/banya` — доменная логика (недельный ритуал, напоминание, выбор факта, тексты).
- `internal/db` — SQLite-хранилище; `internal/telegram` — API и обработчики; `internal/scheduler` — cron; `internal/gemini` — LLM-факты; `internal/pictures` — картинки из Wikimedia Commons.
- Модуль называется `banyabot`, импорты вида `banyabot/internal/...` (не github-путь).

## Commands

```bash
go test ./...                                  # быстрый и достаточный прогон
go test ./internal/banya -run TestName -v      # один тест
go vet ./...
make build                                     # -> bin/banyabot (gitignored)
```

Локальный запуск без Docker: приложение **не читает `.env` само**:

```bash
export $(grep -v '^#' .env | xargs) && ./bin/banyabot
```

Docker: `docker compose up -d --build`.

## Toolchain

- Нужен **Go 1.26+**: это требуют и `go.mod`, и `modernc.org/sqlite`/`libc`. В `Dockerfile` базовый образ должен быть `golang:1.26-*`; официальные golang-образы используют `GOTOOLCHAIN=local`, поэтому старый образ падает на `go mod download`.
- SQLite — `modernc.org/sqlite` (pure Go), поэтому `CGO_ENABLED=0` работает и нужен для кросс-сборки.

## Non-obvious behavior

- Настройки из БД `cron_spec` и `chat_id` **перекрывают** env при старте (`main.go`). Если расписание/группа уже заданы через `/start`, правка `CRON`/`BANYA_CHAT_ID` в `.env` ничего не изменит — меняйте ключи в таблице `settings`.
- Миграции встроены через `//go:embed *.sql`, но `db.Migrate()` читает **только `001_init.sql`**. Новый `.sql` сам по себе не применится; схема — только идемпотентные `CREATE TABLE IF NOT EXISTS`, без версий и `ALTER`.
- Опрос обязан оставаться неанонимным (`IsAnonymous: false`): голоса пишутся из `PollAnswer`, а `/stats` показывает имена. Порядок ритуала — сначала факт, потом опрос; повтор на ту же ISO-неделю запрещён (`ErrAlreadyPosted`).
- Два источника фактов: Gemini при заданном `GEMINI_API_KEY` (при ошибке — откат на JSON), иначе JSON. Корневой `facts.json` попадает в Docker через `FACTS_PATH`, а `internal/banya/facts.json` встроен в бинарник; файлы сейчас идентичны — правьте оба.
- Помимо настраиваемого недельного cron есть отдельная ежечасная задача напоминания; дедупликация — ключ `nagged:<weekID>` в `settings`. Cron 5-польный и работает в `TZ` из конфига, не в UTC.
- Все пользовательские тексты — русские, вынесены в `internal/banya/copy.go`, `internal/telegram/setup.go`, `internal/telegram/bot.go`. Сохраняйте тон «злобного баньщика». Таблица команд в README неполная; источник истины — регистрации хендлеров в `internal/telegram/bot.go` (`/help`, `/stats`, `/banya`, `/fact`, `/start`).

## Tests

- Тесты самодостаточны: `:memory:` SQLite, `httptest` для Gemini, сеть/токены не нужны. `go test ./...` — гейт CI.
- При добавлении метода в `banya.PollSender` нужно обновить `fakeSender` в `internal/banya/weekly_test.go`.

## Deploy

- CI (`.github/workflows/ci.yml`): тесты и кросс-сборка `linux/{amd64,arm64,armv7}` на PR/push; multi-arch образ в GHCR (`ghcr.io/<owner>/<repo>`) — только на push в `main`/`master`.
- Docker: multi-stage, `data/` монтируется в `/data`, `DB_PATH=/data/banya.db`; `.dockerignore` исключает `.env`, `data`, `bin`, `*.md`.
- ARM: `make docker-arm64`, `make build-linux-arm64` (32-bit — `build-linux-armv7`). Пуш в свой registry: `IMAGE=... ./scripts/docker-buildx.sh --push`.
