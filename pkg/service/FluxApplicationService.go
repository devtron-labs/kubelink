package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"fmt"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	k8sCommonBean "github.com/devtron-labs/common-lib/utils/k8s/commonBean"
	"github.com/devtron-labs/kubelink/converter"
	"github.com/devtron-labs/kubelink/fluxApplication/bean"
	client "github.com/devtron-labs/kubelink/grpc"
	clusterRepository "github.com/devtron-labs/kubelink/pkg/cluster"
	"go.uber.org/zap"
	"io"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd/api/latest"
	"strings"
)

type FluxApplicationService interface {
	ListApplications(clusterIds []int) ([]*bean.FluxApplicationListDto, error)
	GetFluxApplicationListForCluster(config *client.ClusterConfig) *client.FluxApplicationList
	GetAppDetail(resourceName, resourceNamespace string, clusterId int) (*bean.FluxApplicationListDto, error)
	//GetServerConfigIfClusterIsNotAddedOnDevtron(resourceResp *k8s.ManifestResponse, restConfig *rest.Config,
	//	clusterWithApplicationObject clusterRepository.Cluster, clusterServerUrlIdMap map[string]int) (*rest.Config, error)
	//GetClusterConfigFromAllClusters(clusterId int) (*k8s.ClusterConfig, clusterRepository.Cluster, map[string]int, error)
	//GetRestConfigForExternalArgo(ctx context.Context, clusterId int, externalFluxApplicationName string) (*rest.Config, error)
}

type FluxApplicationServiceImpl struct {
	logger            *zap.SugaredLogger
	clusterRepository clusterRepository.ClusterRepository
	k8sUtil           k8sUtils.K8sService
	helmAppService    HelmAppService
	converter         converter.ClusterBeanConverter
}

var (
	NilObjMetadata = bean.FluxResource{}
)

func NewFluxApplicationServiceImpl(logger *zap.SugaredLogger,
	clusterRepository clusterRepository.ClusterRepository,
	k8sUtil k8sUtils.K8sService,
	helmAppService HelmAppService, converter converter.ClusterBeanConverter) *FluxApplicationServiceImpl {
	return &FluxApplicationServiceImpl{
		logger:            logger,
		clusterRepository: clusterRepository,
		k8sUtil:           k8sUtil,
		helmAppService:    helmAppService,
		converter:         converter,
	}

}

func (impl *FluxApplicationServiceImpl) ListApplications(clusterIds []int) ([]*bean.FluxApplicationListDto, error) {
	var clusters []*clusterRepository.Cluster
	var err error
	if len(clusterIds) > 0 {
		// getting cluster details by ids
		clusters, err = impl.clusterRepository.FindByIds(clusterIds)
		if err != nil {
			impl.logger.Errorw("error in getting clusters by ids", "err", err, "clusterIds", clusterIds)
			return nil, err
		}
	} else {
		clusters, err = impl.clusterRepository.FindAllActive()
		if err != nil {
			impl.logger.Errorw("error in getting all active clusters", "err", err)
			return nil, err
		}
	}

	appListFinal := make([]*bean.FluxApplicationListDto, 0)
	for _, cluster := range clusters {
		clusterObj := cluster
		if clusterObj.IsVirtualCluster || len(clusterObj.ErrorInConnecting) != 0 {
			continue
		}

		restConfig := &rest.Config{}
		clusterInfo := impl.converter.GetClusterInfo(cluster)
		clusterConfig := impl.converter.GetClusterConfig(clusterInfo)
		restConfig, err = impl.k8sUtil.GetRestConfigByCluster(clusterConfig)
		if err != nil {
			impl.logger.Errorw("error in getting rest config ", "err", err, "clusterId", clusterObj.Id)
			return nil, err
		}
		restConfig2, err := impl.k8sUtil.GetRestConfigByCluster(clusterConfig)
		if err != nil {
			impl.logger.Errorw("error in getting rest config ", "err", err, "clusterId", clusterObj.Id)
			return nil, err
		}
		resp, err := impl.k8sUtil.GetResource(context.Background(), "flux-system", "flux-system", bean.GvkForKustomizationFluxApp, restConfig)
		fmt.Println(resp)
		kustomizationResp, _, err := impl.k8sUtil.GetResourceList(context.Background(), restConfig, bean.GvkForKustomizationFluxApp, bean.AllNamespaces, true, nil)
		if err == nil {
			kustomizationAppLists := getApplicationListDtos(kustomizationResp.Resources.Object, clusterObj.ClusterName, clusterObj.Id, "")
			appListFinal = append(appListFinal, kustomizationAppLists...)
		}

		//restConfig.Timeout = time.Duration(20)

		helmReleaseResp, _, err := impl.k8sUtil.GetResourceList(context.Background(), restConfig2, bean.GvkForHelmreleaseFluxApp, bean.AllNamespaces, true, nil)
		if err == nil {
			helmReleaseAppLists := getApplicationListDtos(helmReleaseResp.Resources.Object, clusterObj.ClusterName, clusterObj.Id, "HelmRelease")
			appListFinal = append(appListFinal, helmReleaseAppLists...)
		}

	}
	return appListFinal, nil
}

