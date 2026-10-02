# moodle-mcp — архитектура данных

Статус: согласовано 2026-10-02.

## 1. Слои и правило зависимостей

```
            ┌──────────────────────────── cmd/moodle-mcp ───────────────────────────┐
            │ env → Config → moodle.Client → cached Source → study.Service →        │
            │ tools.Register(mcp.Server) → transport (stdio; этап 2: streamable HTTP)│
            └───────────────────────────────────────────────────────────────────────┘
                                         │
  internal/tools    MCP-адаптер: схемы входа, вызов study, ошибка → IsError-результат
        │
  internal/study    use-cases: агрегация, merge, резолв, политика ошибок.  Чистая логика.
        │      └──► internal/render   доменные модели → markdown (без I/O)
        │      └──► internal/textfmt  даты Vienna, «через N дней», HTML → текст
        ▼
  study.Source (interface) ◄── internal/cache.Source (декоратор: TTL + singleflight)
        ▲                               │
        └────────── internal/moodle ◄───┘   транспорт + wire-типы + ошибки Moodle
```

Правила:
- `moodle` ничего не знает о MCP, markdown и кэше. Отдаёт wire-типы (почти 1:1 с JSON).
- `study` ничего не знает о MCP и HTTP. На вход получает `Source` и часы, возвращает доменные модели.
- `render` — чистые функции `Model → string`. Тестируются golden-файлами.
- `tools` — тонкий слой: 8 обработчиков по 5–10 строк. На этапе 2 меняется только `cmd/`.

> Изменение относительно первого наброска: логику я вынес из `internal/tools` в
> `internal/study` + `internal/render`. Тогда в `tools` остаются только
> «определения инструментов», как ты и формулировал, а логика тестируется без MCP.

## 2. Получение данных (internal/moodle)

### 2.1 Транспорт
- **POST** `application/x-www-form-urlencoded` на `/webservice/rest/server.php`.
  Токен идёт в теле, а не в URL, поэтому он не попадает в `url.Error`, логи прокси и историю.
- Массивы передаются как `courseids[0]=…&courseids[1]=…`. Для этого есть хелпер `params.IntList("courseids", ids)`.
- `http.Client{Timeout: 20s}` для REST. Для скачивания файлов отдельный клиент: таймаут 5 мин
  на тело ответа и 20 с на заголовки. Иначе PDF на 50 МБ никогда не скачается.
- **Allowlist функций.** `Call` отказывает (`ErrNotAllowed`) любому `wsfunction` не из списка.
  Тест дополнительно проверяет, что в списке нет имён по шаблону
  `_(submit|save|add|update|delete|create|set|send|mark|edit|remove|toggle)_`.
- **Глобальный семафор на 5 запросов в полёте** внутри `Client`. Лимит общий для всех
  инструментов. Если `errgroup.SetLimit(5)` стоит только внутри одного инструмента,
  два параллельных вызова дадут 10 запросов.

### 2.2 Ретрай
| Ситуация | Ретрай? |
|---|---|
| сетевая ошибка (dial, reset, EOF), кроме отмены ctx | 1 раз |
| HTTP 5xx | 1 раз |
| таймаут 20 с | 1 раз (новый дедлайн) |
| HTTP 4xx, Moodle exception, ошибка декодирования | нет |
| `ctx.Done()` | нет, сразу `ctx.Err()` |

Бэкофф: 300 мс ± 30 % джиттера, задаётся в `Config.RetryDelay` (в тестах 0).

### 2.3 Декодирование ответа
1. Тело читается целиком с лимитом 32 МБ (`io.LimitReader`).
2. Если тело не начинается с `{` или `[` (а это HTML-страница обслуживания или
   ответ прокси), возвращается `ErrUnexpectedResponse` с первыми 200 символами тела после redact.
3. Если это объект, сначала пробный разбор в `struct{Exception, ErrorCode, Message, DebugInfo *string}`.
   Если `exception != nil`, возвращается `*moodle.Error`.
4. Иначе `json.Unmarshal(body, out)`.

