package helm

import (
	"context"
	"fmt"
	"strings"

	"github.com/odigos-io/odigos/api/k8sconsts"
	"github.com/odigos-io/odigos/k8sutils/pkg/installationmethod"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/registry"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

var (
	HelmReleaseName  string
	HelmNamespace    string
	HelmChart        string
	HelmValuesFile   string
	HelmSetArgs      []string
	HelmChartVersion string
)

// injected at build time
var OdigosChartVersion string

var (
	HelmResetThenReuseValues = true // default: true (sensible for upgrades)
)

func PrepareChartAndValues(settings *cli.EnvSettings, chartName string) (*chart.Chart, map[string]interface{}, error) {
	return prepareChartAndValues(settings, chartName, k8sconsts.DefaultHelmChart)
}

func PrepareCentralChartAndValues(settings *cli.EnvSettings, chartName string) (*chart.Chart, map[string]interface{}, error) {
	return prepareChartAndValues(settings, chartName, k8sconsts.DefaultCentralHelmChart)
}

// prepareChartAndValues is the common implementation for both OSS and Central flows.
// - chartName controls which embedded chart archive to load (e.g. "odigos" / "odigos-central")
// - embeddedGateChart controls when we attempt embedded chart first (i.e. when HelmChart == embeddedGateChart and no --chart-version override)
func prepareChartAndValues(settings *cli.EnvSettings, chartName string, embeddedGateChart string) (*chart.Chart, map[string]interface{}, error) {
	version := ""
	if HelmChartVersion != "" {
		version = strings.TrimPrefix(HelmChartVersion, "v")
	} else if OdigosChartVersion != "" {
		version = strings.TrimPrefix(OdigosChartVersion, "v")
	}
	// Use embedded chart if available (when using the default chart and no override)
	if HelmChart == embeddedGateChart && HelmChartVersion == "" {
		ch, err := LoadEmbeddedChart(version, chartName)
		if err == nil {
			fmt.Printf("📦 Using embedded chart %s (chart version: %s)\n", ch.Metadata.Name, ch.Metadata.Version)

			// merge values like normal (so --set and --values flags work)
			valOpts := &values.Options{
				ValueFiles: []string{},
				Values:     HelmSetArgs,
			}
			if HelmValuesFile != "" {
				valOpts.ValueFiles = append(valOpts.ValueFiles, HelmValuesFile)
			}
			vals, err := valOpts.MergeValues(getter.All(settings))
			if err != nil {
				return nil, nil, err
			}

			// fallback image.tag to AppVersion if not set
			// During the release of the helm chart, we're setting the appVersion to the same as the image.tag [package-charts.sh]
			if ch.Metadata.AppVersion != "" {
				if _, ok := vals["image"]; !ok {
					vals["image"] = map[string]interface{}{}
				}
				if imgVals, ok := vals["image"].(map[string]interface{}); ok {
					if _, hasTag := imgVals["tag"]; !hasTag || imgVals["tag"] == "" {
						imgVals["tag"] = ch.Metadata.AppVersion
						fmt.Printf("Using the Chart appVersion %s as image.tag\n", ch.Metadata.AppVersion)
					}
				}
			}

			return ch, vals, nil
		}
		// if no embedded chart found, continue with repo fallback
	}

	// otherwise: use remote/local chart.
	// The default Odigos charts are published to an OCI registry. Map the
	// historical "odigos/<chart>" aliases to their oci:// references so Helm
	// can pull them directly (LocateChart handles oci:// natively, no repo add).
	chartRef := resolveOdigosOCIChart(HelmChart)

	// Locate the chart. OCI references require a registry client, which is
	// wired through an install action's ChartPathOptions (the registry client
	// field is unexported and only set via the action configuration).
	locator := action.NewInstall(&action.Configuration{})
	locator.Version = version
	if registry.IsOCI(chartRef) {
		regClient, err := registry.NewClient()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create Helm registry client: %w", err)
		}
		locator.SetRegistryClient(regClient)
	}
	chartPath, err := locator.ChartPathOptions.LocateChart(chartRef, settings)
	if err != nil {
		return nil, nil, err
	}
	ch, err := loader.Load(chartPath)
	if err != nil {
		return nil, nil, err
	}

	valOpts := &values.Options{
		ValueFiles: []string{},
		Values:     HelmSetArgs,
	}
	if HelmValuesFile != "" {
		valOpts.ValueFiles = append(valOpts.ValueFiles, HelmValuesFile)
	}
	vals, err := valOpts.MergeValues(getter.All(settings))
	if err != nil {
		return nil, nil, err
	}

	if ch.Metadata.AppVersion != "" {
		if _, ok := vals["image"]; !ok {
			vals["image"] = map[string]interface{}{}
		}
		if imgVals, ok := vals["image"].(map[string]interface{}); ok {
			if _, hasTag := imgVals["tag"]; !hasTag || imgVals["tag"] == "" {
				imgVals["tag"] = ch.Metadata.AppVersion
				fmt.Printf("Using appVersion %s as image.tag\n", ch.Metadata.AppVersion)
			}
		}
	}

	return ch, vals, nil
}

