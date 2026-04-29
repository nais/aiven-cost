#!/usr/bin/env bash
#MISE description="Run aiven-cost"
set -euo pipefail

go run ./cmd/aiven-cost/main.go
