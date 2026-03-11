#!/usr/bin/env bash

set -euo pipefail

max_attempts="${ASSET_WAIT_MAX_ATTEMPTS:-60}"
sleep_seconds="${ASSET_WAIT_SLEEP_SECONDS:-10}"

shopt -s nullglob

formula_files=("$@")
if [ "${#formula_files[@]}" -eq 0 ]; then
  formula_files=(dist/*.rb)
fi

if [ "${#formula_files[@]}" -eq 0 ]; then
  echo "No Homebrew formula files found in dist/."
  exit 1
fi

declare -A seen_urls
urls=()

for formula in "${formula_files[@]}"; do
  if [ ! -f "$formula" ]; then
    echo "Formula file not found: $formula"
    exit 1
  fi

  echo "Reading release URLs from $formula"
  while IFS= read -r line; do
    if [[ "$line" =~ ^[[:space:]]*url[[:space:]]+\"([^\"]+)\" ]]; then
      url="${BASH_REMATCH[1]}"
      if [ -z "${seen_urls[$url]+x}" ]; then
        seen_urls["$url"]=1
        urls+=("$url")
      fi
    fi
  done < "$formula"
done

if [ "${#urls[@]}" -eq 0 ]; then
  echo "No release URLs found in formula files."
  exit 1
fi

for url in "${urls[@]}"; do
  echo "Waiting for asset URL: $url"

  attempt=1
  while [ "$attempt" -le "$max_attempts" ]; do
    if curl --fail --silent --show-error --location --range 0-0 --output /dev/null "$url"; then
      echo "Asset ready: $url"
      break
    fi

    if [ "$attempt" -eq "$max_attempts" ]; then
      echo "Timed out waiting for asset URL after ${max_attempts} attempts: $url"
      exit 1
    fi

    echo "Asset not ready yet (attempt ${attempt}/${max_attempts}), retrying in ${sleep_seconds}s"
    sleep "$sleep_seconds"
    attempt=$((attempt + 1))
  done
done

echo "All Homebrew release asset URLs are downloadable."
