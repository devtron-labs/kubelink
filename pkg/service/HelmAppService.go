package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/argoproj/gitops-engine/pkg/health"
	"github.com/argoproj/gitops-engine/pkg/utils/kube"
	"github.com/devtron-labs/kubelink/bean"
	client "github.com/devtron-labs/kubelink/grpc"
	"github.com/devtron-labs/kubelink/pkg/helmClient"
	"github.com/devtron-labs/kubelink/pkg/util"
	gitops_engine "github.com/devtron-labs/kubelink/pkg/util/gitops-engine"
	k8sUtils "github.com/devtron-labs/kubelink/pkg/util/k8s"
	jsonpatch "github.com/evanphx/json-patch"
	"google.golang.org/protobuf/types/known/timestamppb"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"net/http"
	"strconv"
)

const (
	hibernateReplicaAnnotation = "hibernator.devtron.ai/replicas"
	hibernatePatch             = `[{"op": "replace", "path": "/spec/replicas", "value":%d}, {"op": "add", "path": "/metadata/annotations", "value": {"%s":"%s"}}]`
)

func BuildAppDetail(req *client.AppDetailRequest) (*bean.AppDetail, error) {

	helmRelease, err := getHelmRelease(req.ClusterConfig, req.Namespace, req.ReleaseName)
	if err != nil {
		return nil, err
	}

	resourceTreeResponse, err := buildResourceTree(req, helmRelease)
	if err != nil {
		return nil, err
	}

	appDetail := &bean.AppDetail{
		ResourceTreeResponse: resourceTreeResponse,
		ApplicationStatus:    buildAppHealthStatus(resourceTreeResponse.Nodes),
		LastDeployed:         helmRelease.Info.LastDeployed.Time,
		ChartMetadata: &bean.ChartMetadata{
			ChartName:    helmRelease.Chart.Name(),
			ChartVersion: helmRelease.Chart.Metadata.Version,
		},
		ReleaseStatus: &bean.ReleaseStatus{
			Status:      string(helmRelease.Info.Status),
			Description: helmRelease.Info.Description,
			Message:     getMessageFromReleaseStatus(helmRelease.Info.Status),
		},
		EnvironmentDetails: &client.EnvironmentDetails{
			ClusterName: req.ClusterConfig.ClusterName,
			ClusterId:   req.ClusterConfig.ClusterId,
			Namespace:   helmRelease.Namespace,
		},
	}

	return appDetail, nil
}

func GetHelmAppValues(req *client.AppDetailRequest) (*client.ReleaseInfo, error) {

	helmRelease, err := getHelmRelease(req.ClusterConfig, req.Namespace, req.ReleaseName)
	if err != nil {
		return nil, err
	}

	if helmRelease == nil {
		err = errors.New("release not found")
		return nil, err
	}

	releaseInfo, err := buildReleaseInfoBasicData(helmRelease)
	if err != nil {
		return nil, err
	}

	appDetail := &client.DeployedAppDetail{
		AppId:        GatAppId(req.ClusterConfig.ClusterId, helmRelease),
		AppName:      helmRelease.Name,
		ChartName:    helmRelease.Chart.Name(),
		ChartAvatar:  helmRelease.Chart.Metadata.Icon,
		LastDeployed: timestamppb.New(helmRelease.Info.LastDeployed.Time),
		ChartVersion: helmRelease.Chart.Metadata.Version,
		EnvironmentDetail: &client.EnvironmentDetails{
			ClusterName: req.ClusterConfig.ClusterName,
			ClusterId:   req.ClusterConfig.ClusterId,
			Namespace:   helmRelease.Namespace,
		},
	}
	releaseInfo.DeployedAppDetail = appDetail
	return releaseInfo, nil

}

