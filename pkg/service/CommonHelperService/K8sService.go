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
	"context"
	"encoding/json"
	"errors"
	"github.com/caarlos0/env"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	k8sCommonBean "github.com/devtron-labs/common-lib/utils/k8s/commonBean"
	"github.com/devtron-labs/kubelink/bean"
	error2 "github.com/devtron-labs/kubelink/error"
	"go.uber.org/zap"
	coreV1 "k8s.io/api/core/v1"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	dynamicClient "k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
)

type ClusterConfig struct {
	Host        string
	BearerToken string
}

type K8sService interface {
	CanHaveChild(gvk schema.GroupVersionKind) bool
	GetLiveManifest(restConfig *rest.Config, namespace string, gvk *schema.GroupVersionKind, name string) (*unstructured.Unstructured, *schema.GroupVersionResource, error)
	GetChildObjects(restConfig *rest.Config, namespace string, parentGvk schema.GroupVersionKind, parentName string, parentApiVersion string) ([]*unstructured.Unstructured, error)
	PatchResource(ctx context.Context, restConfig *rest.Config, r *bean.KubernetesResourcePatchRequest) error
}

type HelmReleaseConfig struct {
	EnableHelmReleaseCache    bool   `env:"ENABLE_HELM_RELEASE_CACHE" envDefault:"true"`
	MaxCountForHelmRelease    int    `env:"MAX_COUNT_FOR_HELM_RELEASE" envDefault:"20"`
	ManifestFetchBatchSize    int    `env:"MANIFEST_FETCH_BATCH_SIZE" envDefault:"2"`
	RunHelmInstallInAsyncMode bool   `env:"RUN_HELM_INSTALL_IN_ASYNC_MODE" envDefault:"false"`
	ParentChildGvkMapping     string `env:"PARENT_CHILD_GVK_MAPPING" envDefault:""`
	ChartWorkingDirectory     string `env:"CHART_WORKING_DIRECTORY" envDefault:"/home/devtron/devtroncd/charts/"`
}

func GetHelmReleaseConfig() (*HelmReleaseConfig, error) {
	cfg := &HelmReleaseConfig{}
	err := env.Parse(cfg)
	return cfg, err
}

type K8sServiceImpl struct {
	logger                *zap.SugaredLogger
	helmReleaseConfig     *HelmReleaseConfig
	gvkVsChildGvrAndScope map[schema.GroupVersionKind][]*k8sCommonBean.GvrAndScope
}

func NewK8sServiceImpl(logger *zap.SugaredLogger, helmReleaseConfig *HelmReleaseConfig) (*K8sServiceImpl, error) {

	gvkVsChildGvrAndScope := make(map[schema.GroupVersionKind][]*k8sCommonBean.GvrAndScope)
	k8sServiceImpl := &K8sServiceImpl{
		logger:                logger,
		helmReleaseConfig:     helmReleaseConfig,
		gvkVsChildGvrAndScope: gvkVsChildGvrAndScope,
	}
	if len(helmReleaseConfig.ParentChildGvkMapping) > 0 {
		k8sServiceImpl.logger.Infow("caching parent gvk to child gvr and scope mapping")
		_, err := k8sServiceImpl.cacheParentChildGvkMapping(gvkVsChildGvrAndScope)
		if err != nil {
			k8sServiceImpl.logger.Errorw("error in caching parent gvk to child gvr and scope mapping", "err", err)
			return nil, err
		}
	}
	return k8sServiceImpl, nil
}

func (impl K8sServiceImpl) cacheParentChildGvkMapping(gvkVsChildGvrAndScope map[schema.GroupVersionKind][]*k8sCommonBean.GvrAndScope) (map[schema.GroupVersionKind][]*k8sCommonBean.GvrAndScope, error) {
	var gvkChildMappings []ParentChildGvkMapping
	parentChildGvkMapping := impl.helmReleaseConfig.ParentChildGvkMapping
	err := json.Unmarshal([]byte(parentChildGvkMapping), &gvkChildMappings)
	if err != nil {
		impl.logger.Errorw("error in unmarshalling ParentChildGvkMapping", "parentChildGvkMapping", parentChildGvkMapping, "err", err)
		return gvkVsChildGvrAndScope, err
	}
	for _, parent := range gvkChildMappings {
		childGvrAndScopes := make([]*k8sCommonBean.GvrAndScope, len(parent.ChildObjects))
		for i, childObj := range parent.ChildObjects {
			childGvrAndScopes[i] = childObj.GetGvrAndScopeForChildObject()
		}
		gvkVsChildGvrAndScope[parent.GetParentGvk()] = childGvrAndScopes
	}
	return gvkVsChildGvrAndScope, nil
}

