#!/bin/sh
# e2e-shard.sh DIR SHARD TOTAL prints a -ginkgo.focus-file regular expression with the test files of DIR that shard
# SHARD (1..TOTAL) runs, so CI can run a suite over TOTAL jobs. Every file is in exactly one shard.
#
# The weight of a file is its run time in seconds from DIR/timings.tsv (the output of scripts/e2e-timings.sh for the
# reports of one PostgreSQL version of a CI run). A file that isn't in it yet is estimated: 5 seconds plus 60 for each
# cluster it starts of its own (WithOwn...(): starting a cluster takes most of the time). The files are divided
# greedily: the heaviest first, each to the lightest shard so far.
set -e
dir=$1 shard=$2 total=$3
if [ -z "$total" ] || [ "$shard" -lt 1 ] || [ "$shard" -gt "$total" ]; then
	echo "usage: $0 DIR SHARD TOTAL (1 <= SHARD <= TOTAL)" >&2
	exit 2
fi
timings=$dir/timings.tsv
for f in "$dir"/*_test.go; do
	name=$(basename "$f")
	[ "$name" = suite_test.go ] && continue
	seconds=
	[ -f "$timings" ] && seconds=$(awk -v name="$name" '$2 == name { print $1; exit }' "$timings")
	[ -n "$seconds" ] || seconds=$(( 5 + 60 * $(grep -cE 'WithOwn[A-Za-z]*\(' "$f" || true) ))
	echo "$seconds $name"
done | sort -k1,1nr -k2,2 | awk -v shard="$shard" -v total="$total" '
	{
		best = 1
		for (s = 2; s <= total; s++) if (load[s] < load[best]) best = s
		load[best] += $1
		if (best == shard) files = files (files == "" ? "" : "|") $2
	}
	END {
		gsub(/\./, "\\.", files)
		print "/(" files ")$"
	}'
