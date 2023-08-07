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
	"github.com/devtron-labs/kubelink/pkg/util/kube"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

// constants starts
var podsGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}, Scope: meta.RESTScopeNameNamespace}
var replicaSetGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}, Scope: meta.RESTScopeNameNamespace}
var jobGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}, Scope: meta.RESTScopeNameNamespace}
var endpointsGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "endpoints"}, Scope: meta.RESTScopeNameNamespace}
var endpointSliceV1Beta1GvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "discovery.k8s.io", Version: "v1beta1", Resource: "endpointslices"}, Scope: meta.RESTScopeNameNamespace}
var endpointSliceV1GvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"}, Scope: meta.RESTScopeNameNamespace}
var pvGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumes"}, Scope: meta.RESTScopeNameRoot}
var pvcGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumeclaims"}, Scope: meta.RESTScopeNameNamespace}
var stsGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, Scope: meta.RESTScopeNameNamespace}
var configGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}, Scope: meta.RESTScopeNameNamespace}
var hpaGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"}, Scope: meta.RESTScopeNameNamespace}
var deployGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, Scope: meta.RESTScopeNameNamespace}
var serviceGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}, Scope: meta.RESTScopeNameNamespace}
var daemonGvrAndScope = GvrAndScope{Gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}, Scope: meta.RESTScopeNameNamespace}

var gvkVsChildGvrAndScope = map[schema.GroupVersionKind][]GvrAndScope{
	schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}:                     append(make([]GvrAndScope, 0), replicaSetGvrAndScope),
	schema.GroupVersionKind{Group: "argoproj.io", Version: "v1alpha1", Kind: "Rollout"}:           append(make([]GvrAndScope, 0), replicaSetGvrAndScope),
	schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"}:                     append(make([]GvrAndScope, 0), podsGvrAndScope),
	schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"}:                       append(make([]GvrAndScope, 0), jobGvrAndScope),
	schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}:                           append(make([]GvrAndScope, 0), podsGvrAndScope),
	schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}:                    append(make([]GvrAndScope, 0), podsGvrAndScope, pvcGvrAndScope, stsGvrAndScope),
	schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"}:                      append(make([]GvrAndScope, 0), podsGvrAndScope),
	schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}:                            append(make([]GvrAndScope, 0), endpointsGvrAndScope, endpointSliceV1Beta1GvrAndScope, endpointSliceV1GvrAndScope),
	schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1", Kind: "Prometheus"}:    append(make([]GvrAndScope, 0), stsGvrAndScope, configGvrAndScope),
	schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1", Kind: "Alertmanager"}:  append(make([]GvrAndScope, 0), stsGvrAndScope, configGvrAndScope),
	schema.GroupVersionKind{Group: "keda.sh", Version: "v1alpha1", Kind: "ScaledObject"}:          append(make([]GvrAndScope, 0), hpaGvrAndScope),
	schema.GroupVersionKind{Group: "autoscaling", Version: "v2", Kind: "HorizontalPodAutoscaler"}: append(make([]GvrAndScope, 0), podsGvrAndScope),
	schema.GroupVersionKind{Group: "flagger.app", Version: "v1beta1", Kind: "Canary"}:             append(make([]GvrAndScope, 0), deployGvrAndScope, serviceGvrAndScope, daemonGvrAndScope),
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

//var K8sNativeGroups = []string{"", "admissionregistration.k8s.io", "apiextensions.k8s.io", "apiregistration.k8s.io", "apps", "authentication.k8s.io", "authorization.k8s.io",
//	"autoscaling", "batch", "certificates.k8s.io", "coordination.k8s.io", "core", "discovery.k8s.io", "events.k8s.io", "flowcontrol.apiserver.k8s.io", "argoproj.io",
//	"internal.apiserver.k8s.io", "networking.k8s.io", "node.k8s.io", "policy", "rbac.authorization.k8s.io", "resource.k8s.io", "scheduling.k8s.io", "storage.k8s.io"}

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
