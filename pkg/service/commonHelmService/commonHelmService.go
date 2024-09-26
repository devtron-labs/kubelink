package commonHelmService

import (
	"errors"
	"fmt"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	k8sCommonBean "github.com/devtron-labs/common-lib/utils/k8s/commonBean"
	k8sObjectUtils "github.com/devtron-labs/common-lib/utils/k8sObjectsUtil"
	yamlUtil "github.com/devtron-labs/common-lib/utils/yaml"
	"github.com/devtron-labs/common-lib/workerPool"
	"github.com/devtron-labs/kubelink/bean"
	globalConfig "github.com/devtron-labs/kubelink/config"
	"github.com/devtron-labs/kubelink/converter"
	client "github.com/devtron-labs/kubelink/grpc"
	"github.com/devtron-labs/kubelink/pkg/cache"
	"github.com/devtron-labs/kubelink/pkg/helmClient"
	"github.com/devtron-labs/kubelink/pkg/util"
	"github.com/devtron-labs/kubelink/pkg/util/argo"
	"go.uber.org/zap"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/api/extensions/v1beta1"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"net/http"
	"sigs.k8s.io/yaml"
	"sync"
)

type CommonHelmService interface {
	GetHelmRelease(clusterConfig *client.ClusterConfig, namespace string, releaseName string) (*release.Release, error)
	BuildResourceTree(appDetailRequest *client.AppDetailRequest, release *release.Release) (*bean.ResourceTreeResponse, error)
	BuildNodes(request *BuildNodesConfig) (*BuildNodeResponse, error)
	GetResourceTreeForExternalResources(req *client.ExternalResourceTreeRequest) (*bean.ResourceTreeResponse, error)
	GetParentGvkListForApp(appConfig *client.AppConfigRequest) ([]*client.Gvk, error)
}

type CommonHelmServiceImpl struct {
	k8sService        K8sService
	logger            *zap.SugaredLogger
	k8sUtil           k8sUtils.K8sService
	converter         converter.ClusterBeanConverter
	helmReleaseConfig *globalConfig.HelmReleaseConfig
}

func NewCommonHelmServiceImpl(logger *zap.SugaredLogger,
	k8sUtil k8sUtils.K8sService, converter converter.ClusterBeanConverter,
	k8sService K8sService, helmReleaseConfig *globalConfig.HelmReleaseConfig) *CommonHelmServiceImpl {
	return &CommonHelmServiceImpl{
		logger:            logger,
		k8sUtil:           k8sUtil,
		converter:         converter,
		k8sService:        k8sService,
		helmReleaseConfig: helmReleaseConfig,
	}

}

func (impl *CommonHelmServiceImpl) GetHelmRelease(clusterConfig *client.ClusterConfig, namespace string, releaseName string) (*release.Release, error) {

	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(clusterConfig)
	conf, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		return nil, err
	}
	opt := &helmClient.RestConfClientOptions{
		Options: &helmClient.Options{
			Namespace: namespace,
		},
		RestConfig: conf,
	}
	helmClient, err := helmClient.NewClientFromRestConf(opt)
	if err != nil {
		return nil, err
	}
	release, err := helmClient.GetRelease(releaseName)
	if err != nil {
		return nil, err
	}
	return release, nil
}
func (impl *CommonHelmServiceImpl) BuildResourceTree(appDetailRequest *client.AppDetailRequest, release *release.Release) (*bean.ResourceTreeResponse, error) {
	conf, err := impl.getRestConfigForClusterConfig(appDetailRequest.ClusterConfig)
	if err != nil {
		return nil, err
	}
	desiredOrLiveManifests, err := impl.getLiveManifests(conf, release)
	if err != nil {
		return nil, err
	}
	// build resource Nodes
	req := NewBuildNodesRequest(NewBuildNodesConfig(conf).
		WithReleaseNamespace(appDetailRequest.Namespace)).
		WithDesiredOrLiveManifests(desiredOrLiveManifests...).
		WithBatchWorker(impl.helmReleaseConfig.BuildNodesBatchSize, impl.logger)
	buildNodesResponse, err := impl.BuildNodes(req)
	if err != nil {
		return nil, err
	}
	updateHookInfoForChildNodes(buildNodesResponse.Nodes)

	// filter Nodes based on ResourceTreeFilter
	resourceTreeFilter := appDetailRequest.ResourceTreeFilter
	if resourceTreeFilter != nil && len(buildNodesResponse.Nodes) > 0 {
		buildNodesResponse.Nodes = impl.filterNodes(resourceTreeFilter, buildNodesResponse.Nodes)
	}

	// build pods metadata
	podsMetadata, err := impl.buildPodMetadata(buildNodesResponse.Nodes, conf)
	if err != nil {
		return nil, err
	}
	resourceTreeResponse := &bean.ResourceTreeResponse{
		ApplicationTree: &bean.ApplicationTree{
			Nodes: buildNodesResponse.Nodes,
		},
		PodMetadata: podsMetadata,
	}
	return resourceTreeResponse, nil
}
func (impl *CommonHelmServiceImpl) getRestConfigForClusterConfig(clusterConfig *client.ClusterConfig) (*rest.Config, error) {
	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(clusterConfig)
	conf, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		impl.logger.Errorw("error in getting rest config by cluster", "clusterName", k8sClusterConfig.ClusterName)
		return nil, err
	}
	return conf, nil
}

