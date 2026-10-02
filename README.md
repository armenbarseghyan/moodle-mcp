# moodle-mcp

Read-only MCP-сервер для Moodle (FH JOANNEUM, Moodle 4.5) на Go. Отвечает на учебные
вопросы — что сдать, что написали преподаватели, где лежит материал — собирая несколько
вызовов Moodle Web Services под капотом. Ничего в Moodle не меняет: клиент физически не
может вызвать функцию вне allowlist из 9 getter-функций.

Архитектура и принятые решения — [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Установка

Нужен Go 1.25+ (с `GOTOOLCHAIN=auto` нужная версия скачается сама).

```bash
make build        # → bin/moodle-mcp
```

## Переменные окружения

| Переменная | Обязательна | По умолчанию | Что это |
|---|---|---|---|
| `MOODLE_URL` | да | — | адрес сайта, `https://moodle.fh-joanneum.at` |
| `MOODLE_TOKEN` | да | — | персональный токен сервиса `moodle_mobile_app` |
| `MOODLE_DOWNLOAD_DIR` | нет | `~/Downloads/moodle` | куда `moodle_download` кладёт файлы |
| `MOODLE_LOG_LEVEL` | нет | `info` | `debug`, `info`, `warn`, `error` |
| `MCP_HTTP_ADDR` | нет | — | то же, что флаг `-http`: слушать streamable HTTP вместо stdio |
| `MCP_HTTP_TOKEN` | для не-loopback адреса | — | bearer-токен, который HTTP-клиенты должны присылать |

Токен читается только из окружения. Логи пишутся в stderr; токен в них, в ошибках и в
ответах инструментов маскируется (`[REDACTED]`). Если переменные не заданы, сервер всё
равно стартует и каждый инструмент объясняет, чего не хватает.

Токен: Moodle → Profil → Einstellungen → Sicherheitsschlüssel, сервис *Moodle mobile web service*.

## Подключение к Claude Code

```bash
claude mcp add moodle --scope user \
  -e MOODLE_URL=https://moodle.fh-joanneum.at \
  -e MOODLE_TOKEN=<твой токен> \
  -- /абсолютный/путь/к/moodle-mcp/bin/moodle-mcp
```

Проверка: `moodle_whoami` покажет пользователя, версию сайта и каких функций не хватает.

## HTTP вместо stdio

```bash
MCP_HTTP_TOKEN=<секрет> ./bin/moodle-mcp -http 127.0.0.1:8765
```

Эндпоинт — `http://127.0.0.1:8765/mcp` (streamable HTTP), `GET /healthz` — проверка живости.
Защита: DNS-rebinding (запросы на localhost с чужим `Host` отклоняются), cross-origin запросы
браузера отклоняются, с `MCP_HTTP_TOKEN` каждый запрос требует `Authorization: Bearer …`.
Слушать не-loopback адрес (`0.0.0.0`, LAN) без токена сервер отказывается: он держит твой токен Moodle.

```bash
claude mcp add moodle-http --transport http http://127.0.0.1:8765/mcp --header "Authorization: Bearer <секрет>"
```

## Инструменты

| Инструмент | Параметры | Что делает |
|---|---|---|
| `moodle_deadlines` | `days=14`, `include_overdue=true`, `refresh` | Дедлайны до конца N-го дня (Вена): календарь + сроки заданий, дедупликация, статус сдачи у каждого задания, просроченные несданные за 7 дней |
| `moodle_announcements` | `days=7`, `refresh` | Свежие посты из форумов-объявлений всех курсов |
| `moodle_courses` | `include_past`, `refresh` | Активные курсы: id, название, даты, прогресс |
| `moodle_course_contents` | `course`, `refresh` | Материалы курса по разделам (с подразделами Moodle 4.5) и ссылками на файлы. `course` — id или кусок названия; при неоднозначности — список кандидатов |
| `moodle_search` | `query`, `course=""`, `in_files=false`, `refresh` | Поиск по названиям, описаниям, разделам и именам файлов во всех активных курсах (или в одном). С `in_files=true` — ещё и по тексту PDF, docx, pptx, ipynb, zip и страниц Moodle, с номерами страниц и цитатой |
| `moodle_grades` | `course=""` | Оценки: балл, максимум, процент, фидбек; по курсу или по всем |
| `moodle_download` | `fileurl`, `dest=""` | Скачивает файл этого Moodle, возвращает локальный путь |
| `moodle_whoami` | — | Диагностика: пользователь, сайт, версия, доступные функции |

Вывод — компактный markdown. Даты — `2026-10-07 17:15 (среда) — через 5 дней`, Europe/Vienna.
Ссылки на файлы — браузерные (`…/pluginfile.php/…`): открываются там, где ты залогинен в Moodle,
токена в них нет.

Кэш в памяти: курсы, содержимое курсов, форумы — 15 мин; дедлайны, статусы сдачи,
объявления — 5 мин; оценки не кэшируются; текст файлов для `in_files` — пока файл не изменился
(URL + размер + время изменения). `refresh=true` обходит кэш.

Поиск внутри файлов: первый вызов скачивает файлы курсов в память (на реальных 8 курсах —
18 файлов, ~4 с), дальше работает из кэша. Файлы больше 40 МБ, видео и картинки пропускаются.
PDF читается без внешних утилит; пробелы восстанавливаются по координатам глифов, а слова от
6 букв сопоставляются и без учёта пробелов — PDF часто теряет или вставляет их.

## Разработка

```bash
make test      # go test -race ./...
make cover     # покрытие
make lint      # go vet + golangci-lint v2 (через go run, ставить не нужно)
make golden    # перезаписать эталонный вывод инструментов (testdata/golden)
```

Тесты не ходят в сеть: фейковый Moodle (`internal/moodletest`) отдаёт фикстуры
`testdata/moodle/<wsfunction>.<scenario>.json` — реальные ответы (обезличенные, `*.real*`)
и синтетические по схемам Moodle 4.5 (`*.synthetic*`).