// resolveOdigosOCIChart maps the historical "odigos/<chart>" aliases to their
// OCI references so Helm can pull them directly from the Fly registry.
// Any other value (local path, explicit oci:// ref, or third-party repo/name)
// is returned unchanged.
func resolveOdigosOCIChart(chartRef string) string {
	switch chartRef {
	case k8sconsts.DefaultHelmChart:
		return k8sconsts.OdigosHelmOCIChart
	case k8sconsts.DefaultCentralHelmChart:
		return k8sconsts.OdigosCentralHelmOCIChart
	default:
		return chartRef
	}
}

// IsLegacyInstallation checks whether Odigos was installed using the old non-Helm method.
func IsLegacyInstallation(ctx context.Context, client corev1.CoreV1Interface, namespace string) (bool, error) {
	cm, err := client.ConfigMaps(namespace).Get(ctx, k8sconsts.OdigosDeploymentConfigMapName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// ConfigMap doesn’t exist — not an old install
			return false, nil
		}
		return false, fmt.Errorf("failed to check installation method: %w", err)
	}

	method := cm.Data[k8sconsts.OdigosDeploymentConfigMapInstallationMethodKey]
	if method == string(installationmethod.K8sInstallationMethodOdigosCli) {
		return true, nil
	}

	return false, nil
}

// IsLegacyCentralInstallation checks whether Odigos Central was installed using the old non-Helm method
// (aka the legacy `odigos pro-dep central install` flow).
func IsLegacyCentralInstallation(ctx context.Context, client corev1.CoreV1Interface, namespace string) (bool, error) {
	cm, err := client.ConfigMaps(namespace).Get(ctx, k8sconsts.OdigosCentralDeploymentConfigMapName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check Central installation method: %w", err)
	}

	method := cm.Data[k8sconsts.OdigosCentralDeploymentConfigMapInstallationMethodKey]
	return method == string(installationmethod.K8sInstallationMethodOdigosCli), nil
}

func isHelmOwnedByRelease(labels map[string]string, annotations map[string]string, releaseName string, namespace string) bool {
	if labels == nil || annotations == nil {
		return false
	}
	return labels["app.kubernetes.io/managed-by"] == "Helm" &&
		annotations["meta.helm.sh/release-name"] == releaseName &&
		annotations["meta.helm.sh/release-namespace"] == namespace
}

func legacyCentralLeftoverErr(kind string, name string, namespace string, releaseName string) error {
	return fmt.Errorf(
		"found pre-existing %s %q in namespace %q that is not owned by Helm release %q; "+
			"this usually happens after running the legacy Central install/uninstall flow (`odigos pro-dep central ...`). "+
			"To proceed, uninstall the legacy Central installation (recommended), or delete the resource / namespace and retry. "+
			"Example: odigos pro-dep central uninstall -n %s --yes  (or kubectl delete namespace %s)",
		kind, name, namespace, releaseName, namespace, namespace,
	)
}

