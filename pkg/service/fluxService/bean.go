package fluxService

import (
	"github.com/devtron-labs/kubelink/bean"
	client "github.com/devtron-labs/kubelink/grpc"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	FluxKustomizationGroup     = "kustomize.toolkit.fluxcd.io"
	FluxAppKustomizationKind   = "Kustomization"
	FluxKustomizationVersionV1 = "v1"
	AllNamespaces              = ""
	FluxHelmReleaseGroup       = "helm.toolkit.fluxcd.io"
	FluxAppHelmreleaseKind     = "HelmRelease"
	FluxHelmReleaseVersionV2   = "v2"
	FluxLabel                  = "labels"
	KustomizeNameLabel         = "kustomize.toolkit.fluxcd.io/name"
	KustomizeNamespaceLabel    = "kustomize.toolkit.fluxcd.io/namespace"
)

var GvkForKustomizationFluxApp = schema.GroupVersionKind{
	Group:   FluxKustomizationGroup,
	Kind:    FluxAppKustomizationKind,
	Version: FluxKustomizationVersionV1,
}

var GvkForHelmreleaseFluxApp = schema.GroupVersionKind{
	Group:   FluxHelmReleaseGroup,
	Kind:    FluxAppHelmreleaseKind,
	Version: FluxHelmReleaseVersionV2,
}

type FluxApplicationDto struct {
	Name                  string             `json:"appName"`
	HealthStatus          string             `json:"appStatus"`
	SyncStatus            string             `json:"syncStatus"`
	EnvironmentDetails    *EnvironmentDetail `json:"environmentDetail"`
	FluxAppDeploymentType FluxAppType        `json:"fluxAppDeploymentType"`
}
type EnvironmentDetail struct {
	ClusterId   int    `json:"clusterId"`
	ClusterName string `json:"clusterName"`
	Namespace   string `json:"namespace"`
}

type FluxKsResourceDetail struct {
	Name      string
	Namespace string
	Group     string
	Version   string
	Kind      string
}
type ObjMetadata struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Group     string `json:"group"`
	Kind      string `json:"kind"`
}

type FluxAppType string

const (
	HelmReleaseFluxAppType FluxAppType = "HelmRelease"
	AppNameKey                         = "Name"
	StatusKey                          = "Status"
	ReadyKey                           = "Ready"
	ColumnNameKey                      = "name"
	NamespaceKey                       = "namespace"
	MetaDataField                      = "metadata"
	ObjectField                        = "object"
)

type FluxAppDetailRequest struct {
	Config      *client.ClusterConfig
	Name        string `json: "name"`
	Namespace   string `json: "namespace"`
	IsKustomize bool   `json: "isKustomize"`
}
type FluxKsAppDetail struct {
	*FluxApplicationDto
	AppStatusDto *FluxAppStatusDetail
	TreeResponse []*bean.ResourceTreeResponse
}
type FluxAppStatusDetail struct {
	Status  string
	Reason  string
	Message string
}
type FluxHr struct {
	Name      string
	Namespace string
}

const (
	STATUS    = "status"
	INVENTORY = "inventory"
	ENTRIES   = "entries"
	ID        = "id"
	VERSION   = "v"
)

type ObjectMetadataCompact struct {
	Id      string `json:"id"`
	Version string `json:"version"`
}

const (
	FieldSeparator  = "_"
	ColonTranscoded = "__"
)