### 2.4 Классификация ошибок (по `errorcode`, сообщения у нас на немецком)
| errorcode | sentinel | что видит пользователь | фатальна для запроса* |
|---|---|---|---|
| `invalidtoken` | `ErrInvalidToken` | «Токен Moodle недействителен или отозван. Создай новый: Profil → Sicherheitsschlüssel, обнови MOODLE_TOKEN.» | да |
| `accessexception` | `ErrAccessDenied` | «Функция X недоступна для сервиса moodle_mobile_app (access control).» | да |
| `requireloginerror` | `ErrNotAccessible` | «Курс/активность недоступны (скрыты или ограничены).» | нет |
| `sitemaintenance` | `ErrMaintenance` | «Moodle на обслуживании.» | да |
| прочие | `*Error` | `moodle: <fn>: <errorcode>: <message>` | нет |

\* «Фатальна» значит, что fan-out прерывается (`errgroup` отменяет контекст) и инструмент
возвращает одну ошибку. Нефатальная ошибка по одному курсу превращается в строку
«⚠ курс X: недоступен» в выводе, остальные курсы показываются.
Хелпер: `moodle.IsFatal(err) bool`.

`*Error` реализует `Is(target)`, поэтому работает `errors.Is(err, moodle.ErrInvalidToken)`.

### 2.5 Wire-типы
- Объявляются **только поля, которые мы используем**. Остальные игнорируются. Так wire-тип
  одновременно служит контрактом: видно, на что мы опираемся.
- Moodle непоследователен в типах. Пример из реальных ответов:
  `visible: 1`, а в соседнем поле `hidden: false`; `progress: null`; `lastaccess` бывает пустым. Поэтому:
  - `moodle.Bool` принимает `true/false/0/1/"0"/"1"/null`;
  - `moodle.Time` принимает unix-секунды; `0` и `null` дают нулевое время (`IsZero()`);
  - `*float64` для `progress`, `graderaw`.
- Предупреждения (`warnings[]`) не теряются: методы возвращают их вторым значением.

### 2.6 Методы клиента
```go
SiteInfo(ctx) (*SiteInfo, error)
UserCourses(ctx, userID) ([]Course, error)
CourseContents(ctx, courseID) ([]Section, error)
ActionEvents(ctx, from, to time.Time) ([]Event, error)          // пагинация aftereventid, limitnum=50
Assignments(ctx, courseIDs []int) ([]CourseAssignments, []Warning, error)  // ОДИН вызов на все курсы
SubmissionStatus(ctx, assignID) (*SubmissionStatus, error)
GradeItems(ctx, courseID, userID) ([]GradeItem, error)
Forums(ctx, courseIDs []int) ([]Forum, error)                    // один вызов на все курсы
Discussions(ctx, forumID, perPage) ([]Discussion, error)         // sortorder=3 (CREATED_DESC), page=0
Download(ctx, fileURL, dest string) (Downloaded, error)
```

## 3. Кэш (internal/cache)

- Кэшируются **данные из Moodle, а не готовый текст**. Рендер выполняется на каждый вызов,
  поэтому «через 4 дня» всегда считается от текущего момента.
- Реализован как декоратор `cache.Source`, который реализует `study.Source` поверх `moodle.Client`.
- Generic-ядро: `TTL[K,V]` с подменяемыми часами и `singleflight` на ключ: 5 параллельных
  запросов одного курса дают один HTTP-вызов. Ошибки не кэшируются.
- Обход кэша: `refresh=true` загружает данные заново и **перезаписывает** запись (а не просто читает мимо кэша).

| Ключ | TTL |
|---|---|
| `siteinfo` | до конца жизни процесса (нужен userid), повтор при ошибке |
| `courses` | 15 мин |
| `contents:<courseid>` | 15 мин |
| `forums:<sorted ids>` | 15 мин |
| `events:<from-day>:<to-day>` | 5 мин |
| `assignments:<sorted ids>` | 5 мин |
| `subst:<assignid>` | 5 мин |
| `discussions:<forumid>` | 5 мин |
| оценки | **не кэшируются**: всегда свежий запрос |