func (impl *FluxApplicationServiceImpl) GetFluxApplicationListForCluster(config *client.ClusterConfig) *client.FluxApplicationList {
	impl.logger.Debugw("Fetching application list ", "clusterId", config.ClusterId, "clusterName", config.ClusterName)
	//var fluxAppListFinal *client.FluxApplicationList
	fluxAppListFinal := new(client.FluxApplicationList)
	var fluxAppFinal []*client.FluxApplication
	appListFinal := make([]*bean.FluxApplicationListDto, 0)

	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(config)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		impl.logger.Errorw("Error in building rest config ", "clusterId", config.ClusterId, "err", err)
		return fluxAppListFinal
	}
	restConfig2, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		impl.logger.Errorw("error in getting rest config ", "err", err, "clusterId", config.ClusterId)
		return fluxAppListFinal
	}

	kustomizationResp, _, err := impl.k8sUtil.GetResourceList(context.Background(), restConfig, bean.GvkForKustomizationFluxApp, bean.AllNamespaces, true, nil)
	if err == nil {
		kustomizationAppLists := getApplicationListDtos(kustomizationResp.Resources.Object, config.ClusterName, int(config.ClusterId), "")
		appListFinal = append(appListFinal, kustomizationAppLists...)
	}

	helmReleaseResp, _, err := impl.k8sUtil.GetResourceList(context.Background(), restConfig2, bean.GvkForHelmreleaseFluxApp, bean.AllNamespaces, true, nil)
	if err == nil {
		helmReleaseAppLists := getApplicationListDtos(helmReleaseResp.Resources.Object, config.ClusterName, int(config.ClusterId), "HelmRelease")
		if len(helmReleaseAppLists) > 0 {
			appListFinal = append(appListFinal, helmReleaseAppLists...)
		}

	}

	for _, item := range appListFinal {

		fluxApp := &client.FluxApplication{
			Name:           item.Name,
			ClusterId:      int32(item.ClusterId),
			ClusterName:    item.ClusterName,
			Namespace:      item.Namespace,
			HealthStatus:   item.HealthStatus,
			SyncStatus:     item.SyncStatus,
			IsKustomizeApp: item.IsKustomizeApp,
		}
		fluxAppFinal = append(fluxAppFinal, fluxApp)
	}
	fluxAppListFinal.FluxApplication = fluxAppFinal
	return fluxAppListFinal
}
func getApplicationListDtos(manifestObj map[string]interface{}, clusterName string, clusterId int, FluxAppType string) []*bean.FluxApplicationListDto {
	appLists := make([]*bean.FluxApplicationListDto, 0)
	// map of keys and index in row cells, initially set as 0 will be updated by object
	keysToBeFetchedFromColumnDefinitions := map[string]int{k8sCommonBean.K8sResourceColumnDefinitionName: 0, "Ready": 0,
		"Status": 0}
	keysToBeFetchedFromRawObject := []string{k8sCommonBean.K8sClusterResourceNamespaceKey}

	columnsDataRaw := manifestObj[k8sCommonBean.K8sClusterResourceColumnDefinitionKey]
	if columnsDataRaw != nil {
		columnsData := columnsDataRaw.([]interface{})
		for i, columnData := range columnsData {
			columnDataMap := columnData.(map[string]interface{})
			for key := range keysToBeFetchedFromColumnDefinitions {
				if columnDataMap[k8sCommonBean.K8sClusterResourceNameKey] == key {
					keysToBeFetchedFromColumnDefinitions[key] = i
				}
			}
		}
	}

	rowsDataRaw := manifestObj[k8sCommonBean.K8sClusterResourceRowsKey]
	if rowsDataRaw != nil {
		rowsData := rowsDataRaw.([]interface{})
		for _, rowData := range rowsData {
			appListDto := &bean.FluxApplicationListDto{
				ClusterId:      clusterId,
				ClusterName:    clusterName,
				IsKustomizeApp: true,
			}
			rowDataMap := rowData.(map[string]interface{})
			rowObject := rowDataMap[k8sCommonBean.K8sClusterResourceObjectKey].(map[string]interface{})
			metadata := rowObject[k8sCommonBean.K8sClusterResourceMetadataKey].(map[string]interface{})

			if FluxAppType == "HelmRelease" {

				// Check for the existence and non-empty values of the required labels
				labels := metadata["labels"].(map[string]interface{})
				nameLabel, nameExists := labels["kustomize.toolkit.fluxcd.io/name"].(string)
				namespaceLabel, namespaceExists := labels["kustomize.toolkit.fluxcd.io/namespace"].(string)
				if nameExists && nameLabel != "" && namespaceExists && namespaceLabel != "" {
					continue
				}
				appListDto.IsKustomizeApp = false
			}

			rowCells := rowDataMap[k8sCommonBean.K8sClusterResourceCellKey].([]interface{})
			for key, value := range keysToBeFetchedFromColumnDefinitions {
				resolvedValueFromRowCell := rowCells[value].(string)
				switch key {
				case k8sCommonBean.K8sResourceColumnDefinitionName:
					appListDto.Name = resolvedValueFromRowCell
				case "Status":
					appListDto.SyncStatus = resolvedValueFromRowCell
				case "Ready":
					appListDto.HealthStatus = resolvedValueFromRowCell
				}
			}
			for _, key := range keysToBeFetchedFromRawObject {
				switch key {
				case k8sCommonBean.K8sClusterResourceNamespaceKey:
					appListDto.Namespace = metadata[k8sCommonBean.K8sClusterResourceNamespaceKey].(string)
				}
			}
			appLists = append(appLists, appListDto)
		}
	}
	return appLists
}

