.PHONY: local

all: tidy fmt build-aiven-cost build-currency check

build-aiven-cost:
	go build -o bin/aiven-cost ./cmd/aiven-cost/main.go

build-currency:
	go build -o bin/currency ./cmd/currency/main.go

aiven-cost:
	go run ./cmd/aiven-cost/main.go

currency:
	go run ./cmd/currency/main.go

check:
	go run honnef.co/go/tools/cmd/staticcheck ./...
	go run golang.org/x/vuln/cmd/govulncheck ./...
	go run golang.org/x/tools/cmd/deadcode@latest -test ./...

fmt:
	go run mvdan.cc/gofumpt -w ./

tidy:
	go mod tidy
