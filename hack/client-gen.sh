#!/usr/bin/env bash

source "$HOME"/go/pkg/mod/k8s.io/code-generator@v0.31.0/kube_codegen.sh

SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..

kube::codegen::gen_client \
  --output-dir "$SCRIPT_ROOT"/pkg/client \
  --output-pkg github.com/mNi-Cloud/operator/pkg/client \
  --boilerplate "$SCRIPT_ROOT"/hack/boilerplate.go.txt \
  "$SCRIPT_ROOT"
