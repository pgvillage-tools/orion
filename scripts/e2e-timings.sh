#!/bin/sh
# e2e-timings.sh REPORT.json...: the run time of each test file (in seconds, the slowest first) from Ginkgo JSON
# reports (make E2E_REPORT_DIR=..., or the e2e-report-* artifacts of CI), summed over all reports (e.g. the shards of a
# run). The run time of a spec includes the BeforeAll that starts its cluster. To weigh the shards in e2e-shard.sh.
set -e
if [ $# -eq 0 ]; then
	echo "usage: $0 REPORT.json..." >&2
	exit 2
fi
jq -rs '
	[.[][].SpecReports[]
		| select(.LeafNodeType == "It" and .State != "skipped" and .State != "pending")
		| {file: (.LeafNodeLocation.FileName | split("/") | last), seconds: (.RunTime / 1e9)}]
	| group_by(.file)
	| map({file: .[0].file, seconds: (map(.seconds) | add)})
	| sort_by(-.seconds)[]
	| "\(.seconds | floor)\t\(.file)"
' "$@"
