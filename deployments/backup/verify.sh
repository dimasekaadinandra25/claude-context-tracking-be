#!/bin/sh
set -o pipefail
file=${1:-$(ls -t /backups/claude_usage-*.sql.gz 2>/dev/null | head -n 1)}
db=claude_usage_verify
export PGOPTIONS="-c client_min_messages=warning"
if [ ! -f "$file" ]; then
  echo "backup verify failed: no dump found" >&2
  exit 1
fi
dropdb --if-exists "$db" && createdb "$db" || exit 1
if gunzip -c "$file" | psql -q -1 -v ON_ERROR_STOP=1 -d "$db" >/dev/null \
  && counts=$(psql -tA -v ON_ERROR_STOP=1 -d "$db" -c "SELECT (SELECT count(*) FROM core.users) || ' users, ' || (SELECT count(*) FROM usage.sessions) || ' sessions'"); then
  echo "backup verified: $file restores cleanly ($counts)"
  status=0
else
  echo "backup verify failed: $file did not restore" >&2
  status=1
fi
dropdb --if-exists "$db"
exit $status
