#!/usr/bin/env bash

Version="unknown"
if [ -z "$(git status --porcelain)" ]; then
  tag=$(git tag --points-at HEAD)
  if [ -z "$tag" ]; then
    # no tag, use commit as version
    Version=$(git rev-list -1 HEAD)
  else
    # has tag, use tag as version
    Version="$tag"
  fi
else
  # has changes not committed
  Version="dirty"
fi

echo -n "$Version"
