# instabot — злобный баньщик

Telegram-бот для группы: **по вторникам в 18:00 (Asia/Almaty)** шлёт факт про баню и неанонимный опрос «Иду / Не иду», пишет голоса в SQLite.

Стек: Go, SQLite (`modernc.org/sqlite`), long polling, cron внутри процесса. Один код для **Debian amd64** и **Raspberry Pi arm64**.

## GitHub Actions

Workflow [`.github/workflows/ci.yml`](.github/workflows/ci.yml):

- на PR и push: `go test ./...` и кросс-сборка `linux/amd64`, `linux/arm64`, `linux/armv7` (артефакт `instabot-linux`)
- на push в `main`/`master` (не на PR): multi-arch Docker (`linux/amd64,linux/arm64`) в **GHCR**: `ghcr.io/<owner>/<repo>`

Пакеты репозитория должны быть **public** или Pi логинится в GHCR. Пример на малинке:

```bash
echo "$GHCR_TOKEN" | docker login ghcr.io -u USER --password-stdin
docker pull ghcr.io/OWNER/instabot:latest
```

В `compose.yaml` можно заменить `build: .` на `image: ghcr.io/OWNER/instabot:latest`.

## Команды

| Команда | Кто | Что делает |
|---------|-----|------------|
| `/help` | все в группе | кратко |
| `/stats` | все в группе | голоса текущей недели |
| `/stats last` | все в группе | прошлый опрос из БД |
| `/banya` | `ADMIN_USER_IDS` | сразу факт + опрос (тест) |

Опрос **не анонимный** — иначе нет имён в статистике. Второй опрос на ту же ISO-неделю бот не создаёт.

## Настройка Telegram

1. [@BotFather](https://t.me/BotFather) → `/newbot` → токен в `BOT_TOKEN`.
2. Добавьте бота в группу, дайте право писать.
3. Узнать `BANYA_CHAT_ID`: напишите в группу, затем локально с токеном:
   `curl "https://api.telegram.org/bot$BOT_TOKEN/getUpdates"` — поле `chat.id` (у супергрупп часто `-100…`).
4. Свой user id — [@userinfobot](https://t.me/userinfobot) → `ADMIN_USER_IDS`.

```bash
cp .env.example .env
# заполните BOT_TOKEN, BANYA_CHAT_ID, ADMIN_USER_IDS
```

## Запуск: Debian desktop (amd64)

Docker:

```bash
docker compose up -d --build
docker compose logs -f
```

Без Docker:

```bash
make test
make build
export $(grep -v '^#' .env | xargs)
./bin/instabot
```

systemd: скопируйте бинарник, [deploy/instabot.service](deploy/instabot.service) и `/etc/instabot/env` (как `.env`).

## Запуск: Raspberry Pi (arm64)

На самой Pi (64-bit OS) — тот же `docker compose up -d --build`.

С десктопа, без сборки на Pi:

```bash
make docker-arm64
# или: ./scripts/docker-buildx.sh --push   # нужен registry и IMAGE=...
docker save instabot:arm64 | ssh pi@host docker load
```

Нативный бинарник:

```bash
make build-linux-arm64
scp bin/instabot-linux-arm64 pi@host:/usr/local/bin/instabot
```

Для 32-bit Pi OS: `make build-linux-armv7`.

Данные: каталог `./data` (в Docker смонтирован в `/data`). Бэкап — копия `data/banya.db`.

## Переменные

См. [.env.example](.env.example). `CRON` — пять полей (`мин час день месяц день_недели`), по умолчанию `0 18 * * 2`. `TZ` задаёт локаль cron.

## Roadmap

- стрейки «N недель подряд Иду»
- напоминание в день бани
- сводка в Google Sheets
- факты из поиска / LLM