`study` получает от `Source` метку `FetchedAt`. Если данные старше 1 мин, render добавляет
в конец вывода строку «_данные из кэша, 7 мин назад_», чтобы было понятно, когда нужен `refresh`.

## 4. Доменные модели (internal/study)

Все времена хранятся как `time.Time` в Europe/Vienna, строки уже без HTML, пустое
означает «не выводить».

```go
type Course struct { ID int; Name, Short string /* Short="" если совпадает с Name */
    Start, End time.Time; Progress *float64; Past bool; URL string }

type Section struct { Num int; Name, Summary string; Items []Item; Hidden int /* кол-во скрытых */ }
type Item struct { CMID int; Kind, Name, Description, URL string; Files []File; Sub *Section /* подсекция */ }
type File struct { Name, URL string /* браузерный URL: /pluginfile.php/… без /webservice и без токена */; Size int64; MIME string }

type Deadline struct {
    Key      DeadlineKey   // {Module string; Instance int}: ключ дедупликации
    Course   CourseRef
    Title, Kind, URL string
    Due      time.Time     // итоговый срок после всех правил
    Sources  SourceSet     // calendar | assign (для отладки и тестов)
    Status   Submission    // Unknown | NotSubmitted | Draft | Submitted | Reopened | NotApplicable
    Overdue  bool
}

type Grade struct { Course CourseRef; Item, Kind, Display string; Max float64; Percent *float64; Feedback string; GradedAt time.Time }
type Announcement struct { Course CourseRef; Subject, Author, Text, URL string; Posted time.Time; Unread bool }
type Hit struct { Course CourseRef; SectionPath []string; Item Item; Field MatchField; Snippet string }
```

## 5. Потоки данных по инструментам

Общий шаг почти везде: `activeCourses()` = `Source.Courses()` → фильтр `!Past`.
Правило Past: `hidden || completed || (!End.IsZero() && End < now)`.

### moodle_courses(include_past, refresh)
`Courses` → фильтр → сортировка по Name → render.

### moodle_deadlines(days=14, include_overdue=true, refresh)
```
window = [now, конец дня (now + days) по Vienna]
overdueWindow = [now − 7д, now)                       ← только для НЕсданных assign
        ┌─ ActionEvents(window.from − 7д, window.to) ──┐
parallel┤                                              ├→ Merge → [assign: SubmissionStatus ×N (≤5)] → filter → sort → render
        └─ Assignments(activeIDs)  (один вызов) ───────┘
```
Правила merge (ключ `(modulename, instance)`; у assign `instance == assignment.id`):
1. Событие календаря и задание с тем же ключом дают одну запись. Название и URL берутся из
   календаря (оно учитывает overrides), курс из задания.
2. **Срок**: `extensionduedate` из статуса сдачи > `timesort` календаря > `duedate` задания.
   Календарь учитывает индивидуальные overrides, поэтому он приоритетнее `duedate`.
3. Задание **без события** сохраняется. Это важно: action-event у assign пропадает после
   сдачи, и без второго источника сданное задание просто исчезло бы из списка.
4. `duedate == 0` (срока нет) — не дедлайн, выбрасывается.
5. Не-assign события (quiz, choice, …) получают статус `NotApplicable`, сабмишен для них не запрашивается.
6. `warnings` «No access rights» превращаются в сноску «ещё N заданий скрыты/недоступны».

Статус: `lastattempt.submission` (или `teamsubmission`, если включены командные сдачи) → `status`:
`new`→«не сдано», `draft`→«черновик, не отправлен!», `submitted`→«сдано», `reopened`→«переоткрыто».
Если `graded` — «сдано, оценено».

Сортировка: `Due`, затем курс, затем название. Просроченные идут отдельным блоком сверху.

