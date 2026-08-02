# URL Shortener

Микросервисный URL Shortener на Go: API Gateway, Core Service, Stat Service, PostgreSQL, Redis, RabbitMQ, gRPC между сервисами.

Проект строится поэтапно — полная дорожная карта, зафиксированные архитектурные решения и их обоснование находятся в [`plans/plans.md`](plans/plans.md).

## Статус

- [x] Фаза 0 — скелет репозитория
- [x] Фаза 1 — Core Service MVP (REST + Postgres)
- [x] Фаза 2 — Docker + Compose + CI baseline
- [ ] Фаза 3 — Redis-кеш
- [ ] Фаза 4 — API Gateway + gRPC
- [ ] Фаза 5 — RabbitMQ + Stat Service
- [ ] Фаза 6 — Сквозное укрепление (тесты, логирование, health-checks)
- [ ] Фаза 7 — Документация и финальная упаковка

## Запуск

```
cp .env.example .env
make up
```

Поднимет Postgres, применит миграции и запустит Core Service с hot-reload (правки в `.go`-файлах подхватываются автоматически, без пересборки образа). Сервис слушает `:8080`.

Прод-эквивалентная сборка (без hot-reload, тот же образ, что уйдёт в прод):
```
make prod-up
```

## Разработка

См. `Makefile` для команд сборки/тестов/линтера (заполняется по мере прохождения фаз).