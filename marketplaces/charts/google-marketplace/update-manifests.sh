#!/bin/bash
# update-manifests.sh — Regenerate chart/datadog-mp/templates/manifests.yaml
# from the CRD YAML files at a given git ref (tag or branch).
#
# Usage:
#   ./update-manifests.sh <git-ref>
#
# Example:
#   ./update-manifests.sh v1.26.0

set -euo pipefail

if [ $# -ne 1 ]; then
    echo "Usage: $0 <git-ref>"
    echo "Example: $0 v1.26.0"
    exit 1
fi

GIT_REF="$1"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
OUTPUT="$SCRIPT_DIR/chart/datadog-mp/templates/manifests.yaml"
CRD_PATH="config/crd/bases/v1"

# Verify the ref exists
if ! git -C "$REPO_ROOT" rev-parse --verify "$GIT_REF" > /dev/null 2>&1; then
    echo "Error: git ref '$GIT_REF' not found"
    exit 1
fi

# List CRD YAML files at the given ref, sorted alphabetically
CRD_FILES=$(
    git -C "$REPO_ROOT" ls-tree --name-only "$GIT_REF" "$CRD_PATH/" \
    | grep '\.yaml$' \
    | sort
)

if [ -z "$CRD_FILES" ]; then
    echo "Error: no YAML files found at '$CRD_PATH' in ref '$GIT_REF'"
    exit 1
fi

FILE_COUNT=$(echo "$CRD_FILES" | wc -l | tr -d ' ')
echo "Generating $OUTPUT from ref '$GIT_REF' using $FILE_COUNT CRD files:"

# Write each CRD file (already starts with ---) to the output, truncating first
> "$OUTPUT"
while IFS= read -r crd_file; do
    echo "  $crd_file"
    git -C "$REPO_ROOT" show "$GIT_REF:$crd_file" >> "$OUTPUT"
done <<< "$CRD_FILES"

echo "Done. $(wc -l < "$OUTPUT") lines written to $OUTPUT"
