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
	"github.com/argoproj/gitops-engine/pkg/utils/kube"
	client "github.com/devtron-labs/kubelink/grpc"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

// constants starts
var PodsGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}, Scope: meta.RESTScopeNameNamespace}
var ReplicaSetGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}, Scope: meta.RESTScopeNameNamespace}
var JobGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}, Scope: meta.RESTScopeNameNamespace}
var EndpointsGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "endpoints"}, Scope: meta.RESTScopeNameNamespace}
var EndpointSliceV1Beta1GvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "discovery.k8s.io", Version: "v1beta1", Resource: "endpointslices"}, Scope: meta.RESTScopeNameNamespace}
var EndpointSliceV1GvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"}, Scope: meta.RESTScopeNameNamespace}
var PvGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumes"}, Scope: meta.RESTScopeNameRoot}
var PvcGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumeclaims"}, Scope: meta.RESTScopeNameNamespace}
var StsGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, Scope: meta.RESTScopeNameNamespace}
var ConfigGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}, Scope: meta.RESTScopeNameNamespace}

var gvkVsChildGvrAndScope = map[schema.GroupVersionKind][]GvrAndScope{
	schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}:           append(make([]GvrAndScope, 0), ReplicaSetGvrAndScope),
	schema.GroupVersionKind{Group: "argoproj.io", Version: "v1alpha1", Kind: "Rollout"}: append(make([]GvrAndScope, 0), ReplicaSetGvrAndScope),
	schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"}:           append(make([]GvrAndScope, 0), PodsGvrAndScope),
	schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"}:             append(make([]GvrAndScope, 0), JobGvrAndScope),
	schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}:                 append(make([]GvrAndScope, 0), PodsGvrAndScope),
	schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}:          append(make([]GvrAndScope, 0), PodsGvrAndScope, PvcGvrAndScope, StsGvrAndScope),
	schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"}:            append(make([]GvrAndScope, 0), PodsGvrAndScope),
	schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}:                  append(make([]GvrAndScope, 0), EndpointsGvrAndScope, EndpointSliceV1Beta1GvrAndScope, EndpointSliceV1GvrAndScope),
}

// constants end

type GvrAndScope struct {
	Gvr   schema.GroupVersionResource
	Scope meta.RESTScopeName
}

const (
	DEFAULT_CLUSTER                    = "default_cluster"
	DEVTRON_SERVICE_NAME               = "devtron-service"
	DEVTRON_APP_LABEL_KEY              = "app"
	DEVTRON_APP_LABEL_VALUE1           = "devtron"
	DEVTRON_APP_LABEL_VALUE2           = "orchestrator"
	PersistentVolumeClaimsResourceType = "persistentvolumeclaims"
	StatefulSetsResourceType           = "statefulsets"
)

var K8sNativeGroups = []string{"", "admissionregistration.k8s.io", "apiextensions.k8s.io", "apiregistration.k8s.io", "apps", "authentication.k8s.io", "authorization.k8s.io",
	"autoscaling", "batch", "certificates.k8s.io", "coordination.k8s.io", "core", "discovery.k8s.io", "events.k8s.io", "flowcontrol.apiserver.k8s.io", "argoproj.io",
	"internal.apiserver.k8s.io", "networking.k8s.io", "node.k8s.io", "policy", "rbac.authorization.k8s.io", "resource.k8s.io", "scheduling.k8s.io", "storage.k8s.io"}

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

func IsService(gvk schema.GroupVersionKind) bool {
	return gvk.Group == "" && gvk.Kind == kube.ServiceKind
}

func IsPod(gvk schema.GroupVersionKind) bool {
	return gvk.Group == "" && gvk.Kind == kube.PodKind && gvk.Version == "v1"
}

func IsDevtronApp(labels map[string]string) bool {
	isDevtronApp := false
	if val, ok := labels[DEVTRON_APP_LABEL_KEY]; ok {
		if val == DEVTRON_APP_LABEL_VALUE1 || val == DEVTRON_APP_LABEL_VALUE2 {
			isDevtronApp = true
		}
	}
	return isDevtronApp
}