func (impl *FluxApplicationServiceImpl) GetAppDetail(AppDto bean.FluxApplicationListDto, config *client.ClusterConfig) error {

	appDetail := &bean.FluxpplicationDetailDto{
		FluxAppDto: &AppDto,
	}
	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(config)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		impl.logger.Errorw("error in getting rest config by cluster Id", "err", err, "clusterId", config.ClusterId)
		return err
	}

	if AppDto.IsKustomizeApp {
		resp, err := impl.k8sUtil.GetResource(context.Background(), AppDto.Namespace, AppDto.Name, bean.GvkForKustomizationFluxApp, restConfig)
		if err != nil {
			impl.logger.Errorw("error in getting resource list", "err", err)
			return err
		}
		if resp != nil && resp.Manifest.Object != nil {
			appDetail.Manifest = resp.Manifest.Object
			//fluxManagedResourceMap := getInventoryMap(resp.Manifest.Object)
			//
			//fluxKsSpecKubeConfig := getFluxKsSpecKubeConfig(resp.Manifest)
			//
			//AppDetailDto := &bean.FluxAppDto{
			//	Name:      AppDto.Name,
			//	Namespace: AppDto.Namespace,
			//}
			fluxKsApp := &bean.FluxKsAppDetail{
				Name:      AppDto.Name,
				Namespace: AppDto.Namespace,
				GroupKind: schema.GroupKind{
					Group: bean.FluxKustomizationGroup,
					Kind:  bean.FluxAppKustomizationKind,
				},
			}

			fluxKustomizationTree := bean.FluxKustomization{
				AppKsDetailDto: fluxKsApp,
				ParentKsApp:    "",
			}
			err1 := impl.getFluxManagedResourceTree(fluxKsApp, config, &fluxKustomizationTree)
			if err1 != nil {
				return err1
			}

			fmt.Println(fluxKustomizationTree)

		}

	}
	return err
}

