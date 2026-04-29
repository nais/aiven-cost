#!/usr/bin/env bash
#MISE description="Build currency binary"
set -euo pipefail

go build -o bin/currency ./cmd/currency/main.go