func Hibernate(ctx context.Context, clusterConfig *client.ClusterConfig, requests []*client.ObjectIdentifier) (*client.HibernateResponse, error) {
	resp := &client.HibernateResponse{}
	conf, err := k8sUtils.GetRestConfig(clusterConfig)
	if err != nil {
		return resp, err
	}
	for _, request := range requests {
		status := &client.HibernateStatus{
			TargetObject: request,
			Success:      false,
			ErrorMsg:     "",
		}
		resp.Status = append(resp.Status, status)
		// get live manifest
		gvk := &schema.GroupVersionKind{
			Group:   request.Group,
			Kind:    request.Kind,
			Version: request.Version,
		}
		liveManifest, gvr, err := k8sUtils.GetLiveManifest(conf, request.Namespace, gvk, request.Name)
		if err != nil {
			status.Success = false
			status.ErrorMsg = err.Error()
			continue
		}
		if liveManifest == nil {

			status.Success = false
			status.ErrorMsg = "manifest not found"
			continue
		}
		replicas, found, err := unstructured.NestedInt64(liveManifest.UnstructuredContent(), "spec", "replicas")
		if err != nil {
			status.Success = false
			status.ErrorMsg = err.Error()
			continue
		}
		if !found {
			status.Success = false
			status.ErrorMsg = "replicas not found in manifest"
			continue
		}

		// patch resource
		patchRequest := &bean.KubernetesResourcePatchRequest{
			Name:                 request.Name,
			Namespace:            request.Namespace,
			GroupVersionResource: *gvr,
			Patch:                fmt.Sprintf(hibernatePatch, 0, hibernateReplicaAnnotation, strconv.Itoa(int(replicas))),
			PatchType:            string(types.JSONPatchType),
		}
		err = k8sUtils.PatchResource(context.Background(), conf, patchRequest)
		if err != nil {
			status.Success = false
			status.ErrorMsg = "replicas not found in manifest"
			continue
		}
		status.Success = true
	}
	return resp, nil
}

func UnHibernate(ctx context.Context, clusterConfig *client.ClusterConfig, requests []*client.ObjectIdentifier) (*client.HibernateResponse, error) {
	resp := &client.HibernateResponse{}

	conf, err := k8sUtils.GetRestConfig(clusterConfig)
	if err != nil {
		return resp, err
	}
	for _, request := range requests {
		status := &client.HibernateStatus{
			TargetObject: request,
			Success:      false,
			ErrorMsg:     "",
		}
		resp.Status = append(resp.Status, status)
		// get live manifest
		gvk := &schema.GroupVersionKind{
			Group:   request.Group,
			Kind:    request.Kind,
			Version: request.Version,
		}
		liveManifest, gvr, err := k8sUtils.GetLiveManifest(conf, request.Namespace, gvk, request.Name)
		if err != nil {
			status.Success = false
			status.ErrorMsg = err.Error()
			continue
		}

		if liveManifest == nil {
			status.Success = false
			status.ErrorMsg = "manifest not found"
			continue
		}

		originalReplicaCount, err := strconv.Atoi(liveManifest.GetAnnotations()[hibernateReplicaAnnotation])
		if err != nil {
			status.Success = false
			status.ErrorMsg = err.Error()
			continue
		}

		// patch resource
		patchRequest := &bean.KubernetesResourcePatchRequest{
			Name:                 request.Name,
			Namespace:            request.Namespace,
			GroupVersionResource: *gvr,
			Patch:                fmt.Sprintf(hibernatePatch, originalReplicaCount, hibernateReplicaAnnotation, "0"),
			PatchType:            string(types.JSONPatchType),
		}
		err = k8sUtils.PatchResource(context.Background(), conf, patchRequest)
		if err != nil {
			status.Success = false
			status.ErrorMsg = err.Error()
			continue
		}
		status.Success = true
	}
	return resp, nil
}