func (impl *FluxApplicationServiceImpl) getFluxManagedResourceTree(fluxKsApp *bean.FluxKsAppDetail, config *client.ClusterConfig, fluxAppTree *bean.FluxKustomization) error {

	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(config)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		return err
	}
	resp, err := impl.k8sUtil.GetResource(context.Background(), fluxKsApp.Namespace, fluxKsApp.Name, bean.GvkForKustomizationFluxApp, restConfig)
	if err != nil {
		impl.logger.Errorw("error in getting resource list", "err", err)
		return err
	}
	if resp != nil && resp.Manifest.Object != nil {
		fluxManagedResourceMap := getInventoryMap(resp.Manifest.Object)

		fluxKsSpecKubeConfig := getFluxKsSpecKubeConfig(resp.Manifest.Object)

		fmt.Println(fluxManagedResourceMap, fluxKsSpecKubeConfig)

		fluxK8sResources := make([]*bean.FluxResource, 0)
		for version, id := range fluxManagedResourceMap {
			fluxResource, err := parseObjMetadata(id)
			if err != nil {
				fmt.Println("issue is here for some rreson , r", err)
			}
			fluxResource.Gvk.Version = version
			if fluxResource.Gvk.Group == fluxKsApp.GroupKind.Group && fluxResource.Gvk.Kind == fluxKsApp.GroupKind.Kind && fluxResource.Name ==                                                                                                            && fluxResource.Namespace == namespace {
				continue
			}

			if fluxResource.Gvk.Kind != bean.FluxAppHelmreleaseKind && fluxResource.Gvk.Kind != bean.FluxAppKustomizationKind {
				fluxK8sResources = append(fluxK8sResources, &fluxResource)
			}

			if fluxResource.Gvk.Group == bean.FluxHelmReleaseGroup &&
				fluxResource.Gvk.Kind == bean.FluxAppHelmreleaseKind {

				fluxK8sResourcesOfHelm := &bean.FluxHelmReleases{
					Name:        fluxResource.Name,
					Namespace:   fluxResource.Namespace,
					ParentKsApp: fluxAppTree.AppKsDetailDto.Name,
				}

				objects, err := impl.getHelmReleaseInventory(fluxK8sResourcesOfHelm.Name, fluxK8sResourcesOfHelm.Namespace, config)
				if err != nil {
					fmt.Println(err)
				}
				compact := true
				compactGroup := "toolkit.fluxcd.io"
				fluxHelmResources := make([]*bean.FluxHelmResource, 0)
				for _, obj := range objects {
					if compact && !strings.Contains(obj.Gvk.Group, compactGroup) {
						continue
					}
					fmt.Println("namespace", obj.Namespace, "Name", obj.Name, "Group", obj.Gvk.Group, "Kind", obj.Gvk.Kind)
					//ks.Add(object2.ObjMetadata(obj))
					fluxHelmResources = append(fluxHelmResources, &bean.FluxHelmResource{
						Gvk:       obj.Gvk,
						Name:      obj.Name,
						Namespace: obj.Namespace,
					})
				}
				fluxK8sResourcesOfHelm.Resources = fluxHelmResources

				fluxAppTree.FluxHelmReleases = append(fluxAppTree.FluxHelmReleases, fluxK8sResourcesOfHelm)

			}

			if fluxResource.Gvk.Group == bean.FluxKustomizationGroup &&
				fluxResource.Gvk.Kind == bean.FluxAppKustomizationKind &&
				// skip kustomization if it targets a remote clusters
				fluxKsSpecKubeConfig == false {

				//resp, err := impl.k8sUtil.GetResource(context.Background(), AppDto.Namespace, AppDto.Name, bean.GvkForKustomizationFluxApp, restConfig)
				//if err != nil {
				//	fmt.Println(err)
				//}
				//if resp != nil && resp.Manifest.Object != nil {
				//	appDetail.Manifest = resp.Manifest.Object
				//	fluxObjectMap := getInventoryMap(resp.Manifest.Object)
				//	fluxKsSpecKubeConfig = false
				//	appDetail.
				//	fluxChildKsTree := bean.FluxKustomization{
				//		AppDetailDto: ,
				//		Resources: bean.FluxK8sResource{
				//			ParentKsApp: AppDto.Name,
				//		},
				//	}
				//
				//}
				fluxKsAppChild := &bean.FluxKsAppDetail{
					Name:      fluxResource.Name,
					Namespace: fluxResource.Namespace,
					GroupKind: schema.GroupKind{
						Group: bean.FluxKustomizationGroup,
						Kind:  bean.FluxAppKustomizationKind,
					},
				}
				fluxChildTree := &bean.FluxKustomization{
					AppKsDetailDto: fluxKsAppChild,
					ParentKsApp:    fluxResource.Name,
				}
				err = impl.getFluxManagedResourceTree(fluxKsAppChild, config, fluxChildTree)
				fluxAppTree.Kustomizations = append(fluxAppTree.Kustomizations, fluxChildTree)

			}

		}
		fluxAppTree.Resources = fluxK8sResources
	}
	return nil
}

