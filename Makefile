#!make
include .env
export $(shell sed 's/=.*//' .env)

MIGRATE := docker run --rm -v $(shell pwd)/migrations:/migrations --network host --user $(id -u):$(id -g) migrate/migrate -path=/migrations/ -database 'postgres://127.0.0.1:5433/expenses?sslmode=disable&user=admin&password=password'
MIGRATE_CREATE := docker run --rm -v $(shell pwd)/migrations:/migrations --network host --user $(shell id -u):$(shell id -g) migrate/migrate create --seq -ext sql -dir /migrations/

.PHONY: run
run: ## run the application locally using air
	air

.PHONY: build
build:
	CGO_ENABLED=0 go build -ldflags='-w -s -extldflags "-static"' -o ./bin/go-telexpenses main.go

.PHONY: container
container: ## create docker container
	docker build -t p4nospet/go-telexpenses .

.PHONY: db-start
db-start: ## start the database
	@mkdir -p testdata/postgres
	docker run --rm --name expensesdb -d -v $(shell pwd)/testdata:/testdata -p 5433:5432 \
		-v $(shell pwd)/testdata/postgres:/var/lib/postgresql/data \
		-e POSTGRES_PASSWORD=password -e POSTGRES_DB=expenses -e POSTGRES_USER=admin -d postgres:15.4

.PHONY: db-stop
db-stop: ## stop the database
	docker stop expensesdb

.PHONY: db-login
db-login: ## login to the database
	docker exec -it expensesdb psql -U admin -d expenses

.PHONY: migrate
migrate: ## revert database to the last migration step
	@echo "Reverting database to the last migration step..."
	@$(MIGRATE) up

.PHONY: migrate-down
migrate-down: ## revert database to the last migration step
	@echo "Reverting database to the last migration step..."
	@$(MIGRATE) down 1

.PHONY: migrate-new
migrate-new: ## create a new database migration
	@read -p "Enter the name of the new migration: " name; \
	$(MIGRATE_CREATE) $${name}

