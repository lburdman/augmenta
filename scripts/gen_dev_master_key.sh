#!/usr/bin/env bash
# Generates a 32-byte base64 encoded master key for local DEV vault encryption
openssl rand -base64 32 | tr -d '\n'
echo ""