func getFluxKsSpecKubeConfig(obj map[string]interface{}) bool {

	kubeconfigSpec := false

	return kubeconfigSpec
}

func parseObjMetadata(s string) (bean.FluxResource, error) {
	index := strings.Index(s, bean.FieldSeparator)
	if index == -1 {
		return NilObjMetadata, fmt.Errorf("unable to parse stored object metadata: %s", s)
	}
	namespace := s[:index]
	s = s[index+1:]
	// Next, parse last field kind
	index = strings.LastIndex(s, bean.FieldSeparator)
	if index == -1 {
		return NilObjMetadata, fmt.Errorf("unable to parse stored object metadata: %s", s)
	}
	kind := s[index+1:]
	s = s[:index]
	// Next, parse next to last field group
	index = strings.LastIndex(s, bean.FieldSeparator)
	if index == -1 {
		return NilObjMetadata, fmt.Errorf("unable to parse stored object metadata: %s", s)
	}
	group := s[index+1:]
	// Finally, second field name. Name may contain colon transcoded as double underscore.
	name := s[:index]
	name = strings.ReplaceAll(name, bean.ColonTranscoded, ":")
	// Check that there are no extra fields by search for fieldSeparator.
	if strings.Contains(name, bean.FieldSeparator) {
		return NilObjMetadata, fmt.Errorf("too many fields within: %s", s)
	}
	// Create the ObjMetadata object from the four parsed fields.
	id := bean.FluxResource{
		Namespace: namespace,
		Name:      name,
		Gvk: schema.GroupVersionKind{
			Group: group,
			Kind:  kind,
		},
	}

	return id, nil
}

func getInventoryMap(obj map[string]interface{}) map[string]string {

	fluxManagedResourcesMap := make(map[string]string)

	if statusRawObj, ok := obj[bean.STATUS]; ok {
		statusObj := statusRawObj.(map[string]interface{})
		if inventoryRawObj, ok2 := statusObj[bean.INVENTORY]; ok2 {
			inventoryObj := inventoryRawObj.(map[string]interface{})
			if entriesRawObj, ok3 := inventoryObj[bean.ENTRIES]; ok3 {
				entriesObj := entriesRawObj.([]interface{})
				for _, itemRawObj := range entriesObj {
					itemObj := itemRawObj.(map[string]interface{})
					var matadataCompact bean.ObjectMetadataCompact
					if metadataRaw, ok4 := itemObj[bean.ID]; ok4 {
						metadata := metadataRaw.(string)
						matadataCompact.Id = metadata
					}
					if metadataVersionRaw, ok5 := itemObj[bean.VERSION]; ok5 {
						metadataVersion := metadataVersionRaw.(string)
						matadataCompact.Version = metadataVersion
					}
					fluxManagedResourcesMap[matadataCompact.Version] = matadataCompact.Id
				}
			}
		}
	}
	return fluxManagedResourcesMap

}

