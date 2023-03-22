#!/usr/bin/env bash

uname_os() {
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$os" in
    msys*) os="windows" ;;
    mingw*) os="windows" ;;
    cygwin*) os="windows" ;;
    win*) os="windows" ;;
  esac
  echo "$os"
}

OS=$(uname_os)
ARCH=$(uname -m)
PLATFORM="$OS-$ARCH"
ROOT=$(git rev-parse --show-toplevel)
SED="sed -i"
if [ "$OS" == "darwin" ]; then
    SED="sed -i .bak"
fi
