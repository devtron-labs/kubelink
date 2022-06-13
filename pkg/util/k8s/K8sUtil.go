/*
 * Copyright (c) 2020 Devtron Labs
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package k8sUtils

import (
	client "github.com/devtron-labs/kubelink/grpc"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

// constants starts
var podsGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}, Scope: meta.RESTScopeNameNamespace}
var replicaSetGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}, Scope: meta.RESTScopeNameNamespace}
var jobGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}, Scope: meta.RESTScopeNameNamespace}
var endpointsGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "endpoints"}, Scope: meta.RESTScopeNameNamespace}
var endpointSliceGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "discovery.k8s.io", Version: "v1beta1", Resource: "endpointslices"}, Scope: meta.RESTScopeNameNamespace}
var pvGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumes"}, Scope: meta.RESTScopeNameRoot}
var pvcGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumeclaims"}, Scope: meta.RESTScopeNameNamespace}

var gvkVsChildGvrAndScope = map[schema.GroupVersionKind][]GvrAndScope{
	schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}:           append(make([]GvrAndScope, 0), replicaSetGvrAndScope),
	schema.GroupVersionKind{Group: "argoproj.io", Version: "v1alpha1", Kind: "Rollout"}: append(make([]GvrAndScope, 0), replicaSetGvrAndScope),
	schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"}:           append(make([]GvrAndScope, 0), podsGvrAndScope),
	schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"}:             append(make([]GvrAndScope, 0), jobGvrAndScope),
	schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}:                 append(make([]GvrAndScope, 0), podsGvrAndScope),
	schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}:          append(make([]GvrAndScope, 0), podsGvrAndScope),
	schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"}:            append(make([]GvrAndScope, 0), podsGvrAndScope),
	schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}:                  append(make([]GvrAndScope, 0), endpointsGvrAndScope, endpointSliceGvrAndScope),
}

// constants end

type GvrAndScope struct {
	Gvr   schema.GroupVersionResource
	Scope meta.RESTScopeName
}

const DEFAULT_CLUSTER = "default_cluster"

func GetGvkVsChildGvrAndScope() map[schema.GroupVersionKind][]GvrAndScope {
	return gvkVsChildGvrAndScope
}

func GetRestConfig(config *client.ClusterConfig) (restConfig *rest.Config, err error) {
	if config.ClusterName == DEFAULT_CLUSTER && len(config.Token) == 0 {
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
		return restConfig, err
	} else {
		restConfig = &rest.Config{Host: config.ApiServerUrl, BearerToken: config.Token, TLSClientConfig: rest.TLSClientConfig{Insecure: true}}
		return restConfig, err
	}
	return nil, nil
}