func (impl *CommonHelmServiceImpl) GetParentGvkListForApp(appConfig *client.AppConfigRequest) ([]*client.Gvk, error) {
	if appConfig == nil {
		return nil, errors.New("appConfig is nil")
	}
	helmRelease, err := impl.GetHelmRelease(appConfig.ClusterConfig, appConfig.Namespace, appConfig.ReleaseName)
	if err != nil {
		impl.logger.Errorw("Error in getting helm release", "appConfig", appConfig, "err", err)
		return nil, err
	}
	if helmRelease == nil {
		return nil, errors.New(fmt.Sprintf("no helm release found for name=%s", appConfig.ReleaseName))
	}
	manifests, err := impl.getManifestsFromHelmRelease(helmRelease)
	if err != nil {
		impl.logger.Errorw("Error in getting helm release", "helmRelease", helmRelease, "err", err)
		return nil, err
	}

	gvkList := make([]*client.Gvk, len(manifests))
	for _, manifest := range manifests {
		gvk := manifest.GroupVersionKind()
		if len(gvk.Kind) > 0 && len(gvk.Group) > 0 {
			gvkList = append(gvkList, GetClientGVKFromSchemaGVK(gvk))
		}

	}

	return gvkList, nil
}

func (impl *CommonHelmServiceImpl) getManifestsFromHelmRelease(helmRelease *release.Release) ([]unstructured.Unstructured, error) {
	manifests, err := yamlUtil.SplitYAMLs([]byte(helmRelease.Manifest))
	if err != nil {
		return nil, err
	}
	manifests = impl.addHookResourcesInManifest(helmRelease, manifests)
	return manifests, nil
}

func (impl *CommonHelmServiceImpl) getLiveManifests(config *rest.Config, helmRelease *release.Release) ([]*bean.DesiredOrLiveManifest, error) {
	manifests, err := impl.getManifestsFromHelmRelease(helmRelease)
	if err != nil {
		impl.logger.Errorw("error in parsing manifests", "payload", helmRelease, "error", err)
		return nil, err
	}
	// get live manifests from kubernetes
	//impl.logger.Infow("manifests added", "manifests", manifests, "helmRelease", helmRelease.Name)
	desiredOrLiveManifests, err := impl.getDesiredOrLiveManifests(config, manifests, helmRelease.Namespace)
	if err != nil {
		impl.logger.Errorw("error in getting desired or live manifest", "host", config.Host, "helmReleaseName", helmRelease.Name, "err", err)
		return nil, err
	}
	return desiredOrLiveManifests, nil
}
func (impl *CommonHelmServiceImpl) addHookResourcesInManifest(helmRelease *release.Release, manifests []unstructured.Unstructured) []unstructured.Unstructured {
	for _, helmHook := range helmRelease.Hooks {
		var hook unstructured.Unstructured
		err := yaml.Unmarshal([]byte(helmHook.Manifest), &hook)
		if err != nil {
			impl.logger.Errorw("error in converting string manifest into unstructured obj", "hookName", helmHook.Name, "releaseName", helmRelease.Name, "err", err)
			continue
		}
		manifests = append(manifests, hook)
	}
	return manifests
}
func (impl *CommonHelmServiceImpl) getDesiredOrLiveManifests(restConfig *rest.Config, desiredManifests []unstructured.Unstructured, releaseNamespace string) ([]*bean.DesiredOrLiveManifest, error) {

	totalManifestCount := len(desiredManifests)
	desiredOrLiveManifestArray := make([]*bean.DesiredOrLiveManifest, totalManifestCount)
	batchSize := impl.helmReleaseConfig.ManifestFetchBatchSize

	for i := 0; i < totalManifestCount; {
		// requests left to process
		remainingBatch := totalManifestCount - i
		if remainingBatch < batchSize {
			batchSize = remainingBatch
		}
		var wg sync.WaitGroup
		for j := 0; j < batchSize; j++ {
			wg.Add(1)
			go func(j int) {
				defer wg.Done()
				desiredOrLiveManifest := impl.getManifestData(restConfig, releaseNamespace, desiredManifests[i+j])
				desiredOrLiveManifestArray[i+j] = desiredOrLiveManifest
			}(j)
		}
		wg.Wait()
		i += batchSize
	}

	return desiredOrLiveManifestArray, nil
}
func (impl *CommonHelmServiceImpl) getManifestData(restConfig *rest.Config, releaseNamespace string, desiredManifest unstructured.Unstructured) *bean.DesiredOrLiveManifest {
	gvk := desiredManifest.GroupVersionKind()
	_namespace := desiredManifest.GetNamespace()
	if _namespace == "" {
		_namespace = releaseNamespace
	}
	liveManifest, _, err := impl.k8sService.GetLiveManifest(restConfig, _namespace, &gvk, desiredManifest.GetName())
	desiredOrLiveManifest := &bean.DesiredOrLiveManifest{}

	if err != nil {
		impl.logger.Errorw("Error in getting live manifest ", "err", err)
		statusError, _ := err.(*errors2.StatusError)
		desiredOrLiveManifest = &bean.DesiredOrLiveManifest{
			// using deep copy as it replaces item in manifest in loop
			Manifest:                 desiredManifest.DeepCopy(),
			IsLiveManifestFetchError: true,
		}
		if statusError != nil {
			desiredOrLiveManifest.LiveManifestFetchErrorCode = statusError.Status().Code
		}
	} else {
		desiredOrLiveManifest = &bean.DesiredOrLiveManifest{
			Manifest: liveManifest,
		}
	}
	return desiredOrLiveManifest
}

