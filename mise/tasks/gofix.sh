#!/usr/bin/env bash
#MISE description="Run the Go fix tool"
set -euo pipefail

go fix ./...
