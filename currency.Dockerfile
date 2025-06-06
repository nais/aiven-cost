FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -a -o bin/currency ./cmd/currency

FROM gcr.io/distroless/base
WORKDIR /app
COPY --from=builder /src/bin/currency currency
CMD ["/app/currency"]
