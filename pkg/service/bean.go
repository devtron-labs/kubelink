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

package service

import (
	"github.com/devtron-labs/common-lib/utils/k8s/commonBean"
	"github.com/devtron-labs/common-lib/workerPool"
	"github.com/devtron-labs/kubelink/bean"
	"github.com/devtron-labs/kubelink/pkg/asyncProvider"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

type HelmReleaseStatusConfig struct {
	InstallAppVersionHistoryId int
	Message                    string
	IsReleaseInstalled         bool
	ErrorInInstallation        bool
}

type ParentChildGvkMapping struct {
	Group        string         `json:"group"`
	Version      string         `json:"version"`
	Kind         string         `json:"kind"`
	ChildObjects []ChildObjects `json:"childObjects"`
}

func (r ParentChildGvkMapping) GetParentGvk() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   r.Group,
		Version: r.Version,
		Kind:    r.Kind,
	}
}

type ChildObjects struct {
	Group    string             `json:"group"`
	Version  string             `json:"version"`
	Resource string             `json:"resource"`
	Scope    meta.RESTScopeName `json:"scope"`
}

func (r ChildObjects) GetGvrAndScopeForChildObject() *commonBean.GvrAndScope {
	return &commonBean.GvrAndScope{
		Gvr: schema.GroupVersionResource{
			Group:    r.Group,
			Version:  r.Version,
			Resource: r.Resource,
		},
		Scope: r.Scope,
	}
}

type BuildNodesConfig struct {
	DesiredOrLiveManifests []*bean.DesiredOrLiveManifest
	batchWorker            *workerPool.WorkerPool[*BuildNodeResponse]
	BuildNodesRequest
}

type GetNodeFromManifestRequest struct {
	DesiredOrLiveManifest *bean.DesiredOrLiveManifest
	BuildNodesRequest
}

type BuildNodesRequest struct {
	RestConfig        *rest.Config
	ReleaseNamespace  string
	ParentResourceRef *bean.ResourceRef
}

func NewBuildNodesRequest(buildNodesConfig *BuildNodesRequest) *BuildNodesConfig {
	if buildNodesConfig == nil {
		return &BuildNodesConfig{}
	}
	req := &BuildNodesConfig{
		BuildNodesRequest: *buildNodesConfig,
	}
	return req
}

func NewGetNodesFromManifest(buildNodesConfig *BuildNodesRequest) *GetNodeFromManifestRequest {
	if buildNodesConfig == nil {
		return &GetNodeFromManifestRequest{}
	}
	req := &GetNodeFromManifestRequest{
		BuildNodesRequest: *buildNodesConfig,
	}
	return req
}

func (req *BuildNodesConfig) WithDesiredOrLiveManifests(desiredOrLiveManifests ...*bean.DesiredOrLiveManifest) *BuildNodesConfig {
	if len(desiredOrLiveManifests) == 0 {
		return req
	}
	req.DesiredOrLiveManifests = append(req.DesiredOrLiveManifests, desiredOrLiveManifests...)
	return req
}

func (req *BuildNodesConfig) WithBatchWorker(buildNodesBatchSize int, logger *zap.SugaredLogger) *BuildNodesConfig {
	if buildNodesBatchSize <= 0 {
		buildNodesBatchSize = 1
	}
	// for parallel processing of Nodes
	req.batchWorker = asyncProvider.NewBatchWorker[*BuildNodeResponse](buildNodesBatchSize, logger)
	return req
}

func (req *GetNodeFromManifestRequest) WithDesiredOrLiveManifest(desiredOrLiveManifest *bean.DesiredOrLiveManifest) *GetNodeFromManifestRequest {
	if desiredOrLiveManifest == nil {
		return req
	}
	req.DesiredOrLiveManifest = desiredOrLiveManifest
	return req
}

func NewBuildNodesConfig(restConfig *rest.Config) *BuildNodesRequest {
	return &BuildNodesRequest{
		RestConfig: restConfig,
	}
}

func (req *BuildNodesRequest) WithReleaseNamespace(releaseNamespace string) *BuildNodesRequest {
	if releaseNamespace == "" {
		return req
	}
	req.ReleaseNamespace = releaseNamespace
	return req
}

func (req *BuildNodesRequest) WithParentResourceRef(parentResourceRef *bean.ResourceRef) *BuildNodesRequest {
	if parentResourceRef == nil {
		return req
	}
	req.ParentResourceRef = parentResourceRef
	return req
}

type BuildNodeResponse struct {
	Nodes             []*bean.ResourceNode
	HealthStatusArray []*bean.HealthStatus
}

type GetNodeFromManifestResponse struct {
	Node                           *bean.ResourceNode
	HealthStatus                   *bean.HealthStatus
	ResourceRef                    *bean.ResourceRef
	DesiredOrLiveChildrenManifests []*bean.DesiredOrLiveManifest
}

func NewGetNodesFromManifestResponse() *GetNodeFromManifestResponse {
	return &GetNodeFromManifestResponse{}
}

func (resp *GetNodeFromManifestResponse) WithNode(node *bean.ResourceNode) *GetNodeFromManifestResponse {
	if node == nil {
		return resp
	}
	resp.Node = node
	return resp
}

func (resp *GetNodeFromManifestResponse) WithHealthStatus(healthStatus *bean.HealthStatus) *GetNodeFromManifestResponse {
	if healthStatus == nil {
		return resp
	}
	resp.HealthStatus = healthStatus
	return resp
}

func (resp *GetNodeFromManifestResponse) WithParentResourceRef(resourceRef *bean.ResourceRef) *GetNodeFromManifestResponse {
	if resourceRef == nil {
		return resp
	}
	resp.ResourceRef = resourceRef
	return resp
}

func (resp *GetNodeFromManifestResponse) WithDesiredOrLiveManifests(desiredOrLiveManifests ...*bean.DesiredOrLiveManifest) *GetNodeFromManifestResponse {
	if len(desiredOrLiveManifests) == 0 {
		return resp
	}
	resp.DesiredOrLiveChildrenManifests = append(resp.DesiredOrLiveChildrenManifests, desiredOrLiveManifests...)
	return resp
}

func NewBuildNodeResponse() *BuildNodeResponse {
	return &BuildNodeResponse{}
}

func (resp *BuildNodeResponse) WithNodes(nodes []*bean.ResourceNode) *BuildNodeResponse {
	if len(nodes) == 0 {
		return resp
	}
	resp.Nodes = append(resp.Nodes, nodes...)
	return resp
}

func (resp *BuildNodeResponse) WithHealthStatusArray(healthStatusArray []*bean.HealthStatus) *BuildNodeResponse {
	if len(healthStatusArray) == 0 {
		return resp
	}
	resp.HealthStatusArray = append(resp.HealthStatusArray, healthStatusArray...)
	return resp
}
