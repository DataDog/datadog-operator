#!/usr/bin/env bash

uname_os() {
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$os" in
    msys_nt) os="windows" ;;
  esac
  echo "$os"
}

OS=$(uname_os)
SED_OPTIONS="-i"
if [ "$OS" == "darwin" ]; then
    SED_OPTIONS="-i ''"
fi
