#!/usr/bin/env bash
#MISE description="Build csv-backfill binary"
set -euo pipefail

go build -o bin/csv-backfill ./cmd/csv-backfill/main.go