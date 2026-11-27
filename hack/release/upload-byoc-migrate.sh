#!/usr/bin/env bash
# Builds kubectl-byoc-migrate for Linux and macOS and uploads the binaries to S3.
# It reads the AWS credentials from the environment, for example from aws-vault exec.
#
# Usage: BUCKET=<bucket> hack/release/upload-byoc-migrate.sh [version]
#
# The binaries go to s3://<bucket>/kubectl-byoc-migrate/<version>/, where version defaults to the short commit hash,
# and replace the ones in s3://<bucket>/kubectl-byoc-migrate/latest/.
# The bucket must let anyone read kubectl-byoc-migrate/*, so that the install command it prints works without credentials.
set -euo pipefail

: "${BUCKET:?Set BUCKET to the S3 bucket to upload to}"

cd "$(git rev-parse --show-toplevel)"

if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
    echo "The working tree has uncommitted changes. Commit or stash them, so that the binaries match the commit." >&2
    exit 1
fi

VERSION=${1:-$(git rev-parse --short HEAD)}
DEST="s3://${BUCKET}/kubectl-byoc-migrate/${VERSION}"

OUT=$(mktemp -d)
trap 'rm -rf "$OUT"' EXIT

for platform in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
    os=${platform%/*}
    arch=${platform#*/}
    name="kubectl-byoc_migrate_${os}_${arch}"
    echo "Building ${name}"
    CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags '-s -w' -o "${OUT}/${name}" ./cmd/kubectl-byoc-migrate
    gzip -9 "${OUT}/${name}"
done

(cd "$OUT" && shasum -a 256 ./*.gz | sed 's| \./| |' > checksums.txt)

echo "Uploading to ${DEST}/"
aws s3 cp "$OUT" "$DEST" --recursive --only-show-errors
aws s3 ls "${DEST}/"

LATEST="s3://${BUCKET}/kubectl-byoc-migrate/latest"
if [ "$VERSION" != latest ]; then
    echo "Uploading to ${LATEST}/"
    aws s3 sync "$OUT" "$LATEST" --delete --only-show-errors
    aws s3 ls "${LATEST}/"
fi

REGION=$(aws s3api get-bucket-location --bucket "$BUCKET" --query LocationConstraint --output text)
if [ "$REGION" = None ]; then
    REGION=us-east-1
fi
URL="https://${BUCKET}.s3.${REGION}.amazonaws.com/kubectl-byoc-migrate"

print_install() {
    cat <<EOF

Install $1 with:

OS=\$(uname -s | tr A-Z a-z); ARCH=\$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/'); curl -fsSL "${URL}/$1/kubectl-byoc_migrate_\${OS}_\${ARCH}.gz" | gunzip > kubectl-byoc_migrate && chmod +x kubectl-byoc_migrate && sudo mv kubectl-byoc_migrate /usr/local/bin/
EOF
}

print_install "$VERSION"
if [ "$VERSION" != latest ]; then
    print_install latest
fi
