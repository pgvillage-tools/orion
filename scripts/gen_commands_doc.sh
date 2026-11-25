#!/usr/bin/env bash

set -e
# shellcheck disable=SC1091
source "$(dirname "$0")"/readlinkdashf.sh

BASEDIR=$(readlinkdashf "$(dirname "$0")"/..)

OUTDIR="$BASEDIR"/docs/reference/commands
# start empty, so that the pages of removed commands go too
rm -f "$OUTDIR"/orion-*.md
mkdir -p "$OUTDIR"
go run "$BASEDIR"/scripts/gen_commands_doc.go "$OUTDIR"/
