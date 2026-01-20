#!/bin/bash

# Plugin path - USE FORWARD SLASHES!
PLUGIN_PATH="/c/Users/son.nguyenhoai/go/bin/bin/"

# Script is at root level (mezon-protobuf/)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Proto files are in ./proto/
PROTO_PATH="${SCRIPT_DIR}/proto"

# Output to ./go/
OUTPUT_PATH="${SCRIPT_DIR}/go"

# Create output directory
mkdir -p "${OUTPUT_PATH}"

echo "===================================="
echo "Script Dir: ${SCRIPT_DIR}"
echo "Proto Path: ${PROTO_PATH}"
echo "Output Path: ${OUTPUT_PATH}"
echo "===================================="

# Check if proto files exist
if [ ! -f "${PROTO_PATH}/rtapi/realtime.proto" ]; then
    echo "Error: rtapi/realtime.proto not found!"
    exit 1
fi

if [ ! -f "${PROTO_PATH}/api/api.proto" ]; then
    echo "Error: api/api.proto not found!"
    exit 1
fi

# Run protoc
protoc \
  --plugin=protoc-gen-go="${PLUGIN_PATH}protoc-gen-go.exe" \
  --plugin=protoc-gen-go-grpc="${PLUGIN_PATH}protoc-gen-go-grpc.exe" \
  --plugin=protoc-gen-openapiv2="${PLUGIN_PATH}protoc-gen-openapiv2.exe" \
  --plugin=protoc-gen-grpc-gateway="${PLUGIN_PATH}protoc-gen-grpc-gateway.exe" \
  --proto_path="${PROTO_PATH}" \
  --go_out="${OUTPUT_PATH}" \
  --go_opt=paths=source_relative \
  --go-grpc_out="${OUTPUT_PATH}" \
  --go-grpc_opt=paths=source_relative \
  --grpc-gateway_out="${OUTPUT_PATH}" \
  --grpc-gateway_opt=paths=source_relative \
  --openapiv2_out="${OUTPUT_PATH}" \
  "${PROTO_PATH}/rtapi/realtime.proto" \
  "${PROTO_PATH}/api/api.proto"

if [ $? -eq 0 ]; then
    echo ""
    echo "✓ Proto generation completed successfully!"
    echo "✓ Check: ${OUTPUT_PATH}"
else
    echo ""
    echo "✗ Failed!"
    exit 1
fi