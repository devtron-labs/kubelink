package service

import (
	"context"
	"fmt"
	"github.com/devtron-labs/kubelink/bean"
	"github.com/devtron-labs/kubelink/grpc"
	"github.com/devtron-labs/kubelink/internal"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ApplicationServiceServerImpl struct {
	client.UnimplementedApplicationServiceServer
	appService            AppService
	logger                *zap.SugaredLogger
	chartRepositoryLocker *internal.ChartRepositoryLocker
}

func (impl *ApplicationServiceServerImpl) mustEmbedUnimplementedApplicationServiceServer() {
	panic("implement me")
}

func NewApplicationServiceServerImpl(logger *zap.SugaredLogger, chartRepositoryLocker *internal.ChartRepositoryLocker) *ApplicationServiceServerImpl {
	return &ApplicationServiceServerImpl{
		logger:                logger,
		chartRepositoryLocker: chartRepositoryLocker,
	}
}

func (impl *ApplicationServiceServerImpl) ListApplications(req *client.AppListRequest, res client.ApplicationService_ListApplicationsServer) error {
	impl.logger.Infow("app list req")
	clusterConfigs := req.GetClusters()
	eg := new(errgroup.Group)
	for _, config := range clusterConfigs {
		clusterConfig := *config
		eg.Go(func() error {
			apps := impl.appService.GetApplicationListForCluster(&clusterConfig)
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
	impl.logger.Infow("list app done")
	return nil
}

func (impl *ApplicationServiceServerImpl) GetAppDetail(ctxt context.Context, req *client.AppDetailRequest) (*client.AppDetail, error) {
	impl.logger.Infow("get app detail", "release", req.ReleaseName, "ns", req.Namespace)
	helmAppDetail, err := BuildAppDetail(req)
	if err != nil {
		return nil, err
	}
	res := impl.AppDetailAdaptor(helmAppDetail)
	impl.logger.Infow("appdetail", "detail", res)
	return res, nil
}

func (impl *ApplicationServiceServerImpl) Hibernate(ctx context.Context, in *client.HibernateRequest) (*client.HibernateResponse, error) {
	impl.logger.Infow("hibernate req")
	res, err := Hibernate(ctx, in.ClusterConfig, in.ObjectIdentifier)
	return res, err
}

func (impl *ApplicationServiceServerImpl) UnHibernate(ctx context.Context, in *client.HibernateRequest) (*client.HibernateResponse, error) {
	impl.logger.Infow("unhibernate req")
	res, err := UnHibernate(ctx, in.ClusterConfig, in.GetObjectIdentifier())
	return res, err
}

func (impl *ApplicationServiceServerImpl) GetDeploymentHistory(ctxt context.Context, in *client.AppDetailRequest) (*client.HelmAppDeploymentHistory, error) {
	impl.logger.Infow("deployment history")
	res, err := GetDeploymentHistory(in)
	impl.logger.Infow("deployment history res", "res", res)

	return res, err
}

func (impl *ApplicationServiceServerImpl) GetValuesYaml(ctx context.Context, in *client.AppDetailRequest) (*client.ReleaseInfo, error) {
	impl.logger.Infow("Get app values")
	return GetHelmAppValues(in)
}

func (impl *ApplicationServiceServerImpl) GetDesiredManifest(ctx context.Context, in *client.ObjectRequest) (*client.DesiredManifestResponse, error) {
	impl.logger.Infow("Get desired manifest request")
	return GetDesiredManifest(in)
}

func (impl *ApplicationServiceServerImpl) UninstallRelease(ctx context.Context, in *client.ReleaseIdentifier) (*client.UninstallReleaseResponse, error) {
	impl.logger.Infow("uninstall release request")
	return UninstallRelease(in)
}

func (impl *ApplicationServiceServerImpl) UpgradeRelease(ctx context.Context, in *client.UpgradeReleaseRequest) (*client.UpgradeReleaseResponse, error) {
	impl.logger.Infow("upgrade release request")
	return UpgradeRelease(ctx, in)
}

func (impl *ApplicationServiceServerImpl) GetDeploymentDetail(ctx context.Context, in *client.DeploymentDetailRequest) (*client.DeploymentDetailResponse, error) {
	impl.logger.Infow("get deployment detail request")
	return GetDeploymentDetail(in)
}

func (impl *ApplicationServiceServerImpl) InstallRelease(ctx context.Context, in *client.InstallReleaseRequest) (*client.InstallReleaseResponse, error) {
	impl.logger.Infow("install release request")
	impl.chartRepositoryLocker.Lock(in.ChartRepository.Name)
	defer impl.chartRepositoryLocker.Unlock(in.ChartRepository.Name)
	return InstallRelease(ctx, in)
}

func (impl *ApplicationServiceServerImpl) UpgradeReleaseWithChartInfo(ctx context.Context, in *client.InstallReleaseRequest) (*client.UpgradeReleaseResponse, error) {
	impl.logger.Infow("upgrade release with chart info request")
	impl.chartRepositoryLocker.Lock(in.ChartRepository.Name)
	defer impl.chartRepositoryLocker.Unlock(in.ChartRepository.Name)
	return UpgradeReleaseWithChartInfo(ctx, in)
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