func (impl *FluxApplicationServiceImpl) getHelmReleaseInventory(name string, namespace string, config *client.ClusterConfig) ([]bean.FluxHelmResource, error) {

	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(config)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	resp, err := impl.k8sUtil.GetResource(context.Background(), namespace, name, bean.GvkForHelmreleaseFluxApp, restConfig)
	if err != nil {
		impl.logger.Errorw("error in getting resource list", "err", err)

	}

	//fmt.Println("spec-install-crds", hr.Spec.Install, "upgrade-data-crds", hr.Spec.Upgrade, "ns", hr.Namespace, "typemeta", hr.TypeMeta, "objectmeta", hr.ObjectMeta, "status", hr.Status)
	// skip release if it targets a remote clusters
	hrSpecKubeConfig := false
	if hrSpecKubeConfig  {
		return nil, nil
	}
	hrStatusStorageNamespace := "flux-system"

	storageNamespace := hrStatusStorageNamespace
	hrStatusHistoryLatest := "flux-system"

	latestNamespace := hrStatusHistoryLatest
	latest := "abcc"

	if len(storageNamespace) == 0 || latest == "" {
		// Skip release if it has no current
		return nil, nil
	}

	storageKey := client.ObjectKey{
		Namespace: storageNamespace,
		Name:      fmt.Sprintf("sh.helm.release.v1.%s.v%v", latest.Name, latest.Version),
	}

	// to be implement using the helm secret code part

	storageSecret := &corev1.Secret{}
	if err := kubeClient.Get(ctx, storageKey, storageSecret); err != nil {
		// skip release if it has no storage
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find the Helm storage object for HelmRelease '%s': %w", objectKey.String(), err)
	}

	releaseData, releaseFound := storageSecret.Data["release"]
	if !releaseFound {
		return nil, fmt.Errorf("failed to decode the Helm storage object for HelmRelease '%s'", objectKey.String())
	}

	// adapted from https://github.com/helm/helm/blob/02685e94bd3862afcb44f6cd7716dbeb69743567/pkg/storage/driver/util.go
	var b64 = base64.StdEncoding
	b, err := b64.DecodeString(string(releaseData))
	if err != nil {
		return nil, err
	}
	var magicGzip = []byte{0x1f, 0x8b, 0x08}
	if bytes.Equal(b[0:3], magicGzip) {
		r, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		b2, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		b = b2
	}

	// extract objects from Helm storage
	var rls hrStorage
	if err := json.Unmarshal(b, &rls); err != nil {
		return nil, fmt.Errorf("failed to decode the Helm storage object for HelmRelease '%s': %w", objectKey.String(), err)
	}

	objects, err := ssautil.ReadObjects(strings.NewReader(rls.Manifest))
	if err != nil {
		return nil, fmt.Errorf("failed to read the Helm storage object for HelmRelease '%s': %w", objectKey.String(), err)
	}

	// set the namespace on namespaced objects
	for _, obj := range objects {
		if obj.GetNamespace() == "" {
			if isNamespaced, _ := apiutil.IsObjectNamespaced(obj, kubeClient.Scheme(), kubeClient.RESTMapper()); isNamespaced {
				obj.SetNamespace(latest.Namespace)
			}
		}
	}

	result := object2.UnstructuredSetToObjMetadataSet(objects)
	//fmt.Println("resources of helmrelease", objectKey.Name, objectKey.Namespace)

	//for _, obj := range result {
	//	fmt.Println("kind", obj.GroupKind.Kind, "Group", obj.GroupKind.Group, "Namespace", obj.Namespace, "Name", obj.Name)
	//}
	//fmt.Println("resources end  of helmrelease", objectKey.Name, objectKey.Namespace)
	// search for CRDs managed by the HelmRelease if installing or upgrading CRDs is enabled in spec
	if (hr.Spec.Install != nil && len(hr.Spec.Install.CRDs) > 0 && hr.Spec.Install.CRDs != helmv2.Skip) ||
		(hr.Spec.Upgrade != nil && len(hr.Spec.Upgrade.CRDs) > 0 && hr.Spec.Upgrade.CRDs != helmv2.Skip) {
		selector := client.MatchingLabels{
			fmt.Sprintf("%s/name", helmv2.GroupVersion.Group):      hr.GetName(),
			fmt.Sprintf("%s/namespace", helmv2.GroupVersion.Group): hr.GetNamespace(),
		}
		//fmt.Println(hr.GetName(), hr.GetNamespace())
		crdKind := "CustomResourceDefinition"
		var list apiextensionsv1.CustomResourceDefinitionList
		if err := kubeClient.List(ctx, &list, selector); err == nil {
			for _, crd := range list.Items {
				found := false
				for _, r := range result {
					if r.Name == crd.GetName() && r.GroupKind.Kind == crdKind {
						//fmt.Println(r.Name)
						found = true
						break
					}
				}

				if !found {
					result = append(result, object2.ObjMetadata{
						Name: crd.GetName(),
						GroupKind: schema.GroupKind{
							Group: apiextensionsv1.GroupName,
							Kind:  crdKind,
						},
					})
				}
				fmt.Println(crd.Name, crd.Namespace)
			}
		}
	}
	for _, obj := range result {
		fmt.Println("kind", obj.GroupKind.Kind, "Group", obj.GroupKind.Group, "Namespace", obj.Namespace, "Name", obj.Name)
	}
	//fmt.Println(result)

	return result, nil
}
