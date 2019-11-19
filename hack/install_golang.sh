#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

mkdir -p /usr/local
curl -Lo go1.12.9.linux-amd64.tar.gz https://dl.google.com/go/go1.12.9.linux-amd64.tar.gz && tar -C /usr/local -xzf go1.12.9.linux-amd64.tar.gz
