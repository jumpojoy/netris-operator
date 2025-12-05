#!/usr/bin/env bash

set -e

get_latest_tag () {
  local latest_tag='v0.0.0'
  for ref in $(git for-each-ref --sort=-creatordate --format '%(refname)' refs/tags); do
    tag="${ref#refs/tags/}"
    if echo "${tag}" | grep -Eq '^v?([0-9]+\.[0-9]+\.[0-9]+)(-([0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*))?(\+[0-9A-Za-z-]+)?$'; then
      latest_tag="${tag}"
      break
    fi
  done
  echo "${latest_tag}"
}

hash=$(git rev-parse --short HEAD)
latest_tag=$(get_latest_tag)
version="${latest_tag#v}"

if ! git describe --tags --exact-match "${hash}" &> /dev/null; then
  IFS='.' read -r MAJOR MINOR PATCH <<< "$version"
  PATCH=$((PATCH + 1))
  version="$MAJOR.$MINOR.$PATCH-$hash"
fi

echo "${version}"
