package k8sconsts

// Diagnose command
const (
	LogsDir    = "Logs"
	CRDsDir    = "CRDs"
	ProfileDir = "Profile"
	MetricsDir = "Metrics"
)

const (
	CliImageName        = "odigos-cli"
	CliOffsetsImageName = "odigos-cli-offsets"
)

// Helm constants
const (
	DefaultHelmChart          = "odigos/odigos"
	OdigosHelmRepoName        = "odigos"
	DefaultCentralHelmChart   = "odigos/odigos-central"
	OdigosCentralHelmRepoName = "odigos-central"
	DefaultCentralReleaseName = "odigos-central"

	// OdigosHelmOCIRepo is the OCI registry where Odigos Helm charts are published.
	// Charts are pulled directly via oci:// references (no `helm repo add` needed).
	OdigosHelmOCIRepo = "oci://odigosdemo.jfrog.io/helmoci"
	// OdigosHelmOCIChart / OdigosCentralHelmOCIChart are the full OCI references for the charts.
	OdigosHelmOCIChart        = OdigosHelmOCIRepo + "/odigos"
	OdigosCentralHelmOCIChart = OdigosHelmOCIRepo + "/odigos-central"
)
