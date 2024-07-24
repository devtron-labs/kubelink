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

type BuildNodesRequest struct {
	DesiredOrLiveManifests []*bean.DesiredOrLiveManifest
	batchWorker            *workerPool.WorkerPool[*BuildNodeResponse]
	BuildNodesConfig
}

type GetNodeFromManifest struct {
	DesiredOrLiveManifest *bean.DesiredOrLiveManifest
	BuildNodesConfig
}

type BuildNodesConfig struct {
	RestConfig        *rest.Config
	ReleaseNamespace  string
	ParentResourceRef *bean.ResourceRef
}

func NewBuildNodesRequest(buildNodesConfig *BuildNodesConfig) *BuildNodesRequest {
	if buildNodesConfig == nil {
		return &BuildNodesRequest{}
	}
	req := &BuildNodesRequest{
		BuildNodesConfig: *buildNodesConfig,
	}
	return req
}

func NewGetNodesFromManifest(buildNodesConfig *BuildNodesConfig) *GetNodeFromManifest {
	if buildNodesConfig == nil {
		return &GetNodeFromManifest{}
	}
	req := &GetNodeFromManifest{
		BuildNodesConfig: *buildNodesConfig,
	}
	return req
}

func (req *BuildNodesRequest) WithDesiredOrLiveManifests(desiredOrLiveManifests ...*bean.DesiredOrLiveManifest) *BuildNodesRequest {
	if len(desiredOrLiveManifests) == 0 {
		return req
	}
	req.DesiredOrLiveManifests = append(req.DesiredOrLiveManifests, desiredOrLiveManifests...)
	return req
}

func (req *BuildNodesRequest) WithBatchWorker(buildNodesBatchSize int, logger *zap.SugaredLogger) *BuildNodesRequest {
	if buildNodesBatchSize <= 0 {
		buildNodesBatchSize = 1
	}
	// for parallel processing of nodes
	req.batchWorker = asyncProvider.NewBatchWorker[*BuildNodeResponse](buildNodesBatchSize, logger)
	return req
}

func (req *GetNodeFromManifest) WithDesiredOrLiveManifest(desiredOrLiveManifest *bean.DesiredOrLiveManifest) *GetNodeFromManifest {
	if desiredOrLiveManifest == nil {
		return req
	}
	req.DesiredOrLiveManifest = desiredOrLiveManifest
	return req
}

func NewBuildNodesConfig(restConfig *rest.Config) *BuildNodesConfig {
	return &BuildNodesConfig{
		RestConfig: restConfig,
	}
}

func (req *BuildNodesConfig) WithReleaseNamespace(releaseNamespace string) *BuildNodesConfig {
	if releaseNamespace == "" {
		return req
	}
	req.ReleaseNamespace = releaseNamespace
	return req
}

func (req *BuildNodesConfig) WithParentResourceRef(parentResourceRef *bean.ResourceRef) *BuildNodesConfig {
	if parentResourceRef == nil {
		return req
	}
	req.ParentResourceRef = parentResourceRef
	return req
}

type BuildNodeResponse struct {
	nodes             []*bean.ResourceNode
	healthStatusArray []*bean.HealthStatus
}

type GetNodeFromManifestResponse struct {
	buildChildNodesRequests []*BuildNodesRequest
	node                    *bean.ResourceNode
	healthStatus            *bean.HealthStatus
}

func NewGetNodesFromManifestResponse() *GetNodeFromManifestResponse {
	return &GetNodeFromManifestResponse{}
}

func NewBuildNodeResponse() *BuildNodeResponse {
	return &BuildNodeResponse{}
}

func (resp *BuildNodeResponse) WithNodes(nodes []*bean.ResourceNode) *BuildNodeResponse {
	if len(nodes) == 0 {
		return resp
	}
	resp.nodes = append(resp.nodes, nodes...)
	return resp
}

func (resp *BuildNodeResponse) WithHealthStatusArray(healthStatusArray []*bean.HealthStatus) *BuildNodeResponse {
	if len(healthStatusArray) == 0 {
		return resp
	}
	resp.healthStatusArray = append(resp.healthStatusArray, healthStatusArray...)
	return resp
}
