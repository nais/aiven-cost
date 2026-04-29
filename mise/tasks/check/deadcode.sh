#!/usr/bin/env bash
#MISE description="Run deadcode detector"
set -euo pipefail

go tool golang.org/x/tools/cmd/deadcode -test -tags integration_test ./...