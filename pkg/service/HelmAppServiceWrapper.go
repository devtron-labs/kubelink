package service

import (
	"context"
	"fmt"
	"github.com/devtron-labs/kubelink/bean"
	"github.com/devtron-labs/kubelink/grpc"
	"github.com/devtron-labs/kubelink/internal/lock"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ApplicationServiceServerImpl struct {
	client.UnimplementedApplicationServiceServer
	Logger                *zap.SugaredLogger
	ChartRepositoryLocker *lock.ChartRepositoryLocker
	HelmAppService        HelmAppService
}

func (impl *ApplicationServiceServerImpl) MustEmbedUnimplementedApplicationServiceServer() {
	panic("implement me")
}

func NewApplicationServiceServerImpl(logger *zap.SugaredLogger, chartRepositoryLocker *lock.ChartRepositoryLocker,
	HelmAppService HelmAppService) *ApplicationServiceServerImpl {
	return &ApplicationServiceServerImpl{
		Logger:                logger,
		ChartRepositoryLocker: chartRepositoryLocker,
		HelmAppService:        HelmAppService,
	}
}

func (impl *ApplicationServiceServerImpl) ListApplications(req *client.AppListRequest, res client.ApplicationService_ListApplicationsServer) error {
	impl.Logger.Infow("app list req")
	clusterConfigs := req.GetClusters()
	eg := new(errgroup.Group)
	for _, config := range clusterConfigs {
		clusterConfig := *config
		eg.Go(func() error {
			apps := impl.HelmAppService.GetApplicationListForCluster(&clusterConfig)
			fmt.Println(apps.String())
			err := res.Send(apps)
			fmt.Println(err)
			return err
		})
	}
	if err := eg.Wait(); err != nil {
		fmt.Println(err)
		return err
	}
	impl.Logger.Infow("list app done")
	return nil
}

func (impl *ApplicationServiceServerImpl) GetAppDetail(ctxt context.Context, req *client.AppDetailRequest) (*client.AppDetail, error) {
	impl.Logger.Infow("get app detail", "release", req.ReleaseName, "ns", req.Namespace)
	helmAppDetail, err := impl.HelmAppService.BuildAppDetail(req)
	if err != nil {
		return nil, err
	}
	res := impl.AppDetailAdaptor(helmAppDetail)
	impl.Logger.Infow("appdetail", "detail", res)
	return res, nil
}

func (impl *ApplicationServiceServerImpl) Hibernate(ctx context.Context, in *client.HibernateRequest) (*client.HibernateResponse, error) {
	impl.Logger.Infow("hibernate req")
	res, err := impl.HelmAppService.Hibernate(ctx, in.ClusterConfig, in.ObjectIdentifier)
	return res, err
}

func (impl *ApplicationServiceServerImpl) UnHibernate(ctx context.Context, in *client.HibernateRequest) (*client.HibernateResponse, error) {
	impl.Logger.Infow("unhibernate req")
	res, err := impl.HelmAppService.UnHibernate(ctx, in.ClusterConfig, in.GetObjectIdentifier())
	return res, err
}

func (impl *ApplicationServiceServerImpl) GetDeploymentHistory(ctxt context.Context, in *client.AppDetailRequest) (*client.HelmAppDeploymentHistory, error) {
	impl.Logger.Infow("deployment history")
	res, err := impl.HelmAppService.GetDeploymentHistory(in)
	impl.Logger.Infow("deployment history res", "res", res)

	return res, err
}

func (impl *ApplicationServiceServerImpl) GetValuesYaml(ctx context.Context, in *client.AppDetailRequest) (*client.ReleaseInfo, error) {
	impl.Logger.Infow("Get app values")
	return impl.HelmAppService.GetHelmAppValues(in)
}

func (impl *ApplicationServiceServerImpl) GetDesiredManifest(ctx context.Context, in *client.ObjectRequest) (*client.DesiredManifestResponse, error) {
	impl.Logger.Infow("Get desired manifest request")
	return impl.HelmAppService.GetDesiredManifest(in)
}

func (impl *ApplicationServiceServerImpl) UninstallRelease(ctx context.Context, in *client.ReleaseIdentifier) (*client.UninstallReleaseResponse, error) {
	impl.Logger.Infow("uninstall release request")
	return impl.HelmAppService.UninstallRelease(in)
}

func (impl *ApplicationServiceServerImpl) UpgradeRelease(ctx context.Context, in *client.UpgradeReleaseRequest) (*client.UpgradeReleaseResponse, error) {
	impl.Logger.Infow("upgrade release request")
	return impl.HelmAppService.UpgradeRelease(ctx, in)
}

func (impl *ApplicationServiceServerImpl) GetDeploymentDetail(ctx context.Context, in *client.DeploymentDetailRequest) (*client.DeploymentDetailResponse, error) {
	impl.Logger.Infow("get deployment detail request")
	return impl.HelmAppService.GetDeploymentDetail(in)
}

func (impl *ApplicationServiceServerImpl) InstallRelease(ctx context.Context, in *client.InstallReleaseRequest) (*client.InstallReleaseResponse, error) {
	impl.Logger.Infow("install release request")
	impl.ChartRepositoryLocker.Lock(in.ChartRepository.Name)
	defer impl.ChartRepositoryLocker.Unlock(in.ChartRepository.Name)
	return impl.HelmAppService.InstallRelease(ctx, in)
}

func (impl *ApplicationServiceServerImpl) UpgradeReleaseWithChartInfo(ctx context.Context, in *client.InstallReleaseRequest) (*client.UpgradeReleaseResponse, error) {
	impl.Logger.Infow("upgrade release with chart info request")
	impl.ChartRepositoryLocker.Lock(in.ChartRepository.Name)
	defer impl.ChartRepositoryLocker.Unlock(in.ChartRepository.Name)
	return impl.HelmAppService.UpgradeReleaseWithChartInfo(ctx, in)
}

func resourceRefResult(resourceRefs []*bean.ResourceRef) (resourceRefResults []*client.ResourceRef) {
	for _, resourceRef := range resourceRefs {
		resourceRefResult := &client.ResourceRef{
			Group:     resourceRef.Group,
			Version:   resourceRef.Version,
			Kind:      resourceRef.Kind,
			Namespace: resourceRef.Namespace,
			Name:      resourceRef.Name,
			Uid:       resourceRef.UID,
		}
		resourceRefResults = append(resourceRefResults, resourceRefResult)
	}
	return resourceRefResults
}
func (impl *ApplicationServiceServerImpl) AppDetailAdaptor(req *bean.AppDetail) *client.AppDetail {
	var resourceNodes []*client.ResourceNode
	for _, node := range req.ResourceTreeResponse.Nodes {
		var healthStatus *client.HealthStatus
		if node.Health != nil {
			healthStatus = &client.HealthStatus{
				Status:  node.Health.Status,
				Message: node.Health.Message,
			}
		}
		resourceNode := &client.ResourceNode{
			Group:      node.Group,
			Version:    node.Version,
			Kind:       node.Kind,
			Namespace:  node.Namespace,
			Name:       node.Name,
			Uid:        node.UID,
			ParentRefs: resourceRefResult(node.ParentRefs),
			NetworkingInfo: &client.ResourceNetworkingInfo{
				Labels: node.NetworkingInfo.Labels,
			},
			ResourceVersion: node.ResourceVersion,
			Health:          healthStatus,
			IsHibernated:    node.IsHibernated,
			CanBeHibernated: node.CanBeHibernated,
		}
		resourceNodes = append(resourceNodes, resourceNode)
	}

	var podMetadatas []*client.PodMetadata
	for _, pm := range req.ResourceTreeResponse.PodMetadata {
		podMetadata := &client.PodMetadata{
			Name:           pm.Name,
			Uid:            pm.UID,
			Containers:     pm.Containers,
			InitContainers: pm.InitContainers,
			IsNew:          pm.IsNew,
		}
		podMetadatas = append(podMetadatas, podMetadata)
	}
	appDetail := &client.AppDetail{
		ApplicationStatus: string(*req.ApplicationStatus),
		ReleaseStatus: &client.ReleaseStatus{
			Status:      req.ReleaseStatus.Status,
			Message:     req.ReleaseStatus.Message,
			Description: req.ReleaseStatus.Description,
		},
		LastDeployed: timestamppb.New(req.LastDeployed),
		ChartMetadata: &client.ChartMetadata{
			ChartName:    req.ChartMetadata.ChartName,
			ChartVersion: req.ChartMetadata.ChartVersion,
			Home:         req.ChartMetadata.Home,
			Sources:      req.ChartMetadata.Sources,
			Description:  req.ChartMetadata.Description,
		},
		ResourceTreeResponse: &client.ResourceTreeResponse{
			Nodes:       resourceNodes,
			PodMetadata: podMetadatas,
		},
		EnvironmentDetails: req.EnvironmentDetails,
	}
	return appDetail
}
