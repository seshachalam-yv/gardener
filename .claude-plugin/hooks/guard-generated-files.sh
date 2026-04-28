#!/usr/bin/env bash
set -euo pipefail

# Guard generated files from manual editing.
# These files are produced by make generate and must not be edited by hand.

FILE_PATH="${CLAUDE_FILE_PATH:-$1}"

case "$FILE_PATH" in
  *zz_generated.deepcopy.go|*zz_generated.conversion.go|*zz_generated.defaults.go|*zz_generated.model_name.go)
    echo "BLOCKED: This is a generated file (zz_generated). Do not edit manually. Run 'make generate' to regenerate."
    exit 1
    ;;
  *generated.pb.go|*generated.proto|*generated.protomessage.pb.go)
    echo "BLOCKED: This is a protobuf generated file. Do not edit manually. Run 'make generate WHAT=\"protobuf\"' to regenerate."
    exit 1
    ;;
  *openapi_generated.go)
    echo "BLOCKED: This is an OpenAPI generated file. Do not edit manually. Run 'make generate' to regenerate."
    exit 1
    ;;
  *third_party/mock/*)
    echo "BLOCKED: This is a pre-generated third-party mock file. Do not edit manually. Mocks are regenerated via go generate."
    exit 1
    ;;
  *docs/api-reference/*.md)
    echo "BLOCKED: API reference docs are auto-generated from Go types. Do not edit manually. Run 'make generate' to regenerate."
    exit 1
    ;;
  *docs/cli-reference/*.md)
    echo "BLOCKED: CLI reference docs are auto-generated. Do not edit manually. Run 'make generate' to regenerate."
    exit 1
    ;;
esac

exit 0