// BuildNodes builds Nodes from desired or live manifest.
//   - It uses recursive approach to build child Nodes.
//   - It uses batch worker to build child Nodes in parallel.
//   - Batch workers configuration is provided in BuildNodesConfig.
//   - NOTE: To avoid creating batch worker recursively, it does not use batch worker for child Nodes.
func (impl *CommonHelmServiceImpl) BuildNodes(request *BuildNodesConfig) (*BuildNodeResponse, error) {
	var buildChildNodesRequests []*BuildNodesConfig
	response := NewBuildNodeResponse()
	for _, desiredOrLiveManifest := range request.DesiredOrLiveManifests {

		// build request to get Nodes from desired or live manifest
		getNodesFromManifest := NewGetNodesFromManifest(NewBuildNodesConfig(request.RestConfig).
			WithParentResourceRef(request.ParentResourceRef).
			WithReleaseNamespace(request.ReleaseNamespace)).
			WithDesiredOrLiveManifest(desiredOrLiveManifest)

		// get Node from desired or live manifest
		getNodesFromManifestResponse, err := impl.getNodeFromDesiredOrLiveManifest(getNodesFromManifest)
		if err != nil {
			return response, err
		}
		// add Node and health status
		if getNodesFromManifestResponse.Node != nil {
			response.Nodes = append(response.Nodes, getNodesFromManifestResponse.Node)
			response.HealthStatusArray = append(response.HealthStatusArray, getNodesFromManifestResponse.Node.Health)
		}

		// add child Nodes request
		if len(getNodesFromManifestResponse.DesiredOrLiveChildrenManifests) > 0 {
			req := NewBuildNodesRequest(NewBuildNodesConfig(request.RestConfig).
				WithReleaseNamespace(request.ReleaseNamespace).
				WithParentResourceRef(getNodesFromManifestResponse.ResourceRef)).
				WithDesiredOrLiveManifests(getNodesFromManifestResponse.DesiredOrLiveChildrenManifests...)
			// NOTE:  Do not use batch worker for child Nodes as it will create batch worker recursively
			buildChildNodesRequests = append(buildChildNodesRequests, req)
		}
	}
	// build child Nodes, if any.
	// NOTE: build child Nodes calls buildNodes recursively
	childNodeResponse, err := impl.buildChildNodesInBatch(request.batchWorker, buildChildNodesRequests)
	if err != nil {
		return response, err
	}
	// add child Nodes and health status to response
	response.WithNodes(childNodeResponse.Nodes).WithHealthStatusArray(childNodeResponse.HealthStatusArray)
	return response, nil
}