### moodle_course_contents(course, refresh)
`ResolveCourse(q)` → `CourseContents(id)` → `BuildTree`:
- Секции с `component == "mod_subsection"` удаляются из верхнего уровня и подвешиваются к
  модулю-подсекции по `module.customdata.sectionid == section.id` (проверено на реальном курсе 12326).
- `uservisible == false` → `Section.Hidden++`, модуль не выводится.
- Пустые секции (нет видимых модулей и summary) не выводятся.
- `label` выводится как текст (очищенный, до 200 символов).

**ResolveCourse(q)**: `q` целиком из цифр → точный ID (только среди своих курсов) →
нормализованное совпадение имени целиком → подстрока → все токены q как подстроки.
На первом шаге, давшем ровно 1 результат, — возврат. Если результатов >1, `*AmbiguousError{Candidates}`
(render покажет список с id). Если 0 — `*NotFoundError` со списком всех активных курсов.
Сначала поиск идёт по активным курсам, при 0 совпадений — по прошлым (с пометкой).
Нормализация: lower, NFD без диакритики, `ß→ss`, `ä→a` (и `ae→a`, чтобы «Pruefung» нашла «Prüfung»), схлопывание пробелов.

### moodle_search(query, refresh)
`activeCourses` → `CourseContents ×N` (errgroup ≤5, кэш) → плоский индекс → матч.
Поля и вес: имя модуля 3, имя файла 2, имя секции 1, описание 1. Все токены запроса
должны встретиться (AND). Топ-20, затем «ещё N совпадений, уточни запрос».

### moodle_grades(course="")
Один курс: `GradeItems(id, userid)`. Все курсы: fan-out ≤5.
- `gradeishidden` → пропуск.
- **Процент считаем сами**: `(graderaw − grademin)/(grademax − grademin)`. `percentageformatted`
  локализован («87,50 %») и для шкал бывает пустым.
- `Display = gradeformatted` (подходит и для шкал/букв). «-» означает «не оценено».
- Курс, где нет ни одной оценки и ни одного фидбека, сворачивается в «оценок пока нет» одной строкой.
- Итог курса (`itemtype=course`) выводится последней строкой.

### moodle_announcements(days=7, refresh)
`Forums(activeIDs)` → `type == "news"` → `Discussions(forumid, perPage=10)` ×N (≤5)
→ `created ≥ now − days` (или `modified`, если пост редактировали) → сортировка по убыванию → render.
Текст: StripHTML, обрезка до 1500 символов + «… [полностью](url)». Закреплённые помечаются 📌.
URL: `/mod/forum/discuss.php?d=<discussion>`.

### moodle_download(fileurl, dest="")
```
parse → host == host(MOODLE_URL)? иначе ОТКАЗ (токен никогда не уходит на чужой хост)
      → path содержит /pluginfile.php/ ? иначе отказ
      → /pluginfile.php/… → /webservice/pluginfile.php/…  (уже webservice — без изменений)
      → удалить token из query, если он был; добавить свой
      → GET (отдельный клиент) → статус 200? JSON-тело → разбор как ошибки Moodle
      → имя: Content-Disposition → последний сегмент пути (url-decode) → sanitize
      → запись во временный файл в dest → fsync → rename (атомарно, без битых файлов)
```
- `dest`: пусто → `$MOODLE_DOWNLOAD_DIR` или `~/Downloads/moodle`. Если `dest` — существующая
  папка или кончается на `/`, файл кладётся внутрь. Иначе `dest` считается полным путём файла.
- Если файл уже существует: при совпадении размера и `Last-Modified` возвращаем «уже скачан»,
  иначе пишем в `name (1).ext`.
- sanitize: только базовое имя, без `..`, `/`, управляющих символов, длина ≤ 200.
- Возврат: локальный путь + размер. URL в ответе всегда без токена.

### moodle_whoami()
`SiteInfo` → имя, логин, сайт, версия, число функций. Плюс **проверка: какие из
функций, нужных серверу, отсутствуют**. Это первый инструмент для диагностики.

## 6. Время и текст (internal/textfmt)