func GetDeploymentHistory(req *client.AppDetailRequest) (*client.HelmAppDeploymentHistory, error) {
	helmReleases, err := getHelmReleaseHistory(req.ClusterConfig, req.Namespace, req.ReleaseName)
	if err != nil {
		return nil, err
	}
	var helmAppDeployments []*client.HelmAppDeploymentDetail
	for _, helmRelease := range helmReleases {
		chartMetadata := helmRelease.Chart.Metadata
		manifests := helmRelease.Manifest
		parsedManifests, err := util.SplitYAMLs([]byte(manifests))
		if err != nil {
			return nil, err
		}
		dockerImages, err := util.ExtractAllDockerImages(parsedManifests)
		if err != nil {
			return nil, err
		}
		deploymentDetail := &client.HelmAppDeploymentDetail{
			DeployedAt: timestamppb.New(helmRelease.Info.LastDeployed.Time),
			ChartMetadata: &client.ChartMetadata{
				ChartName:    chartMetadata.Name,
				ChartVersion: chartMetadata.Version,
				Home:         chartMetadata.Home,
				Sources:      chartMetadata.Sources,
				Description:  chartMetadata.Description,
			},
			DockerImages: dockerImages,
			Version:      int32(helmRelease.Version),
		}
		helmAppDeployments = append(helmAppDeployments, deploymentDetail)
	}
	return &client.HelmAppDeploymentHistory{DeploymentHistory: helmAppDeployments}, nil
}

func GetDesiredManifest(req *client.ObjectRequest) (*client.DesiredManifestResponse, error) {
	objectIdentifier := req.ObjectIdentifier
	helmRelease, err := getHelmRelease(req.ClusterConfig, req.ReleaseNamespace, req.ReleaseName)
	if err != nil {
		return nil, err
	}

	manifests, err := util.SplitYAMLs([]byte(helmRelease.Manifest))
	if err != nil {
		return nil, err
	}

	desiredManifest := ""
	for _, manifest := range manifests {
		gvk := manifest.GroupVersionKind()
		if gvk.Group == objectIdentifier.Group && gvk.Version == objectIdentifier.Version && gvk.Kind == objectIdentifier.Kind && manifest.GetName() == objectIdentifier.Name {
			dataByteArr, err := json.Marshal(manifest.UnstructuredContent())
			if err != nil {
				return nil, err
			}
			desiredManifest = string(dataByteArr)
			break
		}
	}

	desiredManifestResponse := &client.DesiredManifestResponse{
		Manifest: desiredManifest,
	}

	return desiredManifestResponse, nil
}

func UninstallRelease(releaseIdentifier *client.ReleaseIdentifier) (*client.UninstallReleaseResponse, error) {
	conf, err := k8sUtils.GetRestConfig(releaseIdentifier.ClusterConfig)
	if err != nil {
		return nil, err
	}
	opt := &helmClient.RestConfClientOptions{
		Options: &helmClient.Options{
			Namespace: releaseIdentifier.ReleaseNamespace,
		},
		RestConfig: conf,
	}

	helmClient, err := helmClient.NewClientFromRestConf(opt)
	if err != nil {
		return nil, err
	}

	err = helmClient.UninstallReleaseByName(releaseIdentifier.ReleaseName)
	if err != nil {
		return nil, err
	}

	uninstallReleaseResponse := &client.UninstallReleaseResponse{
		Success: true,
	}

	return uninstallReleaseResponse, nil
}

