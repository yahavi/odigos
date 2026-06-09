#!/usr/bin/env bash

set -e

# Setup
TMPDIR="$(mktemp -d)"

prefix () {
	echo "${@:1}"
	echo "${@:2}"
	for i in "${@:2}"; do
		echo "Renaming $i to $1$i"
		mv "$i" "$1$i"
	done
}

if [ -z "$TAG" ]; then
	echo "TAG required"
	exit 1
fi

if [ -z "$GITHUB_REPOSITORY" ]; then
	echo "GITHUB_REPOSITORY required"
	exit 1
fi

echo "------------------------------------------------------------"
echo "📦 Publishing pre-packaged Helm charts for $TAG"
echo "------------------------------------------------------------"

# Verify that the packaged charts exist
if [ ! -f "helm/odigos-${TAG#v}.tgz" ] || [ ! -f "helm/odigos-central-${TAG#v}.tgz" ]; then
	echo "❌ Pre-packaged charts not found in helm/ directory"
	echo "Expected: helm/odigos-${TAG#v}.tgz and helm/odigos-central-${TAG#v}.tgz"
	ls -la helm/
	exit 1
fi

echo "✅ Found pre-packaged charts:"
ls -lah helm/odigos-*.tgz

# Push each packaged chart to the Fly Helm OCI registry.
# Auth is configured on the runner by jfrog/fly-action.
for chart in helm/odigos-*.tgz; do
	echo "📤 Pushing $chart to oci://odigosdemo.jfrog.io/helmoci"
	helm push "$chart" oci://odigosdemo.jfrog.io/helmoci
done

echo "✅ Helm charts published successfully"