- `Vienna` загружается через `time.LoadLocation`. В бинарник встраивается `time/tzdata`, чтобы
  он не зависел от системной базы часовых поясов.
- `Date(t)` → `2026-10-07 17:15 (среда)`.
- `Relative(t, now)` считается по **календарным дням в Vienna**, а не по 24 ч:
  менее 1 ч → «через 40 мин», в тот же день → «сегодня, через 3 ч», 1 день → «завтра»,
  N дней → «через N дн.» с правильным склонением (1 день / 2–4 дня / 5–20 дней / 21 день).
  В прошлом: «вчера», «3 дня назад».
  Переход на зимнее время 25.10.2026 входит в тест-кейсы.
- `StripHTML` использует токенизатор `golang.org/x/net/html`, а не regex:
  блочные теги и `<br>` → перевод строки, `<li>` → «- », entities декодируются, `&nbsp;` → пробел,
  пробелы схлопываются, `<script>/<style>` удаляются целиком. Moodle multilang
  (`<span lang="xx" class="multilang">`): берётся `de`, если есть, иначе первый вариант.
- `Truncate(s, n)` режет по рунам, а не по байтам, с «…».

## 7. Формат вывода (internal/render)

Общие правила: markdown; у каждой сущности ID, чтобы модель могла сделать следующий вызов;
пустые поля не выводятся; никаких unix-времён; лимит около 12 000 символов на ответ с хвостом «… ещё N».

```markdown
## Дедлайны: 14 дней (до 2026-10-16)

**Просрочено**
- ❌ 2026-09-30 23:59 (среда) — 2 дня назад · Programming and Data Processing · [Homework R0](…) · assign · не сдано

- 2026-10-05 23:59 (понедельник) — через 3 дня · Refresher on Unix Shells and LaTeX · [Quiz LaTeX Basics](…) · quiz
- 2026-10-07 17:15 (среда) — через 5 дней · Programming and Data Processing · [Homework R1](…) · assign · ✅ сдано
- 2026-10-10 10:00 (суббота) — через 8 дней · Programming and Data Processing · [Homework R2](…) · assign · ❌ не сдано

_Ещё 1 задание скрыто или недоступно._
```

Префикс `(DAT_WS2026_1)` повторяется во всех курсах, поэтому в списках убирается **общий
префикс всех курсов** (алгоритм, а не захардкоженная строка). В `moodle_courses`
выводится полное имя.

### Ссылки на файлы
Основной способ работы с материалами — **ссылка, которую пользователь открывает в браузере**
(он залогинен в Moodle). Поэтому во всём выводе:
- у модуля ссылка на страницу `…/mod/<type>/view.php?id=<cmid>`;
- у файла **браузерная** ссылка `…/pluginfile.php/…`: `/webservice/` убирается, `forcedownload`
  убирается (PDF откроется во вкладке), токена нет никогда;
- `moodle_download` остаётся для случаев, когда файл нужен локально (например, прочитать его
  содержимое); он принимает обе формы ссылки.

## 8. Ошибки на границе MCP

- Ошибки из `study` возвращаются как `CallToolResult{IsError: true, Content: [текст]}`,
  а не как protocol error. Так модель видит человеческую причину и может среагировать
  (например, предложить обновить токен).
- `AmbiguousError` — не ошибка: это нормальный результат со списком кандидатов.
- Все тексты ошибок проходят через `Redact`.

## 9. Безопасность и логи

- Токен существует только в `moodle.Client` (неэкспортируемое поле) и в одном месте сборки URL скачивания.
- `Redact(s)` заменяет токен и в сыром виде, и в URL-encoded. Применяется: в slog-хендлере
  (каждый атрибут), к текстам ошибок, к ответу download, к ErrUnexpectedResponse.
- `url.Error` от download содержит URL с токеном, поэтому оборачивается до возврата наверх.
- Логи: `slog` в stderr; поля `fn, attempt, status, dur_ms, cache=hit|miss|refresh, bytes`.
  Параметры запросов не логируются. Уровень задаётся через `MOODLE_LOG_LEVEL` (по умолчанию info).