func UpgradeRelease(ctx context.Context, request *client.UpgradeReleaseRequest) (*client.UpgradeReleaseResponse, error) {
	releaseIdentifier := request.ReleaseIdentifier
	conf, err := k8sUtils.GetRestConfig(releaseIdentifier.ClusterConfig)
	if err != nil {
		return nil, err
	}
	opt := &helmClient.RestConfClientOptions{
		Options: &helmClient.Options{
			Namespace: releaseIdentifier.ReleaseNamespace,
		},
		RestConfig: conf,
	}

	helmClientObj, err := helmClient.NewClientFromRestConf(opt)
	if err != nil {
		return nil, err
	}

	helmRelease, err := getHelmRelease(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace, releaseIdentifier.ReleaseName)
	if err != nil {
		return nil, err
	}

	updateChartSpec := &helmClient.ChartSpec{
		ReleaseName: releaseIdentifier.ReleaseName,
		Namespace:   releaseIdentifier.ReleaseNamespace,
		ValuesYaml:  request.ValuesYaml,
	}

	_, err = helmClientObj.UpgradeRelease(context.Background(), helmRelease.Chart, updateChartSpec)
	if err != nil {
		return nil, err
	}

	upgradeReleaseResponse := &client.UpgradeReleaseResponse{
		Success: true,
	}

	return upgradeReleaseResponse, nil
}

func GetDeploymentDetail(request *client.DeploymentDetailRequest) (*client.DeploymentDetailResponse, error) {
	releaseIdentifier := request.ReleaseIdentifier
	helmReleases, err := getHelmReleaseHistory(releaseIdentifier.ClusterConfig, releaseIdentifier.ReleaseNamespace, releaseIdentifier.ReleaseName)
	if err != nil {
		return nil, err
	}

	resp := &client.DeploymentDetailResponse{}
	for _, helmRelease := range helmReleases {
		if request.DeploymentVersion == int32(helmRelease.Version) {
			releaseInfo, err := buildReleaseInfoBasicData(helmRelease)
			if err != nil {
				return nil, err
			}
			resp.Manifest = helmRelease.Manifest
			resp.ValuesYaml = releaseInfo.MergedValues
			break
		}
	}

	return resp, nil
}

func InstallRelease(ctx context.Context, request *client.InstallReleaseRequest) (*client.InstallReleaseResponse, error) {
	releaseIdentifier := request.ReleaseIdentifier
	conf, err := k8sUtils.GetRestConfig(releaseIdentifier.ClusterConfig)
	if err != nil {
		return nil, err
	}
	opt := &helmClient.RestConfClientOptions{
		Options: &helmClient.Options{
			Namespace: releaseIdentifier.ReleaseNamespace,
		},
		RestConfig: conf,
	}

	helmClientObj, err := helmClient.NewClientFromRestConf(opt)
	if err != nil {
		return nil, err
	}

	// Add or update chart repo starts
	chartRepoRequest := request.ChartRepository
	chartRepoName := chartRepoRequest.Name
	chartRepo := repo.Entry{
		Name:     chartRepoName,
		URL:      chartRepoRequest.Url,
		Username: chartRepoRequest.Username,
		Password: chartRepoRequest.Password,
		// Since helm 3.6.1 it is necessary to pass 'PassCredentialsAll = true'.
		PassCredentialsAll:    true,
		InsecureSkipTLSverify: true,
	}

	err = helmClientObj.AddOrUpdateChartRepo(chartRepo)
	if err != nil {
		return nil, err
	}
	// Add or update chart repo ends

	// Install release starts
	chartSpec := &helmClient.ChartSpec{
		ReleaseName:      releaseIdentifier.ReleaseName,
		Namespace:        releaseIdentifier.ReleaseNamespace,
		ValuesYaml:       request.ValuesYaml,
		ChartName:        fmt.Sprintf("%s/%s", chartRepoName, request.ChartName),
		Version:          request.ChartVersion,
		DependencyUpdate: true,
		UpgradeCRDs:      true,
		CreateNamespace:  true,
	}
	_, err = helmClientObj.InstallChart(context.Background(), chartSpec)
	if err != nil {
		return nil, err
	}
	// Install release ends

	installReleaseResponse := &client.InstallReleaseResponse{
		Success: true,
	}

	return installReleaseResponse, nil

}

