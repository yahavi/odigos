#!/usr/bin/env bash

set -e

# Setup
CHARTDIRS=("helm/odigos" "helm/odigos-central")
FLY_HELM_OCI="oci://odigosdemo.jfrog.io/helmoci"

if [ -z "$TAG" ]; then
	echo "TAG required"
	exit 1
fi

if [[ $(git diff -- ${CHARTDIRS[*]} | wc -c) -ne 0 ]]; then
	echo "Helm chart dirty. Aborting."
	exit 1
fi

# Update chart versions and package them
for chart in "${CHARTDIRS[@]}"
do
	echo "Updating $chart/Chart.yaml with version ${TAG#v}"
	sed -i -E 's/0.0.0/'"${TAG#v}"'/' $chart/Chart.yaml
done
helm package ${CHARTDIRS[*]} -d helm/

# Push each packaged chart to the Fly Helm OCI registry.
# Auth is configured on the runner by jfrog/fly-action.
for chart in helm/odigos-*.tgz; do
	echo "📤 Pushing $chart to $FLY_HELM_OCI"
	helm push "$chart" "$FLY_HELM_OCI"
done

# Roll back chart version changes
git checkout ${CHARTDIRS[*]}