- Stdout занят протоколом MCP. Ни одного `fmt.Print` в коде — это проверяет линтер `forbidigo`.

## 10. Конфигурация и старт

| env | обязат. | по умолчанию |
|---|---|---|
| `MOODLE_URL` | да | — |
| `MOODLE_TOKEN` | да | — |
| `MOODLE_DOWNLOAD_DIR` | нет | `~/Downloads/moodle` |
| `MOODLE_LOG_LEVEL` | нет | `info` |

При старте Moodle **не вызывается**: проверяется только наличие env и корректность URL.
Если упасть на старте, Claude Code покажет лишь «Connection closed» (как сейчас).
Лучше стартовать и вернуть понятную ошибку из первого же инструмента.
Версия задаётся через `-ldflags "-X main.version=…"`.

## 11. Файлы

```
cmd/moodle-mcp/main.go            env, сборка графа, stdio
internal/moodle/
  client.go        Config, New, Call, ретрай, семафор, allowlist
  errors.go        Error, sentinels, IsFatal, классификация
  decode.go        Bool, Time, разбор exception
  types.go         wire-типы
  api.go           типизированные методы
  download.go      ToWebserviceURL, Download, sanitize, атомарная запись
  redact.go        Redact, RedactingHandler
  allowlist.go     список функций
internal/cache/
  ttl.go           TTL[K,V] + singleflight
  source.go        декоратор study.Source
internal/study/
  source.go        interface Source
  service.go       Service, New, activeCourses, fan-out helper
  model.go         доменные модели
  resolve.go       ResolveCourse, normalize
  deadlines.go     Merge, статус, окна
  contents.go      BuildTree
  search.go        индекс и ранжирование
  grades.go  announcements.go  download.go  whoami.go
internal/render/   по файлу на инструмент + common.go (лимит, префикс курсов)
internal/textfmt/  date.go relative.go html.go
internal/tools/    register.go (AddTool ×8), inputs.go (структуры входа с jsonschema-тегами)
testdata/moodle/<wsfunction>.<scenario>.json   фикстуры ответов
testdata/errors/<errorcode>.json               фикстуры ошибок
testdata/golden/<tool>.<scenario>.md           ожидаемый вывод
docs/  Makefile  README.md  .gitignore  .golangci.yml
```

## 12. Тесты

**Конвенции.** Табличные тесты `[]struct{name; …}` + `t.Run`, `t.Parallel()` везде, где нет
глобального состояния. Всё запускается с `-race`. Ни одного сетевого вызова: базовый URL
всегда берётся у `httptest.Server`.

**Фейковый Moodle** (`internal/moodletest`):
```go
srv := moodletest.New(t,
    moodletest.Fixture("core_enrol_get_users_courses", "real"),
    moodletest.Route("mod_assign_get_submission_status", func(p url.Values) moodletest.Resp {
        return moodletest.File("synthetic-" + statusByID[p.Get("assignid")])
    }),
    moodletest.Sequence("core_course_get_contents", moodletest.HTTP(502), moodletest.File("real-12308")),
)
srv.Calls("core_course_get_contents")   // счётчик для проверки кэша и singleflight
srv.Requests()                          // полные запросы: метод, токен в теле, а не в URL
```
Фикстуры ищутся по `<wsfunction>.<scenario>.json`. Если для функции не задан маршрут,
тест падает с понятным сообщением (а не получает 404).

