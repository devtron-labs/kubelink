package bean

import "k8s.io/apimachinery/pkg/runtime/schema"

const (
	FluxKustomizationGroup   = "kustomize.toolkit.fluxcd.io"
	FluxAppKustomizationKind = "Kustomization"
	FluxKustomizationVersion = "v1"
	AllNamespaces            = ""
	FluxHelmReleaseGroup     = "helm.toolkit.fluxcd.io"
	FluxAppHelmreleaseKind   = "HelmRelease"
	FluxHelmReleaseVersion   = "v2"

	//DevtronCDNamespae            = "devtroncd"
	//ArgoLabelForManagedResources = "app.kubernetes.io/instance"
)

type FluxApplicationListDto struct {
	Name         string `json:"appName"`
	ClusterId    int    `json:"clusterId"`
	ClusterName  string `json:"clusterName"`
	Namespace    string `json:"namespace"`
	HealthStatus string `json:"appStatus"`
	SyncStatus   string `json:"syncStatus"`
}

var GvkForKustomizationFluxApp = schema.GroupVersionKind{
	Group:   FluxKustomizationGroup,
	Kind:    FluxAppKustomizationKind,
	Version: FluxKustomizationVersion,
}

var GvkForhelmreleaseFluxApp = schema.GroupVersionKind{
	Group:   FluxHelmReleaseGroup,
	Kind:    FluxAppHelmreleaseKind,
	Version: FluxHelmReleaseVersion,
}