// buildChildNodes builds child Nodes sequentially from desired or live manifest.
func (impl *CommonHelmServiceImpl) buildChildNodes(buildChildNodesRequests []*BuildNodesConfig) (*BuildNodeResponse, error) {
	response := NewBuildNodeResponse()
	// for recursive calls, build child Nodes sequentially
	for _, req := range buildChildNodesRequests {
		// build child Nodes
		childNodesResponse, err := impl.BuildNodes(req)
		if err != nil {
			impl.logger.Errorw("error in building child Nodes", "ReleaseNamespace", req.ReleaseNamespace, "parentResource", req.ParentResourceRef.GetGvk(), "err", err)
			return response, err
		}
		response.WithNodes(childNodesResponse.Nodes).WithHealthStatusArray(childNodesResponse.HealthStatusArray)
	}
	return response, nil
}

// buildChildNodesInBatch builds child Nodes in parallel from desired or live manifest.
//   - It uses batch workers workerPool.WorkerPool[*BuildNodeResponse] to build child Nodes in parallel.
//   - If workerPool is not defined, it builds child Nodes sequentially.
func (impl *CommonHelmServiceImpl) buildChildNodesInBatch(wp *workerPool.WorkerPool[*BuildNodeResponse], buildChildNodesRequests []*BuildNodesConfig) (*BuildNodeResponse, error) {
	if wp == nil {
		// build child Nodes sequentially
		return impl.buildChildNodes(buildChildNodesRequests)
	}
	response := NewBuildNodeResponse()
	for index := range buildChildNodesRequests {
		// passing buildChildNodesRequests[index] to closure as it will be updated in next iteration and the func call is async
		func(req *BuildNodesConfig) {
			// submit child Nodes build request to workerPool
			wp.Submit(func() (*BuildNodeResponse, error) {
				// build child Nodes
				return impl.BuildNodes(req)
			})
		}(buildChildNodesRequests[index])
	}
	// wait for all child Nodes build requests to complete and return error from workerPool error channel
	err := wp.StopWait()
	if err != nil {
		return response, err
	}
	// extract the children nodes from workerPool response
	for _, childNode := range wp.GetResponse() {
		response.WithNodes(childNode.Nodes).WithHealthStatusArray(childNode.HealthStatusArray)
	}
	return response, nil
}

func (impl *CommonHelmServiceImpl) getNodeFromDesiredOrLiveManifest(request *GetNodeFromManifestRequest) (*GetNodeFromManifestResponse, error) {
	response := NewGetNodesFromManifestResponse()
	manifest := request.DesiredOrLiveManifest.Manifest
	gvk := manifest.GroupVersionKind()
	_namespace := manifest.GetNamespace()
	if _namespace == "" {
		_namespace = request.ReleaseNamespace
	}
	ports := util.GetPorts(manifest, gvk)
	resourceRef := BuildResourceRef(gvk, *manifest, _namespace)

	if impl.k8sService.CanHaveChild(gvk) {
		children, err := impl.k8sService.GetChildObjects(request.RestConfig, _namespace, gvk, manifest.GetName(), manifest.GetAPIVersion())
		if err != nil {
			return response, err
		}
		desiredOrLiveManifestsChildren := make([]*bean.DesiredOrLiveManifest, 0, len(children))
		for _, child := range children {
			desiredOrLiveManifestsChildren = append(desiredOrLiveManifestsChildren, &bean.DesiredOrLiveManifest{
				Manifest: child,
			})
		}
		response.WithParentResourceRef(resourceRef).
			WithDesiredOrLiveManifests(desiredOrLiveManifestsChildren...)
	}

	creationTimeStamp := ""
	val, found, err := unstructured.NestedString(manifest.Object, "metadata", "creationTimestamp")
	if found && err == nil {
		creationTimeStamp = val
	}
	node := &bean.ResourceNode{
		ResourceRef:     resourceRef,
		ResourceVersion: manifest.GetResourceVersion(),
		NetworkingInfo: &bean.ResourceNetworkingInfo{
			Labels: manifest.GetLabels(),
		},
		CreatedAt: creationTimeStamp,
		Port:      ports,
	}
	node.IsHook, node.HookType = util.GetHookMetadata(manifest)

	if request.ParentResourceRef != nil {
		node.ParentRefs = append(make([]*bean.ResourceRef, 0), request.ParentResourceRef)
	}

	// set health of Node
	if request.DesiredOrLiveManifest.IsLiveManifestFetchError {
		if request.DesiredOrLiveManifest.LiveManifestFetchErrorCode == http.StatusNotFound {
			node.Health = &bean.HealthStatus{
				Status:  bean.HealthStatusMissing,
				Message: "Resource missing as live manifest not found",
			}
		} else {
			node.Health = &bean.HealthStatus{
				Status:  bean.HealthStatusUnknown,
				Message: "Resource state unknown as error while fetching live manifest",
			}
		}
	} else {
		cache.SetHealthStatusForNode(node, manifest, gvk)
	}

	// hibernate set starts
	if request.ParentResourceRef == nil {

		// set CanBeHibernated
		cache.SetHibernationRules(node, &node.Manifest)
	}
	// hibernate set ends

	if k8sUtils.IsPod(gvk) {
		infoItems, _ := argo.PopulatePodInfo(manifest)
		node.Info = infoItems
	}
	util.AddSelectiveInfoInResourceNode(node, gvk, manifest.Object)

	response.WithNode(node).WithHealthStatus(node.Health)
	return response, nil
}

