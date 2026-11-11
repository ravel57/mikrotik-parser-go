#!/usr/bin/env bash
set -euo pipefail

path="$(pwd)"
cd ../../WebstormProjects/mikrotik_parser

npm install
npm run build

dest="$path/web/dist"

if [ -d "./dist" ]; then
    rm -rf "$dest"
    mkdir -p "$dest"
    cp -R ./dist/. "$dest"
    cd "$path"
else
    echo "Copying error: ./dist not found" >&2
    cd "$path"
    exit 1
fi
