.PHONY: local

all: tidy fmt build-aiven-cost build-currency-data check

build-aiven-cost:
	GOOS=linux GOARCH=amd64 go build -o bin/aiven-cost ./cmd/aiven-cost/main.go

build-currency-data:
	GOOS=linux GOARCH=amd64 go build -o bin/currency-data ./cmd/currency-data/main.go

aiven-cost:
	go run ./cmd/aiven-cost/main.go

currency-data:
	go run ./cmd/currency-data/main.go

check:
	go run honnef.co/go/tools/cmd/staticcheck ./...
	go run golang.org/x/vuln/cmd/govulncheck ./...

fmt:
	go run mvdan.cc/gofumpt -w ./

tidy:
	go mod tidy