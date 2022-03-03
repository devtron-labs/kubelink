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
	"context"
	"errors"
	"fmt"
	"github.com/devtron-labs/kubelink/bean"
	client "github.com/devtron-labs/kubelink/grpc"
	gitops_engine "github.com/devtron-labs/kubelink/pkg/util/gitops-engine"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	dynamicClient "k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
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
	schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}:  append(make([]GvrAndScope, 0), replicaSetGvrAndScope),
	schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"}:  append(make([]GvrAndScope, 0), podsGvrAndScope),
	schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"}:    append(make([]GvrAndScope, 0), jobGvrAndScope),
	schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}:        append(make([]GvrAndScope, 0), podsGvrAndScope),
	schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}: append(make([]GvrAndScope, 0), podsGvrAndScope),
	schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"}:   append(make([]GvrAndScope, 0), podsGvrAndScope),
	schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}:         append(make([]GvrAndScope, 0), endpointsGvrAndScope, endpointSliceGvrAndScope),
}

// constants end

type ClusterConfig struct {
	Host        string
	BearerToken string
}

type GvrAndScope struct {
	Gvr   schema.GroupVersionResource
	Scope meta.RESTScopeName
}

func CanHaveChild(gvk schema.GroupVersionKind) bool {
	_, ok := gvkVsChildGvrAndScope[gvk]
	return ok
}

func GetLiveManifest(restConfig *rest.Config, namespace string, gvk *schema.GroupVersionKind, name string) (*unstructured.Unstructured, *schema.GroupVersionResource, error) {
	gvr, scope, err := getGvrAndScopeFromGvk(gvk, restConfig)
	if err != nil {
		return nil, nil, err
	}

	dynamicClient, err := dynamicClient.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, err
	}
	if scope.Name() != meta.RESTScopeNameNamespace {
		manifest, err := dynamicClient.Resource(*gvr).Get(context.Background(), name, metav1.GetOptions{})
		return manifest, gvr, err
	} else {
		manifest, err := dynamicClient.Resource(*gvr).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
		return manifest, gvr, err
	}
}
func GetChildObjects(restConfig *rest.Config, namespace string, parentGvk schema.GroupVersionKind, parentName string, parentApiVersion string) ([]*unstructured.Unstructured, error) {

	gvrAndScopes, ok := gvkVsChildGvrAndScope[parentGvk]
	if !ok {
		return nil, errors.New("grv not found for given kind")
	}
	client, err := dynamicClient.NewForConfig(restConfig)

	if err != nil {
		return nil, err
	}

	var manifests []*unstructured.Unstructured
	for _, gvrAndScope := range gvrAndScopes {
		gvr := gvrAndScope.Gvr
		scope := gvrAndScope.Scope

		var objects *unstructured.UnstructuredList
		if scope != meta.RESTScopeNameNamespace {
			objects, err = client.Resource(gvr).List(context.Background(), metav1.ListOptions{})
		} else {
			objects, err = client.Resource(gvr).Namespace(namespace).List(context.Background(), metav1.ListOptions{})
		}

		if err != nil {
			return nil, err
		}

		for _, item := range objects.Items {
			ownerRefs, _ := gitops_engine.ResolveResourceReferences(&item)
			item.SetOwnerReferences(ownerRefs)
			for _, ownerRef := range item.GetOwnerReferences() {
				if ownerRef.Name == parentName && ownerRef.Kind == parentGvk.Kind && ownerRef.APIVersion == parentApiVersion {
					// using deep copy as it replaces item in manifest in loop
					manifests = append(manifests, item.DeepCopy())
				}
			}
		}
	}

	return manifests, nil
}

func PatchResource(ctx context.Context, restConfig *rest.Config, r *bean.KubernetesResourcePatchRequest) error {
	client, err := dynamicClient.NewForConfig(restConfig)
	if err != nil {
		return err
	}
	_, err = client.Resource(r.GroupVersionResource).Namespace(r.Namespace).Patch(ctx, r.Name, types.PatchType(r.PatchType), []byte(r.Patch), metav1.PatchOptions{})
	if err != nil {
		return err
	}
	return nil
}

func getGvrAndScopeFromGvk(gvk *schema.GroupVersionKind, restConfig *rest.Config) (*schema.GroupVersionResource, meta.RESTScope, error) {
	descoClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, nil, err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(descoClient))
	restMapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, nil, err
	}
	if restMapping == nil {
		return nil, nil, errors.New("gvr not found for given gvk")
	}
	return &restMapping.Resource, restMapping.Scope, nil
}

const DEFAULT_CLUSTER = "default_cluster"

func GetRestConfig(config *client.ClusterConfig) (restConfig *rest.Config, err error) {
	if config.ClusterName == DEFAULT_CLUSTER && len(config.Token) == 0 {
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
		fmt.Println("using InClusterConfig")
		return restConfig, err
	} else {
		restConfig = &rest.Config{Host: config.ApiServerUrl, BearerToken: config.Token, TLSClientConfig: rest.TLSClientConfig{Insecure: true}}
		return restConfig, err
	}
	return nil, nil
}