func UpgradeReleaseWithChartInfo(ctx context.Context, request *client.InstallReleaseRequest) (*client.UpgradeReleaseResponse, error) {
	releaseIdentifier := request.ReleaseIdentifier
	conf, err := k8sUtils.GetRestConfig(releaseIdentifier.ClusterConfig)
	if err != nil {
		return nil, err
	}
	opt := &helmClient.RestConfClientOptions{
		Options: &helmClient.Options{
			Namespace: releaseIdentifier.ReleaseNamespace,
		},
		RestConfig: conf,
	}

	helmClientObj, err := helmClient.NewClientFromRestConf(opt)
	if err != nil {
		return nil, err
	}

	// Add or update chart repo starts
	chartRepoRequest := request.ChartRepository
	chartRepoName := chartRepoRequest.Name
	chartRepo := repo.Entry{
		Name:     chartRepoName,
		URL:      chartRepoRequest.Url,
		Username: chartRepoRequest.Username,
		Password: chartRepoRequest.Password,
		// Since helm 3.6.1 it is necessary to pass 'PassCredentialsAll = true'.
		PassCredentialsAll:    true,
		InsecureSkipTLSverify: true,
	}

	err = helmClientObj.AddOrUpdateChartRepo(chartRepo)
	if err != nil {
		return nil, err
	}
	// Add or update chart repo ends

	// Update release starts
	chartSpec := &helmClient.ChartSpec{
		ReleaseName:      releaseIdentifier.ReleaseName,
		Namespace:        releaseIdentifier.ReleaseNamespace,
		ValuesYaml:       request.ValuesYaml,
		ChartName:        fmt.Sprintf("%s/%s", chartRepoName, request.ChartName),
		Version:          request.ChartVersion,
		DependencyUpdate: true,
		UpgradeCRDs:      true,
	}
	_, err = helmClientObj.UpgradeReleaseWithChartInfo(context.Background(), chartSpec)
	if err != nil {
		return nil, err
	}
	// Update release ends

	upgradeReleaseResponse := &client.UpgradeReleaseResponse{
		Success: true,
	}

	return upgradeReleaseResponse, nil

}

