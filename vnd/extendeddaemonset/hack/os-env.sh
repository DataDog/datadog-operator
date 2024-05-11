#!/usr/bin/env bash

uname_os() {
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$os" in
    msys_nt) os="windows" ;;
  esac
  echo "$os"
}

OS=$(uname_os)
SED="sed -i"
if [ "$OS" == "darwin" ]; then
    SED="sed -i .bak"
fi
