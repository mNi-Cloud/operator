#!/usr/bin/env bash

components=$(find . -type f -name kustomization.yaml | sed 's|/kustomization.yaml||g')
echo {} > index.yaml

for component in $components; do
  echo "Building component: $component"
  kustomize build "$component" --enable-helm > "$component.yaml"
  yq -i "$(echo "$component" | awk -F'/' '{print "."$2" = [\""$3"\"] + ."$2}')" index.yaml
done