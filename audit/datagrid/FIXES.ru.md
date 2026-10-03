# DataGrid: первый этап исправлений

Исправления выполнены поверх публичной базы `7d51c83`, после сохранения аудита и контрактных тестов отдельным локальным коммитом `49e8746`. Архитектура и границы модулей сохранены. Новых зависимостей нет. Remote push, PR, merge и deploy не выполнялись.

## Проверки и локальные коммиты

Финальный frontend-прогон: **295 обычных passing tests, 0 expected failures**, 22 файла; manifest 28/28. Go tidy/format/vet/lint/race+coverage, frontend lint/format/typecheck, production build и demo build прошли. После сборки отдельно прошли embedded-assets/client-bundle и external-consumer проверки. Независимое ревью не оставило блокеров в согласованном объёме.

Изменения разделены локально:
- `49e8746` — baseline contracts/demo до исправлений
- `1cd5213` — backend SQL/query/state/SDK fixes
- `594792d` — frontend safety/interactions/state/CSV regression fixes
- `9081889` — обновлённые embedded assets

295 tests не включают браузерный или PostgreSQL runtime E2E: эти ограничения перечислены ниже.

## Что исправлено

- Richtext: экранирование текста и атрибутов, фиксированный набор генерируемых HTML-тегов, проверка структуры документа и уровня heading, разрешённые URL-схемы. Небезопасные ссылки не превращаются в активные ссылки. Даже ошибка разбора не возвращает сырой HTML.
- SQL helpers: все JSONB operands передаются параметрами; операторы `?`, `?|`, `?&`, `@?` корректно проходят PostgreSQL placeholder conversion. Негативные тесты проверяют SQL и arguments для всех 11 операторов. Columns/operators остаются доверенной конфигурацией host; реальные SQL-запросы к PostgreSQL в этом этапе не выполнялись.
- Backend state: date-фильтры, effective limit=10 и скалярные значения сохраняются в metadata/ссылках. Существующий тип `map[string]string` сохранён. Reserved names не перезаписывают параметры протокола. Search передаётся через `_search`.
- SDK/errors: добавлен публичный alias `ErrorData`; default HTML error теперь не раскрывает внутреннюю ошибку. Явные пользовательские error callbacks продолжают получать исходную ошибку и управлять HTTP-статусом.
- Ячейки и controls: подключён реальный Checkbox с API установленной Reka UI (`modelValue`), 0/false отображаются, строки без действий сохраняют action-cell. Sort headers используют focusable native buttons, `scope` и `aria-sort`.
- Запросы: единый debounce для search/filter, более поздняя пагинация/сортировка отменяет ожидающий debounce. Устаревшие ответы, в том числе с прежнего apiUrl, не перезаписывают состояние. Retry сохраняет запрос целиком.
- URL: initialData/fetch используют одинаковую hydration, поддерживается существующий глобальный dataResponse wrapper. Сохраняются hash/history.state и effective limit/order. Popstate обрабатывается с защитой от гонок. Когда в URL нет limit/sort, backend выбирает собственные defaults; retry сохраняет это отсутствие параметров, а обычный refresh сохраняет текущий запрос.
- Selection: `allSelected` проверяет ID видимых строк, select-all убирает дубликаты. Существующее очищение выбора после успешной загрузки сохранено; новая cross-page policy не вводилась.
- Отдельный неиспользуемый CSV helper: исправлены 0/false/null и стандартное экранирование кавычек, запятых и переводов строк. Он не подключался к UI export; формульные строки сохраняются как исходные данные, без новой политики spreadsheet-санитизации.

Все девять прежних `it.fails` переведены в обычные регрессионные тесты. Парные проверки прежних дефектов обновлены на исправленное поведение; тесты не удалялись ради зелёного результата.

## Независимое ревью

Проведено отдельное read-only ревью. Оно дополнительно выявило custom-default/legacy URL случай и stale response при смене endpoint; эти случаи исправлены и включены в регрессии. Итог и ограничения — в `independent-review.md`.

## Границы этапа

Не унифицировались GET limit=100 и POST limit=1000, POST validation, malformed-body fallback и неоднозначные contract policies. Не добавлялись inline-create, bulk actions, формы/CRUD внутри DataGrid, новые exporters или отдельный versioned module.

Browser visual/mobile/реальная клавиатурная активация остаются NOT RUN: локальный preview в тестовом браузере был заблокирован `ERR_BLOCKED_BY_CLIENT`. DOM-тесты проверяют реальные Vue/Reka components, события, callbacks и состояние, но не заменяют browser paint/scroll/focus QA. Дорогие повторные 1000-row JSDOM mounts не повторялись; скорость 1000 строк в настоящем браузере пока не обещается.

## Как проверить демо

Архив содержит обновлённое автономное mock-демо и исходники. Из распакованного корня:

```sh
python3 -m http.server 5184 --bind 127.0.0.1 --directory resources/audit/datagrid/dist
```

Открыть `http://127.0.0.1:5184/`. Все данные синтетические. Журнал показывает callbacks/API/DOM events, есть 0/100/1000 записей, ошибки/retry, race, фильтры/сортировка, permissions, host-form simulation и безопасная escaping probe. Исходное демо: `resources/audit/datagrid/`.

Актуальный журнал команд и финальные результаты: `STAGE1-VERIFICATION.md`. Исторический аудит до правок сохранён отдельно.
