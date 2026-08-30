# URL Shortener

Микросервисный URL Shortener на Go: API Gateway, Core Service, Stat Service, PostgreSQL, Redis, RabbitMQ, gRPC между сервисами.

Проект строится поэтапно — полная дорожная карта, зафиксированные архитектурные решения и их обоснование находятся в [`plans/plans.md`](plans/plans.md).

## Статус

- [x] Фаза 0 — скелет репозитория
- [x] Фаза 1 — Core Service MVP (REST + Postgres)
- [x] Фаза 2 — Docker + Compose + CI baseline
- [x] Фаза 3 — Redis-кеш
- [x] Фаза 4 — API Gateway + gRPC
- [x] Фаза 5 — RabbitMQ + Stat Service
- [x] Фаза 6 — Сквозное укрепление (тесты, логирование, health-checks)
- [ ] Фаза 7 — Документация и финальная упаковка

## Запуск

```
cp .env.example .env
make up
```

Поднимет Postgres, Redis, RabbitMQ, применит миграции и запустит все три сервиса с hot-reload (правки в `.go`-файлах подхватываются автоматически, без пересборки образа):

- **API Gateway** — единственная публичная точка входа, HTTP на `:8080`:
  - `POST /links` — создать короткую ссылку
  - `GET /{code}` — редирект (публикует событие клика в RabbitMQ неблокирующе)
  - `GET /links/{code}` — информация о ссылке
  - `GET /links/{code}/stats` — статистика переходов
- **Core Service** — внутренний, только gRPC на `:9090` (создание/резолв ссылок, свой Redis-кеш из Фазы 3).
- **Stat Service** — внутренний, только gRPC на `:9091` (агрегированная статистика); отдельно потребляет очередь `link.clicks` из RabbitMQ через пул воркеров, батчево пишущих в Postgres.

Порты Core/Stat Service опубликованы наружу только для локальной отладки (`grpcurl`) — Gateway обращается к ним по internal docker-сети.

RabbitMQ management UI: `http://localhost:15672` (логин/пароль — `RABBITMQ_USER`/`RABBITMQ_PASSWORD` из `.env`). Обратите внимание: встроенный пользователь `guest` у RabbitMQ ограничен только loopback-подключениями, поэтому для меж-контейнерного трафика заведён отдельный пользователь — см. `docker-compose.yml`.

### Наблюдаемость

- **Health-checks**: `GET /health` на Gateway (liveness); Core/Stat Service отдают стандартный gRPC health-протокол (`grpc.health.v1.Health`), проверяется через `grpc-health-probe` внутри контейнера — все три сервиса подключены к `docker-compose` healthcheck.
- **Request-id**: каждый входящий HTTP-запрос к Gateway получает `X-Request-Id` (переиспользуется, если передан вызывающей стороной, иначе генерируется). ID прокидывается через gRPC-метадату в Core/Stat Service и как заголовок AMQP-сообщения в RabbitMQ — во всех логах (`request`/`rpc`) есть поле `request_id`, по которому можно проследить один запрос через все сервисы.
- **gRPC reflection** включён на Core/Stat Service — можно ходить `grpcurl` без локальной копии `.proto`:
  ```
  grpcurl -plaintext localhost:9090 list
  grpcurl -plaintext -d '{"url":"https://example.com"}' localhost:9090 link.v1.LinkService/CreateLink
  ```

Регенерация gRPC-кода после правки `.proto`-файлов (требует локально установленный `protoc` + `protoc-gen-go`/`protoc-gen-go-grpc`):
```
make proto
```
Сгенерированный код (`api/proto/linkpb/`, `api/proto/statspb/`) закоммичен в репозиторий — для обычной сборки/CI/Docker `protoc` не нужен.

Прод-эквивалентная сборка (без hot-reload, тот же образ, что уйдёт в прод):
```
make prod-up
```

## Разработка

См. `Makefile` для команд сборки/тестов/линтера (заполняется по мере прохождения фаз).
