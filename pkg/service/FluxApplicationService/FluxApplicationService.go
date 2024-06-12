package FluxApplicationService

import (
	"context"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	"github.com/devtron-labs/kubelink/converter"
	client "github.com/devtron-labs/kubelink/grpc"
	clusterRepository "github.com/devtron-labs/kubelink/pkg/cluster"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
)

type FluxApplicationService interface {
	GetFluxApplicationListForCluster(config *client.ClusterConfig) *client.FluxApplicationList
}

type FluxApplicationServiceImpl struct {
	logger            *zap.SugaredLogger
	clusterRepository clusterRepository.ClusterRepository
	k8sUtil           k8sUtils.K8sService
	converter         converter.ClusterBeanConverter
}

func NewFluxApplicationServiceImpl(logger *zap.SugaredLogger,
	clusterRepository clusterRepository.ClusterRepository,
	k8sUtil k8sUtils.K8sService,
	converter converter.ClusterBeanConverter) *FluxApplicationServiceImpl {
	return &FluxApplicationServiceImpl{
		logger:            logger,
		clusterRepository: clusterRepository,
		k8sUtil:           k8sUtil,
		converter:         converter,
	}

}

func (impl *FluxApplicationServiceImpl) GetFluxApplicationListForCluster(config *client.ClusterConfig) *client.FluxApplicationList {
	impl.logger.Debugw("Fetching application list ", "clusterId", config.ClusterId, "clusterName", config.ClusterName)

	appListFinal := make([]*FluxApplicationDto, 0)
	k8sClusterConfig := impl.converter.GetClusterConfigFromClientBean(config)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(k8sClusterConfig)
	if err != nil {
		impl.logger.Errorw("Error in building rest config ", "clusterId", config.ClusterId, "err", err)
		return &client.FluxApplicationList{}
	}
	/*	copying the restConfig for getting it reused again for the listing FluxCd HelmReleases
		After getting first call for kustomization list, same rest config cannot be used again due to wrap transport issue that changes the response header type.
	*/
	var restConfigForHrList rest.Config
	restConfigForHrList = *restConfig

	kustomizationResp, _, err := impl.k8sUtil.GetResourceList(context.Background(), restConfig, GvkForKustomizationFluxApp, AllNamespaces, true, nil)
	if err != nil {
		impl.logger.Errorw("Error in fetching kustomizationList Resource", "err", err)
		return &client.FluxApplicationList{}
	} else {
		if kustomizationResp != nil {
			kustomizationAppLists := getApplicationListDtos(kustomizationResp.Resources, config.ClusterName, int(config.ClusterId), "")
			if len(kustomizationAppLists) > 0 {
				appListFinal = append(appListFinal, kustomizationAppLists...)
			}
		}
	}

	helmReleaseResp, _, err := impl.k8sUtil.GetResourceList(context.Background(), &restConfigForHrList, GvkForHelmreleaseFluxApp, AllNamespaces, true, nil)
	if err != nil {
		impl.logger.Errorw("Error in fetching helmReleaseList Resources", "err", err)
		return &client.FluxApplicationList{}
	} else {
		if helmReleaseResp != nil {
			helmReleaseAppLists := getApplicationListDtos(helmReleaseResp.Resources, config.ClusterName, int(config.ClusterId), HelmReleaseFluxAppType)
			if len(helmReleaseAppLists) > 0 {
				appListFinal = append(appListFinal, helmReleaseAppLists...)
			}
		}
	}

	appListFinalDto := make([]*client.FluxApplicationDetail, 0)

	for _, appDetail := range appListFinal {
		fluxAppDetailDto := getFluxAppDetailDto(appDetail)
		appListFinalDto = append(appListFinalDto, fluxAppDetailDto)
	}
	finalAppListDto := &client.FluxApplicationList{
		ClusterId:             config.ClusterId,
		FluxApplicationDetail: appListFinalDto,
	}
	return finalAppListDto
}
