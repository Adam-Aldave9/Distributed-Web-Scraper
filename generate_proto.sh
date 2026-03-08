#!/usr/bin/env bash
set -euo pipefail

# Generate Go code from proto files into both Supervisor and Worker directories.
# Requires: protoc, protoc-gen-go, protoc-gen-go-grpc

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "Generating proto files for Supervisor..."
protoc --proto_path=proto \
  --go_out=Supervisor --go-grpc_out=Supervisor \
  proto/supervisor.proto proto/worker.proto

echo "Generating proto files for Worker..."
protoc --proto_path=proto \
  --go_out=Worker --go-grpc_out=Worker \
  proto/supervisor.proto proto/worker.proto

echo "Done."
