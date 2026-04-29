#!/usr/bin/env bash
#MISE description="Build aiven-cost binary"
set -euo pipefail

go build -o bin/aiven-cost ./cmd/aiven-cost/main.go