| Уровень | Что | Как |
|---|---|---|
| moodle | разбор каждой фикстуры; exception → sentinel (табл. по `testdata/errors/*`); HTML-тело; 5xx→retry→ok; 5xx×2→err; 4xx без ретрая; таймаут; отмена ctx; allowlist; Bool/Time | httptest |
| moodle/download | URL-нормализация (табл.: pluginfile, webservice, чужой хост, без pluginfile, с ?token=, с forcedownload); имя из Content-Disposition; sanitize `../../etc`; атомарность; коллизии | httptest + `t.TempDir()` |
| redact | токен в URL, в url-encoded виде, в тексте ошибки, в логах (буфер slog), в выводе download | property: `assertNoToken(t, everyOutput)` |
| cache | hit/miss/expire по фейковым часам; refresh перезаписывает; ошибка не кэшируется; singleflight (10 горутин → 1 load) | фейковые часы, без sleep |
| textfmt | Date/Relative/склонения/DST/полночь; StripHTML (табл. ~20 кейсов из реальных описаний) | чистые |
| study | ResolveCourse (id, подстрока, регистр, умлауты, неоднозначность, прошлые); Merge (оба источника, только календарь, только assign, срок=0, приоритет срока, extension, вне окна, просрочка); BuildTree с подсекциями; поиск/ранжирование; политика частичных ошибок | фейковый Source, часы `2026-10-02 12:00 Vienna` |
| render | вывод каждого инструмента | golden-файлы, `go test ./... -update` |
| tools (e2e) | 8 инструментов через `mcp.NewInMemoryTransports()`: list_tools (схемы), call_tool → golden; invalidtoken → `IsError` с понятным текстом | httptest + реальный клиент + in-memory MCP |

**Makefile**: `build`, `test` (`-race -count=1`), `cover`, `lint` (golangci-lint:
govet, staticcheck, errcheck, gosec, forbidigo), `golden` (`-update`).

## 12a. Дополнения после первой версии

**Поиск внутри файлов** (`moodle_search(in_files=true)`):
- `internal/extract` — чистые функции «байты → текст»: PDF (`ledongthuc/pdf`, pure Go; строки и
  пробелы восстанавливаются по координатам глифов в порядке потока), HTML, docx/pptx (XML внутри
  zip), ipynb, текст и исходники, zip (одна вложенность, бюджет 64 МБ на архив, ≤500 записей).
  Сбой парсера — ошибка, а не паника (recover).
- `moodle.Client.FetchFile` читает pluginfile в память с лимитом; та же валидация URL, что у download.
- `cache.Source.FileText` кэширует текст навсегда по ключу URL + размер + mtime: изменённый файл
  перечитывается сам.
- Сопоставление: слова ищутся подстрокой в нормализованном тексте с пробелами (пропавший пробел
  «offcampus» не мешает); слова от 6 букв — ещё и в тексте без пробелов («di ff erent»). Короче —
  нельзя: «ssh» нашлось бы в «cla**ss h**ierarchy».
- Результат по файлу: страницы/слайды/записи архива, где есть все слова (части с некоторыми
  словами — только если полных нет), и цитата — строка с наибольшим числом слов.

**Транспорт HTTP** (`internal/server`): один `*mcp.Server` для stdio и streamable HTTP. Слои
защиты: DNS-rebinding (SDK), `http.CrossOriginProtection`, опциональный bearer
(`auth.RequireBearerToken`, сравнение за постоянное время). Не-loopback адрес без токена — отказ
на старте. `/healthz` без авторизации.

**Обход бага go-sdk v1.8.0:** `"arguments": null` при применении default-значений схемы вызывает
панику (nil map в jsonschema-go) и роняет процесс. Middleware `nullArguments` подменяет `null` на `{}`.

**Найдено тестами под `-race`:** `transform.Chain` из `x/text` хранит состояние — общий экземпляр
в `Normalize` падал при параллельных вызовах; теперь создаётся на вызов.

## 13. Принятые решения

1. Окно «N дней» — **до конца N-го дня по Вене** (календарные дни, корректно при смене DST),
   а не N×24 ч: дедлайны в Moodle обычно на 23:59, результат не зависит от часа запроса.
2. Просроченные несданные assign за последние 7 дней показываются (`include_overdue=true`).
3. Оценки не кэшируются.
4. Общий префикс названий курсов в списках убирается.
5. `make contract` / живые тесты не делаем (живая проверка — вручную, smoke-клиентом).
6. Go: `GOTOOLCHAIN=auto` (go-sdk v1.8 требует 1.25).
