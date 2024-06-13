package FluxApplicationService

import (
	client "github.com/devtron-labs/kubelink/grpc"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	FluxKustomizationGroup   = "kustomize.toolkit.fluxcd.io"
	FluxAppKustomizationKind = "Kustomization"
	FluxKustomizationVersion = "v1"
	AllNamespaces            = ""
	FluxHelmReleaseGroup     = "helm.toolkit.fluxcd.io"
	FluxAppHelmreleaseKind   = "HelmRelease"
	FluxHelmReleaseVersion   = "v2"
	FluxLabel                = "labels"
	KustomizeNameLabel       = "kustomize.toolkit.fluxcd.io/name"
	KustomizeNamespaceLabel  = "kustomize.toolkit.fluxcd.io/namespace"
)

var GvkForKustomizationFluxApp = schema.GroupVersionKind{
	Group:   FluxKustomizationGroup,
	Kind:    FluxAppKustomizationKind,
	Version: FluxKustomizationVersion,
}

var GvkForHelmreleaseFluxApp = schema.GroupVersionKind{
	Group:   FluxHelmReleaseGroup,
	Kind:    FluxAppHelmreleaseKind,
	Version: FluxHelmReleaseVersion,
}

type FluxApplicationDto struct {
	Name               string             `json:"appName"`
	HealthStatus       string             `json:"appStatus"`
	SyncStatus         string             `json:"syncStatus"`
	EnvironmentDetails *EnvironmentDetail `json:"environmentDetail"`
	AppType            string             `json:"appType"`
}
type EnvironmentDetail struct {
	ClusterId   int    `json:"clusterId"`
	ClusterName string `json:"clusterName"`
	Namespace   string `json:"namespace"`
}
type FluxKustomization struct {
	AppKsDetailDto   *FluxKsAppDetail                    `json:"appDetailDto"`
	Resources        *client.ExternalResourceTreeRequest `json:"fluxResource"`
	Kustomizations   []*FluxKustomization                `json:"kustomizations"`
	FluxHelmReleases []*client.AppDetailRequest          `json:"fluxHelmReleases"`
	ParentKsApp      string                              `json:"parentKsApp"`
	Manifest         unstructured.Unstructured           `json:"manifest"`
}

type FluxKsAppDetail struct {
	Name      string
	Namespace string
	GroupKind schema.GroupKind
}

type FluxAppType string

const (
	HelmReleaseFluxAppType FluxAppType = "HelmRelease"
	NameKey                            = "Name"
	StatusKey                          = "Status"
	ReadyKey                           = "Ready"
	ColumnNameKey                      = "name"
)
