#!/bin/bash

# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

# Reads secrets from files in the directory given as the only argument, using
# the embedded secret-generic-connector (SGC) "k8s.file" backend.
#
# The operator sends the legacy secret backend payload ({"version","secrets"}),
# so this script injects the SGC backend "type" and "config" into it.

set -euo pipefail
shopt -s extglob

SGC_BIN=/usr/local/bin/secret-generic-connector
# Matches the historical limit of the "helpers read-secret" command.
MAX_SECRET_FILE_SIZE=8192

# The operator splits -secretBackendArgs on single spaces, which yields empty
# arguments on repeated/trailing spaces. The previous version of this script
# expanded $@ unquoted, so keep that word-splitting for compatibility.
# shellcheck disable=SC2068
set -- $@

if [[ $# -ne 1 ]]; then
    echo "Error: expected exactly 1 argument (secrets directory), got $#" >&2
    exit 1
fi

dir=${1//\\/\\\\}
dir=${dir//\"/\\\"}

payload=$(cat)
payload=${payload%%+([[:space:]])}
if [[ $payload != \{*\} ]]; then
    echo "Error: failed to unmarshal json input" >&2
    exit 1
fi

if [[ ! $payload =~ \"version\"[[:space:]]*:[[:space:]]*\"1(\.|\") ]]; then
    echo "Error: incompatible protocol version" >&2
    exit 1
fi

if [[ ! $payload =~ \"secrets\"[[:space:]]*:[[:space:]]*\[[[:space:]]*\" ]]; then
    echo "Error: no secrets listed in input" >&2
    exit 1
fi

body=${payload%\}}
body=${body%%+([[:space:]])}

printf '%s,"type":"k8s.file","config":{"secrets_path":"%s","max_file_read_size":%d}}' \
    "$body" "$dir" "$MAX_SECRET_FILE_SIZE" | exec "$SGC_BIN"
