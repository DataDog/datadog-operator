#!/usr/bin/env bash

# Exit on error, undefined variable, and pipe failure
set -o errexit
set -o nounset
set -o pipefail

# Determine the script directory
SCRIPTS_DIR="$(dirname "$0")"
# Source common installation variables and functions
source "$SCRIPTS_DIR/os-env.sh"

# Define tool paths
JQ="$ROOT/bin/$PLATFORM/jq"
YQ="$ROOT/bin/$PLATFORM/yq"

# Ensure jq and yq are available
if [[ ! -x "$JQ" ]]; then
    echo "Error: jq is not executable or found at $JQ"
    exit 1
fi

if [[ ! -x "$YQ" ]]; then
    echo "Error: yq is not executable or found at $YQ"
    exit 1
fi

# Get Go version from go.work and parse it
GOVERSION=$(go work edit --json | $JQ -r .Go)
IFS='.' read -r major minor revision <<< "$GOVERSION"

echo "----------------------------------------"
echo "Golang version from go.work: $GOVERSION"
echo "- major: $major"
echo "- minor: $minor"
echo "- revision: $revision"
echo "----------------------------------------"

# Define new minor version
new_minor_version=$major.$minor

# Update go.work
go_work_file="$ROOT/go.work"
if [[ -f $go_work_file ]]; then
    echo "Processing $go_work_file..."
    sed -i -E "s/^go [^ ].*/go $GOVERSION/gm" "$go_work_file"
else
    echo "Warning: $go_work_file not found, skipping."
fi

# Update devcontainer.json
dev_container_file="$ROOT/.devcontainer/devcontainer.json"
if [[ -f $dev_container_file ]]; then
    echo "Processing $dev_container_file..."
    sed -i -E "s|(mcr\.microsoft\.com/devcontainers/go:)[^\"]+|\1dev-$new_minor_version|" "$dev_container_file"
else
    echo "Warning: $dev_container_file not found, skipping."
fi

# Update Dockerfiles
dockerfile_files="$ROOT/Dockerfile $ROOT/check-operator.Dockerfile"
for file in $dockerfile_files; do
    if [[ -f $file ]]; then
        echo "Processing $file..."
        sed -i -E "s|(FROM golang:)[^ ]+|\1$GOVERSION|" "$file"
    else
        echo "Warning: $file not found, skipping."
    fi
done

# Update .gitlab-ci.yml
gitlab_file="$ROOT/.gitlab-ci.yml"
if [[ -f $gitlab_file ]]; then
    echo "Processing $gitlab_file..."
    sed -i -E "s|(image: registry\.ddbuild\.io/images/mirror/library/golang:)[^ ]+|\1$GOVERSION|" "$gitlab_file"
else
    echo "Warning: $gitlab_file not found, skipping."
fi

# Update GitHub Actions workflows
actions_directory="$ROOT/.github/workflows"
if [[ -d $actions_directory ]]; then
    for file in "$actions_directory"/*; do
        if [[ -f $file ]]; then
            go_version=$($YQ .env.GO_VERSION "$file")
            if [[ $go_version != "null" ]]; then
                echo "Processing $file"
                $YQ -i ".env.GO_VERSION = \"$GOVERSION\"" $file
            fi
        fi
    done
else
    echo "Warning: $actions_directory not found, skipping."
fi

# Run go work sync
echo "Running go work sync..."
go work sync

# Update go.mod files
go_mod_files="$ROOT/go.mod $ROOT/test/e2e/go.mod $ROOT/api/go.mod"
for file in $go_mod_files; do
    if [[ -f $file ]]; then
        echo "Processing $file..."
        go mod edit -go $new_minor_version $file
        go mod edit -toolchain go$GOVERSION $file
        parent_dir=$(dirname "$file")
        cd $parent_dir; cd $ROOT
    else
        echo "Warning: $file not found, skipping."
    fi
done