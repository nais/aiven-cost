FROM golang:1.21-alpine as builder

RUN apk add --no-cache git

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY . /workspace

# Build
RUN CGO_ENABLED=0 go build -a -o aiven-cost ./cmd/aiven-cost

FROM alpine
WORKDIR /

COPY --from=builder /workspace/aiven-cost .
USER 65532:65532

CMD ["/aiven-cost"]