func (impl K8sServiceImpl) GetChildGvrFromParentGvk(parentGvk schema.GroupVersionKind) ([]*k8sCommonBean.GvrAndScope, bool) {
	var gvrAndScopes []*k8sCommonBean.GvrAndScope
	var ok bool
	//if parent child gvk mapping found from CM override it over local hardcoded gvk mapping
	if len(impl.helmReleaseConfig.ParentChildGvkMapping) > 0 && len(impl.gvkVsChildGvrAndScope) > 0 {
		gvrAndScopes, ok = impl.gvkVsChildGvrAndScope[parentGvk]
	} else {
		gvrAndScopes, ok = k8sCommonBean.GetGvkVsChildGvrAndScope()[parentGvk]
	}
	return gvrAndScopes, ok
}

func (impl K8sServiceImpl) CanHaveChild(gvk schema.GroupVersionKind) bool {
	_, ok := impl.GetChildGvrFromParentGvk(gvk)
	return ok
}

func (impl K8sServiceImpl) GetLiveManifest(restConfig *rest.Config, namespace string, gvk *schema.GroupVersionKind, name string) (*unstructured.Unstructured, *schema.GroupVersionResource, error) {
	impl.logger.Debugw("Getting live manifest ", "namespace", namespace, "gvk", gvk, "name", name)

	gvr, scope, err := impl.getGvrAndScopeFromGvk(gvk, restConfig)
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

func (impl K8sServiceImpl) GetChildObjects(restConfig *rest.Config, namespace string, parentGvk schema.GroupVersionKind, parentName string, parentApiVersion string) ([]*unstructured.Unstructured, error) {
	impl.logger.Debugw("Getting child objects ", "namespace", namespace, "parentGvk", parentGvk, "parentName", parentName, "parentApiVersion", parentApiVersion)

	gvrAndScopes, ok := impl.GetChildGvrFromParentGvk(parentGvk)
	if !ok {
		return nil, errors.New("grv not found for given kind")
	}
	client, err := dynamicClient.NewForConfig(restConfig)

	if err != nil {
		return nil, err
	}
	var pvcs []unstructured.Unstructured
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
			statusError, ok := err.(*errors2.StatusError)
			if !ok || statusError.ErrStatus.Reason != metav1.StatusReasonNotFound {
				internalErr := error2.ConvertHelmErrorToInternalError(err)
				if internalErr != nil {
					err = internalErr
				}
				return nil, err
			}
		}

		if objects != nil {
			for _, item := range objects.Items {
				ownerRefs, isInferredParentOf := k8sUtils.ResolveResourceReferences(&item)
				if parentGvk.Kind == k8sCommonBean.StatefulSetKind && gvr.Resource == k8sCommonBean.PersistentVolumeClaimsResourceType {
					pvcs = append(pvcs, item)
					continue
				}
				//special handling for pvcs created via statefulsets
				if gvr.Resource == k8sCommonBean.StatefulSetsResourceType && isInferredParentOf != nil {
					for _, pvc := range pvcs {
						var pvcClaim coreV1.PersistentVolumeClaim
						err := runtime.DefaultUnstructuredConverter.FromUnstructured(pvc.Object, &pvcClaim)
						if err != nil {
							return manifests, err
						}
						isCurrentStsParentOfPvc := isInferredParentOf(k8sUtils.ResourceKey{
							Group:     "",
							Kind:      pvcClaim.Kind,
							Namespace: namespace,
							Name:      pvcClaim.Name,
						})
						if isCurrentStsParentOfPvc && item.GetName() == parentName {
							manifests = append(manifests, pvc.DeepCopy())
						}
					}
				}
				item.SetOwnerReferences(ownerRefs)
				for _, ownerRef := range item.GetOwnerReferences() {
					if ownerRef.Name == parentName && ownerRef.Kind == parentGvk.Kind && ownerRef.APIVersion == parentApiVersion {
						// using deep copy as it replaces item in manifest in loop
						manifests = append(manifests, item.DeepCopy())
					}
				}
			}
		}

	}

	return manifests, nil
}

func (impl K8sServiceImpl) PatchResource(ctx context.Context, restConfig *rest.Config, r *bean.KubernetesResourcePatchRequest) error {
	impl.logger.Debugw("Patching resource ", "namespace", r.Namespace, "name", r.Name)

	dynamicClient, err := dynamicClient.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	gvr, scope, err := impl.getGvrAndScopeFromGvk(r.Gvk, restConfig)
	if err != nil {
		return err
	}

	if scope.Name() != meta.RESTScopeNameNamespace {
		_, err = dynamicClient.Resource(*gvr).Patch(ctx, r.Name, types.PatchType(r.PatchType), []byte(r.Patch), metav1.PatchOptions{})
	} else {
		_, err = dynamicClient.Resource(*gvr).Namespace(r.Namespace).Patch(ctx, r.Name, types.PatchType(r.PatchType), []byte(r.Patch), metav1.PatchOptions{})
	}

	if err != nil {
		return err
	}

	return nil
}

func (impl K8sServiceImpl) getGvrAndScopeFromGvk(gvk *schema.GroupVersionKind, restConfig *rest.Config) (*schema.GroupVersionResource, meta.RESTScope, error) {
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