func (impl *CommonHelmServiceImpl) filterNodes(resourceTreeFilter *client.ResourceTreeFilter, nodes []*bean.ResourceNode) []*bean.ResourceNode {
	resourceFilters := resourceTreeFilter.ResourceFilters
	globalFilter := resourceTreeFilter.GlobalFilter
	if globalFilter == nil && (resourceFilters == nil || len(resourceFilters) == 0) {
		return nodes
	}

	filteredNodes := make([]*bean.ResourceNode, 0, len(nodes))

	// handle global
	if globalFilter != nil && len(globalFilter.Labels) > 0 {
		globalLabels := globalFilter.Labels
		for _, node := range nodes {
			toAdd := util.IsMapSubset(node.NetworkingInfo.Labels, globalLabels)
			if toAdd {
				filteredNodes = append(filteredNodes, node)
			}
		}
		return filteredNodes
	}

	// handle gvk level
	var gvkVsLabels map[schema.GroupVersionKind]map[string]string
	for _, resourceFilter := range resourceTreeFilter.ResourceFilters {
		gvk := resourceFilter.Gvk
		gvkVsLabels[schema.GroupVersionKind{
			Group:   gvk.Group,
			Version: gvk.Version,
			Kind:    gvk.Kind,
		}] = resourceFilter.ResourceIdentifier.Labels
	}

	for _, node := range nodes {
		nodeGvk := node.Manifest.GroupVersionKind()
		if val, ok := gvkVsLabels[nodeGvk]; ok {
			toAdd := util.IsMapSubset(node.NetworkingInfo.Labels, val)
			if toAdd {
				filteredNodes = append(filteredNodes, node)
			}
		}
	}

	return filteredNodes
}

func (impl *CommonHelmServiceImpl) buildPodMetadata(nodes []*bean.ResourceNode, restConfig *rest.Config) ([]*bean.PodMetadata, error) {
	deploymentPodHashMap, rolloutMap, uidVsExtraNodeInfoMap := getExtraNodeInfoMappings(nodes)
	podsMetadata := make([]*bean.PodMetadata, 0, len(nodes))
	for _, node := range nodes {

		if node.Kind != k8sCommonBean.PodKind {
			continue
		}
		// check if pod is new
		var isNew bool
		var err error
		if len(node.ParentRefs) > 0 {
			isNew, err = impl.isPodNew(nodes, node, deploymentPodHashMap, rolloutMap, uidVsExtraNodeInfoMap, restConfig)
			if err != nil {
				return podsMetadata, err
			}
		}

		// set containers,initContainers and ephemeral container names
		var containerNames []string
		var initContainerNames []string
		var ephemeralContainersInfo []bean.EphemeralContainerInfo
		var ephemeralContainerStatus []bean.EphemeralContainerStatusesInfo

		for _, nodeInfo := range node.Info {
			switch nodeInfo.Name {
			case bean.ContainersNamesType:
				containerNames = nodeInfo.Value.([]string)
			case bean.InitContainersNamesType:
				initContainerNames = nodeInfo.Value.([]string)
			case bean.EphemeralContainersInfoType:
				ephemeralContainersInfo = nodeInfo.Value.([]bean.EphemeralContainerInfo)
			case bean.EphemeralContainersStatusType:
				ephemeralContainerStatus = nodeInfo.Value.([]bean.EphemeralContainerStatusesInfo)
			default:
				continue
			}
		}

		ephemeralContainerStatusMap := make(map[string]bool)
		for _, c := range ephemeralContainerStatus {
			// c.state contains three states running,waiting and terminated
			// at any point of time only one state will be there
			if c.State.Running != nil {
				ephemeralContainerStatusMap[c.Name] = true
			}
		}
		ephemeralContainers := make([]*bean.EphemeralContainerData, 0, len(ephemeralContainersInfo))
		// sending only running ephemeral containers in the list
		for _, ec := range ephemeralContainersInfo {
			if _, ok := ephemeralContainerStatusMap[ec.Name]; ok {
				containerData := &bean.EphemeralContainerData{
					Name:       ec.Name,
					IsExternal: k8sObjectUtils.IsExternalEphemeralContainer(ec.Command, ec.Name),
				}
				ephemeralContainers = append(ephemeralContainers, containerData)
			}
		}

		podMetadata := &bean.PodMetadata{
			Name:                node.Name,
			UID:                 node.UID,
			Containers:          containerNames,
			InitContainers:      initContainerNames,
			EphemeralContainers: ephemeralContainers,
			IsNew:               isNew,
		}

		podsMetadata = append(podsMetadata, podMetadata)

	}
	return podsMetadata, nil
}

