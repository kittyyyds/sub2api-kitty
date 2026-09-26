#!/bin/sh

set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
config="$repo_root/deploy/kitty-site/root-api-compat.conf"

usage_block=$(awk '
  /^location = \/usage \{/ { in_block = 1 }
  in_block { print }
  in_block && /^}/ { exit }
' "$config")

printf '%s\n' "$usage_block" | grep -Fq 'if ($http_accept ~* "text/html") {'
printf '%s\n' "$usage_block" | grep -Fq 'rewrite ^ /index.html last;'
printf '%s\n' "$usage_block" | grep -Fq 'rewrite ^ /v1/usage last;'

html_line=$(printf '%s\n' "$usage_block" | grep -nF 'rewrite ^ /index.html last;' | cut -d: -f1)
api_line=$(printf '%s\n' "$usage_block" | grep -nF 'rewrite ^ /v1/usage last;' | cut -d: -f1)
if [ "$html_line" -ge "$api_line" ]; then
  echo "HTML navigation rewrite must run before the bare API rewrite" >&2
  exit 1
fi

echo "root API compatibility keeps browser /usage navigations on the SPA"
