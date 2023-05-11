package service

import (
	"context"
	"errors"
	"github.com/argoproj/gitops-engine/pkg/utils/kube"
	"github.com/devtron-labs/kubelink/bean"
	gitops_engine "github.com/devtron-labs/kubelink/pkg/util/gitops-engine"
	k8sUtils "github.com/devtron-labs/kubelink/pkg/util/k8s"
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

type K8sServiceImpl struct {
	logger *zap.SugaredLogger
}

func NewK8sServiceImpl(logger *zap.SugaredLogger) *K8sServiceImpl {
	return &K8sServiceImpl{
		logger: logger,
	}
}

func (impl K8sServiceImpl) CanHaveChild(gvk schema.GroupVersionKind) bool {
	_, ok := k8sUtils.GetGvkVsChildGvrAndScope()[gvk]
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

	gvrAndScopes, ok := k8sUtils.GetGvkVsChildGvrAndScope()[parentGvk]
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
				return nil, err
			}
		}

		if objects != nil {
			for _, item := range objects.Items {
				ownerRefs, isInferredParentOf := gitops_engine.ResolveResourceReferences(&item)
				if parentGvk.Kind == kube.StatefulSetKind && gvr.Resource == k8sUtils.PersistentVolumeClaimsResourceType {
					pvcs = append(pvcs, item)
					continue
				}
				//special handling for pvcs created via statefulsets
				if gvr.Resource == k8sUtils.StatefulSetsResourceType && isInferredParentOf != nil {
					for _, pvc := range pvcs {
						var pvcClaim coreV1.PersistentVolumeClaim
						err := runtime.DefaultUnstructuredConverter.FromUnstructured(pvc.Object, &pvcClaim)
						if err != nil {
							return manifests, err
						}
						isCurrentStsParentOfPvc := isInferredParentOf(kube.ResourceKey{
							Group:     "",
							Kind:      pvcClaim.Kind,
							Namespace: namespace,
							Name:      pvcClaim.Name,
						})
						if isCurrentStsParentOfPvc {
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
