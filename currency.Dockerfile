ARG GO_VERSION=1.26

FROM golang:${GO_VERSION} AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -a -o bin/currency ./cmd/currency

FROM gcr.io/distroless/base:nonroot
WORKDIR /app
COPY --from=builder /src/bin/currency .
CMD ["/app/currency"]
