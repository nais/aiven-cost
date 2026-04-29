#!/usr/bin/env bash
#MISE description="Format all go code"
#MISE wait_for=["gofix"]
set -euo pipefail

go tool mvdan.cc/gofumpt -w ./
