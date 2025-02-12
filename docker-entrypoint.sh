#!/bin/sh
case "$(uname -m)" in
    x86_64)  exec ./midsommar-amd64 ;;
    aarch64) exec ./midsommar-arm64 ;;
    *)       echo "Unsupported architecture: $(uname -m)" && exit 1 ;;
esac