func (impl *CommonHelmServiceImpl) GetResourceTreeForExternalResources(req *client.ExternalResourceTreeRequest) (*bean.ResourceTreeResponse, error) {
	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(req.ClusterConfig)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		impl.logger.Errorw("error in getting restConfig", "err", err)
		return nil, err
	}

	manifests := impl.getManifestsForExternalResources(restConfig, req.ExternalResourceDetail)
	// build resource Nodes
	buildNodesRequest := NewBuildNodesRequest(NewBuildNodesConfig(restConfig)).
		WithDesiredOrLiveManifests(manifests...).
		WithBatchWorker(impl.helmReleaseConfig.BuildNodesBatchSize, impl.logger)
	buildNodesResponse, err := impl.BuildNodes(buildNodesRequest)
	if err != nil {
		impl.logger.Errorw("error in building Nodes", "err", err)
		return nil, err
	}

	// build pods metadata
	podsMetadata, err := impl.buildPodMetadata(buildNodesResponse.Nodes, restConfig)
	if err != nil {
		return nil, err
	}
	resourceTreeResponse := &bean.ResourceTreeResponse{
		ApplicationTree: &bean.ApplicationTree{
			Nodes: buildNodesResponse.Nodes,
		},
		PodMetadata: podsMetadata,
	}
	return resourceTreeResponse, nil
}

func (impl *CommonHelmServiceImpl) getManifestsForExternalResources(restConfig *rest.Config, externalResourceDetails []*client.ExternalResourceDetail) []*bean.DesiredOrLiveManifest {
	var manifests []*bean.DesiredOrLiveManifest
	for _, resource := range externalResourceDetails {
		gvk := &schema.GroupVersionKind{
			Group:   resource.GetGroup(),
			Version: resource.GetVersion(),
			Kind:    resource.GetKind(),
		}
		manifest, _, err := impl.k8sService.GetLiveManifest(restConfig, resource.GetNamespace(), gvk, resource.GetName())
		if err != nil {
			impl.logger.Errorw("Error in getting live manifest", "err", err)
			statusError, _ := err.(*errors2.StatusError)
			desiredManifest := &unstructured.Unstructured{}
			desiredManifest.SetGroupVersionKind(*gvk)
			desiredManifest.SetName(resource.Name)
			desiredManifest.SetNamespace(resource.Namespace)
			desiredOrLiveManifest := &bean.DesiredOrLiveManifest{
				Manifest: desiredManifest,
				// using deep copy as it replaces item in manifest in loop
				IsLiveManifestFetchError: true,
			}
			if statusError != nil {
				desiredOrLiveManifest.LiveManifestFetchErrorCode = statusError.Status().Code
			}
			manifests = append(manifests, desiredOrLiveManifest)
		} else {
			manifests = append(manifests, &bean.DesiredOrLiveManifest{
				Manifest: manifest,
			})
		}
	}
	return manifests
}

