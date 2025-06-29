DC := docker compose
MIGRATE := docker run --rm -v $(PWD)/migrations:/migrations --network merch-network migrate/migrate:v4.18.1
DB_URL := postgres://postgres:postgres@postgres:5432/merch?sslmode=disable
K6 := ./k6

.PHONY: up down migrate migrate-down test logs clean help

help:
	@echo "Доступные команды:"
	@echo "  make up         - Запустить все сервисы Docker Compose"
	@echo "  make down       - Остановить и удалить контейнеры"
	@echo "  make migrate    - Применить все миграции"
	@echo "  make migrate-down - Откатить последнюю миграцию"
	@echo "  make test       - Запустить нагрузочный тест k6"
	@echo "  make logs       - Показать логи всех сервисов"
	@echo "  make clean      - Сбросить окружение и данные"

up:
	$(DC) up -d

down:
	$(DC) down

migrate:
	$(MIGRATE) -path /migrations -database $(DB_URL) up

migrate-down:
	$(MIGRATE) -path /migrations -database $(DB_URL) down 1

test:
	$(K6) run load_test.js

logs:
	$(DC) logs -f

clean:
	$(DC) exec postgres psql -U postgres -d merch -c "TRUNCATE TABLE users, transactions RESTART IDENTITY;"
	$(DC) exec redis redis-cli FLUSHDB
	$(DC) exec kafka kafka-topics --bootstrap-server kafka:9092 --delete --topic users
	$(DC) exec kafka kafka-topics --bootstrap-server kafka:9092 --delete --topic transactions
	$(DC) exec postgres psql -U postgres -d merch -c "UPDATE users SET balance = 1000;"