func getHelmRelease(clusterConfig *client.ClusterConfig, namespace string, releaseName string) (*release.Release, error) {
	conf, err := k8sUtils.GetRestConfig(clusterConfig)
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

func getHelmReleaseHistory(clusterConfig *client.ClusterConfig, releaseNamespace string, releaseName string) ([]*release.Release, error) {
	conf, err := k8sUtils.GetRestConfig(clusterConfig)
	if err != nil {
		return nil, err
	}
	opt := &helmClient.RestConfClientOptions{
		Options: &helmClient.Options{
			Namespace: releaseNamespace,
		},
		RestConfig: conf,
	}

	helmClient, err := helmClient.NewClientFromRestConf(opt)
	if err != nil {
		return nil, err
	}

	releases, err := helmClient.ListReleaseHistory(releaseName, 20)
	if err != nil {
		return nil, err
	}

	return releases, nil
}

func buildReleaseInfoBasicData(helmRelease *release.Release) (*client.ReleaseInfo, error) {
	defaultValues := helmRelease.Chart.Values
	overrideValues := helmRelease.Config
	var mergedValues map[string]interface{}
	if overrideValues == nil {
		mergedValues = defaultValues
	} else {
		defaultValuesByteArr, err := json.Marshal(defaultValues)
		if err != nil {
			return nil, err
		}
		overrideValuesByteArr, err := json.Marshal(overrideValues)
		if err != nil {
			return nil, err
		}
		mergedValuesByteArr, err := jsonpatch.MergePatch(defaultValuesByteArr, overrideValuesByteArr)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(mergedValuesByteArr, &mergedValues)
		if err != nil {
			return nil, err
		}
	}
	defaultValString, err := json.Marshal(defaultValues)
	if err != nil {
		return nil, err
	}
	overrideValuesString, err := json.Marshal(overrideValues)
	if err != nil {
		return nil, err
	}
	mergedValuesString, err := json.Marshal(mergedValues)
	if err != nil {
		return nil, err
	}
	var readme string
	for _, file := range helmRelease.Chart.Files {
		if file.Name == "README.md" {
			readme = string(file.Data)
			continue
		}
	}

	res := &client.ReleaseInfo{
		DefaultValues:  string(defaultValString),
		OverrideValues: string(overrideValuesString),
		MergedValues:   string(mergedValuesString),
		Readme:         readme,
	}

	return res, nil
}

func buildResourceTree(appDetailRequest *client.AppDetailRequest, release *release.Release) (*bean.ResourceTreeResponse, error) {
	conf, err := k8sUtils.GetRestConfig(appDetailRequest.ClusterConfig)
	if err != nil {
		return nil, err
	}
	manifests, err := util.SplitYAMLs([]byte(release.Manifest))
	if err != nil {
		return nil, err
	}
	// get live manifests from kubernetes
	desiredOrLiveManifests, err := getDesiredOrLiveManifests(conf, manifests, appDetailRequest.Namespace)
	if err != nil {
		return nil, err
	}
	// build resource nodes
	nodes, err := buildNodes(conf, desiredOrLiveManifests, appDetailRequest.Namespace, nil)
	if err != nil {
		return nil, err
	}
	// build pods metadata
	podsMetadata, err := buildPodMetadata(nodes)
	if err != nil {
		return nil, err
	}
	resourceTreeResponse := &bean.ResourceTreeResponse{
		ApplicationTree: &bean.ApplicationTree{
			Nodes: nodes,
		},
		PodMetadata: podsMetadata,
	}
	return resourceTreeResponse, nil
}

func getDesiredOrLiveManifests(restConfig *rest.Config, desiredManifests []unstructured.Unstructured, releaseNamespace string) ([]*bean.DesiredOrLiveManifest, error) {

	var desiredOrLiveManifests []*bean.DesiredOrLiveManifest
	for _, desiredManifest := range desiredManifests {
		gvk := desiredManifest.GroupVersionKind()

		_namespace := desiredManifest.GetNamespace()
		if _namespace == "" {
			_namespace = releaseNamespace
		}

		liveManifest, _, err := k8sUtils.GetLiveManifest(restConfig, _namespace, &gvk, desiredManifest.GetName())
		desiredOrLiveManifest := &bean.DesiredOrLiveManifest{}

		if err != nil {
			statusError, _ := err.(*errors2.StatusError)
			desiredOrLiveManifest = &bean.DesiredOrLiveManifest{
				// using deep copy as it replaces item in manifest in loop
				Manifest:                   desiredManifest.DeepCopy(),
				IsLiveManifestFetchError:   true,
				LiveManifestFetchErrorCode: statusError.Status().Code,
			}
		} else {
			desiredOrLiveManifest = &bean.DesiredOrLiveManifest{
				Manifest: liveManifest,
			}
		}
		desiredOrLiveManifests = append(desiredOrLiveManifests, desiredOrLiveManifest)
	}

	return desiredOrLiveManifests, nil
}

func buildNodes(restConfig *rest.Config, desiredOrLiveManifests []*bean.DesiredOrLiveManifest, releaseNamespace string, parentResourceRef *bean.ResourceRef) ([]*bean.ResourceNode, error) {
	var nodes []*bean.ResourceNode
	for _, desiredOrLiveManifest := range desiredOrLiveManifests {
		manifest := desiredOrLiveManifest.Manifest
		gvk := manifest.GroupVersionKind()

		_namespace := manifest.GetNamespace()
		if _namespace == "" {
			_namespace = releaseNamespace
		}

		resourceRef := buildResourceRef(gvk, *manifest, _namespace)

		if k8sUtils.CanHaveChild(gvk) {
			children, err := k8sUtils.GetChildObjects(restConfig, _namespace, gvk, manifest.GetName(), manifest.GetAPIVersion())
			if err != nil {
				return nil, err
			}
			var desiredOrLiveManifestsChildren []*bean.DesiredOrLiveManifest
			for _, child := range children {
				desiredOrLiveManifestsChildren = append(desiredOrLiveManifestsChildren, &bean.DesiredOrLiveManifest{
					Manifest: child,
				})
			}
			childNodes, err := buildNodes(restConfig, desiredOrLiveManifestsChildren, releaseNamespace, resourceRef)
			if err != nil {
				return nil, err
			}

			for _, childNode := range childNodes {
				nodes = append(nodes, childNode)
			}
		}

		node := &bean.ResourceNode{
			ResourceRef:     resourceRef,
			ResourceVersion: manifest.GetResourceVersion(),
			NetworkingInfo: &bean.ResourceNetworkingInfo{
				Labels: manifest.GetLabels(),
			},
		}

		if parentResourceRef != nil {
			node.ParentRefs = append(make([]*bean.ResourceRef, 0), parentResourceRef)
		}

		// set health of node
		if desiredOrLiveManifest.IsLiveManifestFetchError {
			if desiredOrLiveManifest.LiveManifestFetchErrorCode == http.StatusNotFound {
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
			if healthCheck := gitops_engine.GetHealthCheckFunc(gvk); healthCheck != nil {
				health, err := healthCheck(manifest)
				if err != nil {
					node.Health = &bean.HealthStatus{
						Status:  bean.HealthStatusUnknown,
						Message: err.Error(),
					}
				} else if health != nil {
					node.Health = &bean.HealthStatus{
						Status:  string(health.Status),
						Message: health.Message,
					}
				}
			}
		}

		// hibernate set starts
		if parentResourceRef == nil {
			annotations := node.Manifest.GetAnnotations()
			if annotations != nil {
				if _, ok := annotations[hibernateReplicaAnnotation]; ok {
					node.IsHibernated = true
				}
			}
			if !node.IsHibernated {
				_, found, _ := unstructured.NestedInt64(node.Manifest.UnstructuredContent(), "spec", "replicas")
				if found {
					node.CanBeHibernated = true
				}
			}
		}
		// hibernate set ends

		nodes = append(nodes, node)
	}

	return nodes, nil
}

func buildResourceRef(gvk schema.GroupVersionKind, manifest unstructured.Unstructured, namespace string) *bean.ResourceRef {
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

func buildPodMetadata(nodes []*bean.ResourceNode) ([]*bean.PodMetadata, error) {
	var podsMetadata []*bean.PodMetadata
	for _, node := range nodes {
		if node.Kind != kube.PodKind {
			continue
		}

		var pod coreV1.Pod
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(node.Manifest.UnstructuredContent(), &pod)
		if err != nil {
			return nil, err
		}

		// check if pod is new
		isNew := true
		if len(node.ParentRefs) > 0 {
			parentRef := node.ParentRefs[0]
			parentKind := parentRef.Kind

			// if parent is StatefulSet - then pod label controller-revision-hash should match StatefulSet's update revision
			if parentKind == kube.StatefulSetKind {
				var statefulSet appsV1.StatefulSet
				err := runtime.DefaultUnstructuredConverter.FromUnstructured(parentRef.Manifest.UnstructuredContent(), &statefulSet)
				if err != nil {
					return nil, err
				}
				isNew = statefulSet.Status.UpdateRevision == pod.GetLabels()["controller-revision-hash"]
			}

			// if parent is Job - then pod label controller-revision-hash should match StatefulSet's update revision
			if parentKind == kube.JobKind {
				//TODO - new or old logic not built in orchestrator for Job's pods. hence not implementing here. as don't know the logic :)
				isNew = true
			}

			// if parent kind is replica set then
			if parentKind == kube.ReplicaSetKind {
				var replicaSet appsV1.ReplicaSet
				err := runtime.DefaultUnstructuredConverter.FromUnstructured(parentRef.Manifest.UnstructuredContent(), &replicaSet)
				if err != nil {
					return nil, err
				}
				replicaSetNode := getMatchingNode(nodes, parentKind, replicaSet.Name)

				// if parent of replicaset is deployment, compare label pod-template-hash
				if replicaSetNode != nil && len(replicaSetNode.ParentRefs) > 0 && replicaSetNode.ParentRefs[0].Kind == kube.DeploymentKind {
					isNew = replicaSet.GetLabels()["pod-template-hash"] == pod.GetLabels()["pod-template-hash"]
				}
			}

			// if parent kind is DaemonSet then compare DaemonSet's Child ControllerRevision's label controller-revision-hash with pod label controller-revision-hash
			if parentKind == kube.DaemonSetKind {
				var daemonSet appsV1.DaemonSet
				err := runtime.DefaultUnstructuredConverter.FromUnstructured(parentRef.Manifest.UnstructuredContent(), &daemonSet)
				if err != nil {
					return nil, err
				}

				controllerRevisionNodes := getMatchingNodes(nodes, "ControllerRevision")
				for _, controllerRevisionNode := range controllerRevisionNodes {
					if len(controllerRevisionNode.ParentRefs) > 0 && controllerRevisionNode.ParentRefs[0].Kind == parentKind &&
						controllerRevisionNode.ParentRefs[0].Name == daemonSet.Name {

						var controlRevision appsV1.ControllerRevision
						err := runtime.DefaultUnstructuredConverter.FromUnstructured(parentRef.Manifest.UnstructuredContent(), &controlRevision)
						if err != nil {
							return nil, err
						}
						isNew = controlRevision.GetLabels()["controller-revision-hash"] == pod.GetLabels()["controller-revision-hash"]
					}
				}

			}

		}

		// set containers and initContainers names
		var containerNames []string
		var initContainerNames []string
		for _, container := range pod.Spec.Containers {
			containerNames = append(containerNames, container.Name)
		}
		for _, initContainer := range pod.Spec.InitContainers {
			initContainerNames = append(initContainerNames, initContainer.Name)
		}

		podMetadata := &bean.PodMetadata{
			Name:           node.Name,
			UID:            node.UID,
			Containers:     containerNames,
			InitContainers: initContainerNames,
			IsNew:          isNew,
		}

		podsMetadata = append(podsMetadata, podMetadata)

	}
	return podsMetadata, nil
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
	var nodesRes []*bean.ResourceNode
	for _, node := range nodes {
		if node.Kind == kind {
			nodesRes = append(nodesRes, node)
		}
	}
	return nodesRes
}

// app health is worst of the nodes health
func buildAppHealthStatus(nodes []*bean.ResourceNode) *bean.HealthStatusCode {
	appHealthStatus := bean.HealthStatusHealthy

	for _, node := range nodes {
		nodeHealth := node.Health
		if nodeHealth == nil {
			continue
		}
		if health.IsWorse(health.HealthStatusCode(appHealthStatus), health.HealthStatusCode(nodeHealth.Status)) {
			appHealthStatus = nodeHealth.Status
		}
	}

	return &appHealthStatus
}

func getMessageFromReleaseStatus(releaseStatus release.Status) string {
	switch releaseStatus {
	case release.StatusUnknown:
		return "The release is in an uncertain state"
	case release.StatusDeployed:
		return "The release has been pushed to Kubernetes"
	case release.StatusUninstalled:
		return "The release has been uninstalled from Kubernetes"
	case release.StatusSuperseded:
		return "The release object is outdated and a newer one exists"
	case release.StatusFailed:
		return "The release was not successfully deployed"
	case release.StatusUninstalling:
		return "The release uninstall operation is underway"
	case release.StatusPendingInstall:
		return "The release install operation is underway"
	case release.StatusPendingUpgrade:
		return "The release upgrade operation is underway"
	case release.StatusPendingRollback:
		return "The release rollback operation is underway"
	default:
		fmt.Println("un handled release status", releaseStatus)
	}

	return ""
}