func (impl *CommonHelmServiceImpl) isPodNew(nodes []*bean.ResourceNode, node *bean.ResourceNode, deploymentPodHashMap map[string]string, rolloutMap map[string]*util.ExtraNodeInfo,
	uidVsExtraNodeInfoMap map[string]*util.ExtraNodeInfo, restConfig *rest.Config) (bool, error) {

	isNew := false
	parentRef := node.ParentRefs[0]
	parentKind := parentRef.Kind

	// if parent is StatefulSet - then pod label controller-revision-hash should match StatefulSet's update revision
	if parentKind == k8sCommonBean.StatefulSetKind && node.NetworkingInfo != nil {
		isNew = uidVsExtraNodeInfoMap[parentRef.UID].UpdateRevision == node.NetworkingInfo.Labels["controller-revision-hash"]
	}

	// if parent is Job - then pod label controller-revision-hash should match StatefulSet's update revision
	if parentKind == k8sCommonBean.JobKind {
		// TODO - new or old logic not built in orchestrator for Job's pods. hence not implementing here. as don't know the logic :)
		isNew = true
	}

	// if parent kind is replica set then
	if parentKind == k8sCommonBean.ReplicaSetKind {
		replicaSetNode := getMatchingNode(nodes, parentKind, parentRef.Name)

		// if parent of replicaset is deployment, compare label pod-template-hash
		if replicaSetParent := replicaSetNode.ParentRefs[0]; replicaSetNode != nil && len(replicaSetNode.ParentRefs) > 0 && replicaSetParent.Kind == k8sCommonBean.DeploymentKind {
			deploymentPodHash := deploymentPodHashMap[replicaSetParent.Name]
			replicaSetObj, err := impl.getReplicaSetObject(restConfig, replicaSetNode)
			if err != nil {
				return isNew, err
			}
			deploymentNode := getMatchingNode(nodes, replicaSetParent.Kind, replicaSetParent.Name)
			// TODO: why do we need deployment object for collisionCount ??
			var deploymentCollisionCount *int32
			if deploymentNode != nil && deploymentNode.DeploymentCollisionCount != nil {
				deploymentCollisionCount = deploymentNode.DeploymentCollisionCount
			} else {
				deploymentCollisionCount, err = impl.getDeploymentCollisionCount(restConfig, replicaSetParent)
				if err != nil {
					return isNew, err
				}
			}
			replicaSetPodHash := util.GetReplicaSetPodHash(replicaSetObj, deploymentCollisionCount)
			isNew = replicaSetPodHash == deploymentPodHash
		} else if replicaSetParent.Kind == k8sCommonBean.K8sClusterResourceRolloutKind {

			rolloutExtraInfo := rolloutMap[replicaSetParent.Name]
			rolloutPodHash := rolloutExtraInfo.RolloutCurrentPodHash
			replicasetPodHash := util.GetRolloutPodTemplateHash(replicaSetNode)

			isNew = rolloutPodHash == replicasetPodHash

		}

	}

	// if parent kind is DaemonSet then compare DaemonSet's Child ControllerRevision's label controller-revision-hash with pod label controller-revision-hash
	if parentKind == k8sCommonBean.DaemonSetKind {
		controllerRevisionNodes := getMatchingNodes(nodes, "ControllerRevision")
		for _, controllerRevisionNode := range controllerRevisionNodes {
			if len(controllerRevisionNode.ParentRefs) > 0 && controllerRevisionNode.ParentRefs[0].Kind == parentKind &&
				controllerRevisionNode.ParentRefs[0].Name == parentRef.Name && uidVsExtraNodeInfoMap[parentRef.UID].ResourceNetworkingInfo != nil &&
				node.NetworkingInfo != nil {

				isNew = uidVsExtraNodeInfoMap[parentRef.UID].ResourceNetworkingInfo.Labels["controller-revision-hash"] == node.NetworkingInfo.Labels["controller-revision-hash"]
			}
		}
	}
	return isNew, nil
}

func (impl *CommonHelmServiceImpl) getReplicaSetObject(restConfig *rest.Config, replicaSetNode *bean.ResourceNode) (*v1beta1.ReplicaSet, error) {
	gvk := &schema.GroupVersionKind{
		Group:   replicaSetNode.Group,
		Version: replicaSetNode.Version,
		Kind:    replicaSetNode.Kind,
	}
	var replicaSetNodeObj map[string]interface{}
	var err error
	if replicaSetNode.Manifest.Object == nil {
		replicaSetNodeManifest, _, err := impl.k8sService.GetLiveManifest(restConfig, replicaSetNode.Namespace, gvk, replicaSetNode.Name)
		if err != nil {
			impl.logger.Errorw("error in getting replicaSet live manifest", "clusterName", restConfig.ServerName, "replicaSetName", replicaSetNode.Name)
			return nil, err
		}
		if replicaSetNodeManifest != nil {
			replicaSetNodeObj = replicaSetNodeManifest.Object
		}
	} else {
		replicaSetNodeObj = replicaSetNode.Manifest.Object
	}

	replicaSetObj, err := util.ConvertToV1ReplicaSet(replicaSetNodeObj)
	if err != nil {
		impl.logger.Errorw("error in converting replicaSet unstructured object to replicaSet object", "clusterName", restConfig.ServerName, "replicaSetName", replicaSetNode.Name)
		return nil, err
	}
	return replicaSetObj, nil
}

