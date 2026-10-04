#!/usr/bin/env bash
# Simple load generator for the demo system. Sends N requests to the api service
# at a given concurrency and prints a histogram of HTTP status codes.
#
# Usage: load/loadgen.sh [URL] [REQUESTS] [CONCURRENCY]
#   URL          default http://localhost:8080/
#   REQUESTS     default 200
#   CONCURRENCY  default 5
set -euo pipefail

URL="${1:-http://localhost:8080/}"
REQUESTS="${2:-200}"
CONCURRENCY="${3:-5}"

echo "Sending ${REQUESTS} requests to ${URL} (concurrency ${CONCURRENCY})..."
seq "${REQUESTS}" \
  | xargs -P "${CONCURRENCY}" -I{} curl -s -o /dev/null -w "%{http_code}\n" "${URL}" \
  | sort | uniq -c | sed 's/^/  /'
echo "done."
