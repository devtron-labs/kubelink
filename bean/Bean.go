/*
 * Copyright (c) 2024. Devtron Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package bean

import (
	"github.com/devtron-labs/common-lib/utils/k8s/commonBean"
	remoteConnectionBean "github.com/devtron-labs/common-lib/utils/remoteConnection/bean"
	client "github.com/devtron-labs/kubelink/grpc"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"time"
)

type HelmAppValues struct {
	// Values are default config for this chart.
	DefaultValues map[string]interface{} `json:"defaultValues"`
	// ValuesOverride is the set of extra Values added to the chart.
	// These values override the default values inside of the chart.
	OverrideValues map[string]interface{} `json:"overrideValues"`
	// Merged values are merged of default and override
	MergedValues map[string]interface{} `json:"mergedValues"`
}

type AppDetail struct {
	ApplicationStatus    *commonBean.HealthStatusCode `json:"applicationStatus"`
	ReleaseStatus        *ReleaseStatus               `json:"releaseStatus"`
	LastDeployed         time.Time                    `json:"lastDeployed"`
	ChartMetadata        *ChartMetadata               `json:"chartMetadata"`
	ResourceTreeResponse *ResourceTreeResponse        `json:"resourceTreeResponse"`
	EnvironmentDetails   *client.EnvironmentDetails   `json:"environmentDetails"`
	ReleaseExists        bool                         `json:"releaseExists"`
}

type ReleaseStatus struct {
	Status      HelmReleaseStatus `json:"status"`
	Message     string            `json:"message"`
	Description string            `json:"description"`
}

type HelmReleaseStatus = string

// Describe the status of a release
// NOTE: Make sure to update cmd/helm/status.go when adding or modifying any of these statuses.
const (
	// StatusUnknown indicates that a release is in an uncertain state.
	StatusUnknown HelmReleaseStatus = "unknown"
	// StatusDeployed indicates that the release has been pushed to Kubernetes.
	StatusDeployed HelmReleaseStatus = "deployed"
	// StatusUninstalled indicates that a release has been uninstalled from Kubernetes.
	StatusUninstalled HelmReleaseStatus = "uninstalled"
	// StatusSuperseded indicates that this release object is outdated and a newer one exists.
	StatusSuperseded HelmReleaseStatus = "superseded"
	// StatusFailed indicates that the release was not successfully deployed.
	StatusFailed HelmReleaseStatus = "failed"
	// StatusUninstalling indicates that a uninstall operation is underway.
	StatusUninstalling HelmReleaseStatus = "uninstalling"
	// StatusPendingInstall indicates that an install operation is underway.
	StatusPendingInstall HelmReleaseStatus = "pending-install"
	// StatusPendingUpgrade indicates that an upgrade operation is underway.
	StatusPendingUpgrade HelmReleaseStatus = "pending-upgrade"
	// StatusPendingRollback indicates that an rollback operation is underway.
	StatusPendingRollback HelmReleaseStatus = "pending-rollback"
)

type ChartMetadata struct {
	// The name of the chart
	ChartName string `json:"chartName"`
	// version string of the chart
	ChartVersion string `json:"chartVersion"`
	// The URL to a relevant project page, git repo, or contact person
	Home string `json:"home,omitempty"`
	// Source is the URL to the source code of this chart
	Sources []string `json:"sources,omitempty"`
	// A one-sentence description of the chart
	Description string `json:"description,omitempty"`
	// Contains the rendered templates/NOTES.txt if available
	Notes string `json:"notes,omitempty"`
}

type ResourceTreeResponse struct {
	*ApplicationTree
	PodMetadata []*commonBean.PodMetadata `json:"podMetadata"`
}

// ApplicationTree holds nodes which belongs to the application
type ApplicationTree struct {
	Nodes []*commonBean.ResourceNode `json:"nodes,omitempty" protobuf:"bytes,1,rep,name=nodes"`
}

type HelmReleaseDetailRequest struct {
	ClusterHost        string `json:"clusterHost"  validate:"required"`
	ClusterBaererToken string `json:"clusterBaererToken"  validate:"required"`
	Namespace          string `json:"namespace"  validate:"required"`
	ReleaseName        string `json:"releaseName"  validate:"required"`
}

type ClusterConfig struct {
	ApiServerUrl string `protobuf:"bytes,1,opt,name=apiServerUrl,proto3" json:"apiServerUrl,omitempty"`
	Token        string `protobuf:"bytes,2,opt,name=token,proto3" json:"token,omitempty"`
	ClusterId    int32  `protobuf:"varint,3,opt,name=clusterId,proto3" json:"clusterId,omitempty"`
	ClusterName  string `protobuf:"bytes,4,opt,name=clusterName,proto3" json:"clusterName,omitempty"`
}

type KubernetesResourcePatchRequest struct {
	Name      string
	Namespace string
	Gvk       *schema.GroupVersionKind
	Patch     string
	PatchType string
}

type HelmAppDeploymentDetail struct {
	DeployedAt    time.Time      `json:"deployedAt"`
	ChartMetadata *ChartMetadata `json:"chartMetadata"`
	// Manifest is the string representation of the rendered template.
	Manifest     string   `json:"manifest"`
	DockerImages []string `json:"dockerImages"`
	// Version is an int which represents the revision of the release.
	Version int `json:"version,omitempty"`
}

type DesiredOrLiveManifest struct {
	Manifest                   *unstructured.Unstructured `json:"manifest"`
	IsLiveManifestFetchError   bool                       `json:"isLiveManifestFetchError"`
	LiveManifestFetchErrorCode int32                      `json:"liveManifestFetchErrorCode"`
}

type ClusterInfo struct {
	ClusterId              int                                              `json:"clusterId"`
	ClusterName            string                                           `json:"clusterName"`
	BearerToken            string                                           `json:"bearerToken"`
	ServerUrl              string                                           `json:"serverUrl"`
	InsecureSkipTLSVerify  bool                                             `json:"insecureSkipTLSVerify"`
	KeyData                string                                           `json:"-"`
	CertData               string                                           `json:"-"`
	CAData                 string                                           `json:"-"`
	RemoteConnectionConfig *remoteConnectionBean.RemoteConnectionConfigBean `json:"remoteConnectionConfig"`
}