func (impl *CommonHelmServiceImpl) getDeploymentCollisionCount(restConfig *rest.Config, deploymentInfo *bean.ResourceRef) (*int32, error) {
	parentGvk := &schema.GroupVersionKind{
		Group:   deploymentInfo.Group,
		Version: deploymentInfo.Version,
		Kind:    deploymentInfo.Kind,
	}
	var deploymentNodeObj map[string]interface{}
	var err error
	if deploymentInfo.Manifest.Object == nil {
		deploymentLiveManifest, _, err := impl.k8sService.GetLiveManifest(restConfig, deploymentInfo.Namespace, parentGvk, deploymentInfo.Name)
		if err != nil {
			impl.logger.Errorw("error in getting parent deployment live manifest", "clusterName", restConfig.ServerName, "deploymentName", deploymentInfo.Name)
			return nil, err
		}
		if deploymentLiveManifest != nil {
			deploymentNodeObj = deploymentLiveManifest.Object
		}
	} else {
		deploymentNodeObj = deploymentInfo.Manifest.Object
	}

	deploymentObj, err := util.ConvertToV1Deployment(deploymentNodeObj)
	if err != nil {
		impl.logger.Errorw("error in converting parent deployment unstructured object to replicaSet object", "clusterName", restConfig.ServerName, "deploymentName", deploymentInfo.Name)
		return nil, err
	}
	return deploymentObj.Status.CollisionCount, nil
}

func updateHookInfoForChildNodes(nodes []*bean.ResourceNode) {
	hookUidToHookTypeMap := make(map[string]string)
	for _, node := range nodes {
		if node.IsHook {
			hookUidToHookTypeMap[node.UID] = node.HookType
		}
	}
	// if node's parentRef is a hook then add hook info in child node also
	if len(hookUidToHookTypeMap) > 0 {
		for _, node := range nodes {
			if node.ParentRefs != nil && len(node.ParentRefs) > 0 {
				if hookType, ok := hookUidToHookTypeMap[node.ParentRefs[0].UID]; ok {
					node.IsHook = true
					node.HookType = hookType
				}
			}
		}
	}
}
func getExtraNodeInfoMappings(nodes []*bean.ResourceNode) (map[string]string, map[string]*util.ExtraNodeInfo, map[string]*util.ExtraNodeInfo) {
	deploymentPodHashMap := make(map[string]string)
	rolloutNameVsExtraNodeInfoMapping := make(map[string]*util.ExtraNodeInfo)
	uidVsExtraNodeInfoMapping := make(map[string]*util.ExtraNodeInfo)
	for _, node := range nodes {
		if node.Kind == k8sCommonBean.DeploymentKind {
			deploymentPodHashMap[node.Name] = node.DeploymentPodHash
		} else if node.Kind == k8sCommonBean.K8sClusterResourceRolloutKind {
			rolloutNameVsExtraNodeInfoMapping[node.Name] = &util.ExtraNodeInfo{
				RolloutCurrentPodHash: node.RolloutCurrentPodHash,
			}
		} else if node.Kind == k8sCommonBean.StatefulSetKind || node.Kind == k8sCommonBean.DaemonSetKind {
			if _, ok := uidVsExtraNodeInfoMapping[node.UID]; !ok {
				uidVsExtraNodeInfoMapping[node.UID] = &util.ExtraNodeInfo{UpdateRevision: node.UpdateRevision, ResourceNetworkingInfo: node.NetworkingInfo}
			}
		}
	}
	return deploymentPodHashMap, rolloutNameVsExtraNodeInfoMapping, uidVsExtraNodeInfoMapping
}
func getMatchingNode(nodes []*bean.ResourceNode, kind string, name string) *bean.ResourceNode {
	for _, node := range nodes {
		if node.Kind == kind && node.Name == name {
			return node
		}
	}
	return nil
}
func getMatchingNodes(nodes []*bean.ResourceNode, kind string) []*bean.ResourceNode {
	nodesRes := make([]*bean.ResourceNode, 0, len(nodes))
	for _, node := range nodes {
		if node.Kind == kind {
			nodesRes = append(nodesRes, node)
		}
	}
	return nodesRes
}
func BuildResourceRef(gvk schema.GroupVersionKind, manifest unstructured.Unstructured, namespace string) *bean.ResourceRef {
	resourceRef := &bean.ResourceRef{
		Group:     gvk.Group,
		Version:   gvk.Version,
		Kind:      gvk.Kind,
		Namespace: namespace,
		Name:      manifest.GetName(),
		UID:       string(manifest.GetUID()),
		Manifest:  manifest,
	}
	return resourceRef
}
