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
package CommonHelperService

import (
	"github.com/devtron-labs/common-lib/utils/k8s/commonBean"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
