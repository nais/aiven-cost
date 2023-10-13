ARG GO_VERSION=1.21
FROM golang:${GO_VERSION}-alpine as builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -a -o bin/aiven-cost ./cmd/aiven-cost

FROM gcr.io/distroless/base
WORKDIR /app
COPY --from=builder /src/bin/aiven-cost .
CMD ["/app/aiven-cost"]