// ValidateCentralHelmInstallPreconditions fails fast when the legacy Central install/uninstall flow left behind
// resources that would block a Helm-based install/upgrade (Helm cannot adopt existing objects without ownership metadata).
// It validates:
// - The legacy install marker ConfigMap (`odigos-central-deployment`)
// - The on-prem token Secret (`odigos-central`) which legacy creates without labels
// - Any other legacy Central "system objects" in the namespace (Deployments/Services/SAs/Roles/RoleBindings/Secrets/ConfigMaps/PVCs/HPAs)
func ValidateCentralHelmInstallPreconditions(
	ctx context.Context,
	coreClient corev1.CoreV1Interface,
	dyn dynamic.Interface,
	namespace string,
	releaseName string,
) error {
	// 1) Explicitly check the Central deployment marker ConfigMap (common collision).
	cm, err := coreClient.ConfigMaps(namespace).Get(ctx, k8sconsts.OdigosCentralDeploymentConfigMapName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to check existing Central deployment ConfigMap: %w", err)
	}
	if err == nil && !isHelmOwnedByRelease(cm.GetLabels(), cm.GetAnnotations(), releaseName, namespace) {
		return legacyCentralLeftoverErr("ConfigMap", cm.GetName(), namespace, releaseName)
	}

	// 2) Explicitly check the Central token secret, because legacy creates it without system labels (so label-scans may miss it).
	sec, err := coreClient.Secrets(namespace).Get(ctx, k8sconsts.OdigosCentralSecretName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to check existing Central token Secret: %w", err)
	}
	if err == nil && !isHelmOwnedByRelease(sec.GetLabels(), sec.GetAnnotations(), releaseName, namespace) {
		return legacyCentralLeftoverErr("Secret", sec.GetName(), namespace, releaseName)
	}

	// 3) Scan all legacy Central system objects (created via ApplyResource) by label.
	// Note: chart objects are labeled with odigos.io/system-object, but legacy Central objects get the *central* label as well
	// because the legacy installer sets SystemObjectLabelKey = OdigosSystemLabelCentralKey.
	labelSelector := fmt.Sprintf("%s=%s", k8sconsts.OdigosSystemLabelCentralKey, k8sconsts.OdigosSystemLabelValue)

	type check struct {
		gvr        schema.GroupVersionResource
		kind       string
		namespaced bool
	}
	checks := []check{
		{gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}, kind: "ConfigMap", namespaced: true},
		{gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}, kind: "Secret", namespaced: true},
		{gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "serviceaccounts"}, kind: "ServiceAccount", namespaced: true},
		{gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}, kind: "Service", namespaced: true},
		{gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumeclaims"}, kind: "PersistentVolumeClaim", namespaced: true},
		{gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, kind: "Deployment", namespaced: true},
		{gvr: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"}, kind: "Role", namespaced: true},
		{gvr: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"}, kind: "RoleBinding", namespaced: true},
		{gvr: schema.GroupVersionResource{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"}, kind: "HorizontalPodAutoscaler", namespaced: true},
		{gvr: schema.GroupVersionResource{Group: "autoscaling", Version: "v2beta2", Resource: "horizontalpodautoscalers"}, kind: "HorizontalPodAutoscaler", namespaced: true},
		{gvr: schema.GroupVersionResource{Group: "autoscaling", Version: "v2beta1", Resource: "horizontalpodautoscalers"}, kind: "HorizontalPodAutoscaler", namespaced: true},
	}

	for _, c := range checks {
		if c.namespaced {
			ul, e := dyn.Resource(c.gvr).Namespace(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
			if e == nil {
				for _, item := range ul.Items {
					if isHelmOwnedByRelease(item.GetLabels(), item.GetAnnotations(), releaseName, namespace) {
						continue
					}
					return legacyCentralLeftoverErr(c.kind, item.GetName(), namespace, releaseName)
				}
				continue
			}
			if !apierrors.IsNotFound(e) {
				return fmt.Errorf("failed to scan existing Central %s resources: %w", c.kind, e)
			}
		} else {
			ul, e := dyn.Resource(c.gvr).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
			if e == nil {
				for _, item := range ul.Items {
					// cluster-scoped resources don't have release-namespace annotation set to a namespace? Helm still sets it.
					if isHelmOwnedByRelease(item.GetLabels(), item.GetAnnotations(), releaseName, namespace) {
						continue
					}
					return legacyCentralLeftoverErr(c.kind, item.GetName(), namespace, releaseName)
				}
				continue
			}
			if !apierrors.IsNotFound(e) {
				return fmt.Errorf("failed to scan existing Central %s resources: %w", c.kind, e)
			}
		}
	}

	return nil
}
