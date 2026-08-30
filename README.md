# URL Shortener

Микросервисный URL Shortener на Go: API Gateway, Core Service, Stat Service, PostgreSQL, Redis, RabbitMQ, gRPC между сервисами.

Проект строится поэтапно — полная дорожная карта, зафиксированные архитектурные решения и их обоснование находятся в [`plans/plans.md`](plans/plans.md).

## Статус

- [x] Фаза 0 — скелет репозитория
- [x] Фаза 1 — Core Service MVP (REST + Postgres)
- [x] Фаза 2 — Docker + Compose + CI baseline
- [x] Фаза 3 — Redis-кеш
- [x] Фаза 4 — API Gateway + gRPC
- [ ] Фаза 5 — RabbitMQ + Stat Service
- [ ] Фаза 6 — Сквозное укрепление (тесты, логирование, health-checks)
- [ ] Фаза 7 — Документация и финальная упаковка

## Запуск

```
cp .env.example .env
make up
```

Поднимет Postgres, Redis, применит миграции и запустит оба сервиса с hot-reload (правки в `.go`-файлах подхватываются автоматически, без пересборки образа):

- **API Gateway** — единственная публичная точка входа, HTTP на `:8080` (`POST /links`, `GET /{code}` редирект, `GET /links/{code}` инфо).
- **Core Service** — внутренний, только gRPC на `:9090` (не предназначен для прямых обращений клиентов; порт опубликован наружу только для локальной отладки через `grpcurl`).

Gateway обращается к Core Service по gRPC на каждый запрос; Core Service сам кеширует ответы в Redis (см. Фазу 3) — Gateway не дублирует кеш.

Регенерация gRPC-кода после правки `api/proto/link.proto` (требует локально установленный `protoc` + `protoc-gen-go`/`protoc-gen-go-grpc`, см. `make proto`):
```
make proto
```
Сгенерированный код (`api/proto/linkpb/`) закоммичен в репозиторий — для обычной сборки/CI/Docker `protoc` не нужен.

Прод-эквивалентная сборка (без hot-reload, тот же образ, что уйдёт в прод):
```
make prod-up
```

## Разработка

См. `Makefile` для команд сборки/тестов/линтера (заполняется по мере прохождения фаз).