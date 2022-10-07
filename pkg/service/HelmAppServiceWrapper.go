package service

import (
	"context"
	"errors"
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

func (impl *ApplicationServiceServerImpl) InstallReleaseWithCustomChart(ctx context.Context, req *client.HelmInstallCustomRequest) (*client.HelmInstallCustomResponse, error) {
	impl.Logger.Infow("helm install request", "releaseIdentifier", req.ReleaseIdentifier, "values", req.ValuesYaml)
	flag, err := impl.HelmAppService.InstallReleaseWithCustomChart(req)
	if err != nil {
		impl.Logger.Errorw("Error in HelmInstallCustom  request", "err", err)
		return nil, err
	}
	res := &client.HelmInstallCustomResponse{Success: flag}
	return res, nil
}

func (impl *ApplicationServiceServerImpl) ListApplications(req *client.AppListRequest, res client.ApplicationService_ListApplicationsServer) error {
	impl.Logger.Info("List Application Request")
	clusterConfigs := req.GetClusters()
	eg := new(errgroup.Group)
	for _, config := range clusterConfigs {
		clusterConfig := *config
		eg.Go(func() error {
			apps := impl.HelmAppService.GetApplicationListForCluster(&clusterConfig)
			err := res.Send(apps)
			return err
		})
	}
	if err := eg.Wait(); err != nil {
		impl.Logger.Errorw("Error in fetching application list", "err", err)
		return err
	}
	impl.Logger.Info("List Application Request served")
	return nil
}

func (impl *ApplicationServiceServerImpl) GetAppDetail(ctxt context.Context, req *client.AppDetailRequest) (*client.AppDetail, error) {
	return nil, errors.New("hello")
}

func (impl *ApplicationServiceServerImpl) Hibernate(ctx context.Context, in *client.HibernateRequest) (*client.HibernateResponse, error) {
	impl.Logger.Info("Hibernate request")
	res, err := impl.HelmAppService.ScaleObjects(ctx, in.ClusterConfig, in.ObjectIdentifier, true)
	if err != nil {
		impl.Logger.Errorw("Error in Hibernating", "err", err)
	}
	impl.Logger.Info("Hibernate request served")
	return res, err
}

func (impl *ApplicationServiceServerImpl) UnHibernate(ctx context.Context, in *client.HibernateRequest) (*client.HibernateResponse, error) {
	impl.Logger.Info("UnHibernate request")
	res, err := impl.HelmAppService.ScaleObjects(ctx, in.ClusterConfig, in.GetObjectIdentifier(), false)
	if err != nil {
		impl.Logger.Errorw("Error in UnHibernating", "err", err)
	}
	impl.Logger.Info("UnHibernate request served")
	return res, err
}

func (impl *ApplicationServiceServerImpl) GetDeploymentHistory(ctxt context.Context, in *client.AppDetailRequest) (*client.HelmAppDeploymentHistory, error) {
	impl.Logger.Infow("Deployment history request", "clusterName", in.ClusterConfig.ClusterName, "releaseName", in.ReleaseName,
		"namespace", in.Namespace)

	res, err := impl.HelmAppService.GetDeploymentHistory(in)
	if err != nil {
		impl.Logger.Errorw("Error in Deployment history request", "err", err)
	}
	impl.Logger.Info("Deployment history request request served")
	return res, err
}

func (impl *ApplicationServiceServerImpl) GetValuesYaml(ctx context.Context, in *client.AppDetailRequest) (*client.ReleaseInfo, error) {
	impl.Logger.Infow("Values Yaml request", "clusterName", in.ClusterConfig.ClusterName, "releaseName", in.ReleaseName,
		"namespace", in.Namespace)

	res, err := impl.HelmAppService.GetHelmAppValues(in)
	if err != nil {
		impl.Logger.Errorw("Error in Values Yaml request", "err", err)
	}
	impl.Logger.Info("Values Yaml request served")
	return res, err
}

func (impl *ApplicationServiceServerImpl) GetDesiredManifest(ctx context.Context, in *client.ObjectRequest) (*client.DesiredManifestResponse, error) {
	impl.Logger.Infow("Desired Manifest request", "clusterName", in.ClusterConfig.ClusterName, "releaseName", in.ReleaseName,
		"namespace", in.ReleaseNamespace, "objectName", in.ObjectIdentifier.Name)

	res, err := impl.HelmAppService.GetDesiredManifest(in)
	if err != nil {
		impl.Logger.Errorw("Error in Desired Manifest request", "err", err)
	}
	impl.Logger.Info("Desired Manifest request served")

	return res, err
}

func (impl *ApplicationServiceServerImpl) UninstallRelease(ctx context.Context, in *client.ReleaseIdentifier) (*client.UninstallReleaseResponse, error) {
	impl.Logger.Infow("Uninstall release request", "clusterName", in.ClusterConfig.ClusterName, "releaseName", in.ReleaseName,
		"namespace", in.ReleaseNamespace)

	res, err := impl.HelmAppService.UninstallRelease(in)
	if err != nil {
		impl.Logger.Errorw("Error in Uninstall release request", "err", err)
	}
	impl.Logger.Info("Uninstall release request served")

	return res, err
}

func (impl *ApplicationServiceServerImpl) UpgradeRelease(ctx context.Context, in *client.UpgradeReleaseRequest) (*client.UpgradeReleaseResponse, error) {
	releaseIdentifier := in.ReleaseIdentifier
	impl.Logger.Infow("Upgrade release request", "clusterName", releaseIdentifier.ClusterConfig.ClusterName, "releaseName", releaseIdentifier.ReleaseName,
		"namespace", releaseIdentifier.ReleaseNamespace)

	res, err := impl.HelmAppService.UpgradeRelease(ctx, in)
	if err != nil {
		impl.Logger.Errorw("Error in Upgrade release request", "err", err)
	}
	impl.Logger.Info("Upgrade release request served")

	return res, err
}

func (impl *ApplicationServiceServerImpl) GetDeploymentDetail(ctx context.Context, in *client.DeploymentDetailRequest) (*client.DeploymentDetailResponse, error) {
	releaseIdentifier := in.ReleaseIdentifier
	impl.Logger.Infow("Deployment detail request", "clusterName", releaseIdentifier.ClusterConfig.ClusterName, "releaseName", releaseIdentifier.ReleaseName,
		"namespace", releaseIdentifier.ReleaseNamespace, "deploymentVersion", in.DeploymentVersion)

	res, err := impl.HelmAppService.GetDeploymentDetail(in)
	if err != nil {
		impl.Logger.Errorw("Error in Deployment detail request", "err", err)
	}
	impl.Logger.Info("Deployment detail request served")

	return res, err
}

func (impl *ApplicationServiceServerImpl) InstallRelease(ctx context.Context, in *client.InstallReleaseRequest) (*client.InstallReleaseResponse, error) {
	releaseIdentifier := in.ReleaseIdentifier
	impl.Logger.Infow("Install release request", "clusterName", releaseIdentifier.ClusterConfig.ClusterName, "releaseName", releaseIdentifier.ReleaseName,
		"namespace", releaseIdentifier.ReleaseNamespace)

	impl.ChartRepositoryLocker.Lock(in.ChartRepository.Name)
	defer impl.ChartRepositoryLocker.Unlock(in.ChartRepository.Name)

	res, err := impl.HelmAppService.InstallRelease(ctx, in)
	if err != nil {
		impl.Logger.Errorw("Error in Install release request", "err", err)
	}
	impl.Logger.Info("Install release request served")

	return res, err
}

func (impl *ApplicationServiceServerImpl) UpgradeReleaseWithChartInfo(ctx context.Context, in *client.InstallReleaseRequest) (*client.UpgradeReleaseResponse, error) {
	releaseIdentifier := in.ReleaseIdentifier
	impl.Logger.Infow("Upgrade release with chart Info request", "clusterName", releaseIdentifier.ClusterConfig.ClusterName, "releaseName", releaseIdentifier.ReleaseName,
		"namespace", releaseIdentifier.ReleaseNamespace)

	impl.ChartRepositoryLocker.Lock(in.ChartRepository.Name)
	defer impl.ChartRepositoryLocker.Unlock(in.ChartRepository.Name)

	res, err := impl.HelmAppService.UpgradeReleaseWithChartInfo(ctx, in)
	if err != nil {
		impl.Logger.Errorw("Error in Upgrade release request with Chart Info", "err", err)
	}
	impl.Logger.Info("Upgrade release with chart Info request served")

	return res, err
}

func (impl *ApplicationServiceServerImpl) IsReleaseInstalled(ctx context.Context, releaseIdentifier *client.ReleaseIdentifier) (*client.BooleanResponse, error) {
	impl.Logger.Infow("IsReleaseInstalled request", "clusterName", releaseIdentifier.ClusterConfig.ClusterName, "releaseName", releaseIdentifier.ReleaseName,
		"namespace", releaseIdentifier.ReleaseNamespace)

	isInstalled, err := impl.HelmAppService.IsReleaseInstalled(ctx, releaseIdentifier)
	if err != nil {
		impl.Logger.Errorw("Error in IsReleaseInstalled request", "err", err)
	}
	impl.Logger.Info("IsReleaseInstalled request served")

	res := &client.BooleanResponse{
		Result: isInstalled,
	}
	return res, err
}

func (impl *ApplicationServiceServerImpl) RollbackRelease(ctx context.Context, in *client.RollbackReleaseRequest) (*client.BooleanResponse, error) {
	releaseIdentifier := in.ReleaseIdentifier
	impl.Logger.Infow("Rollback release request", "clusterName", releaseIdentifier.ClusterConfig.ClusterName, "releaseName", releaseIdentifier.ReleaseName,
		"namespace", releaseIdentifier.ReleaseNamespace, "version", in.Version)

	success, err := impl.HelmAppService.RollbackRelease(in)
	if err != nil {
		impl.Logger.Errorw("Error in Rollback release request", "err", err)
	}
	impl.Logger.Info("Rollback release request served")

	res := &client.BooleanResponse{
		Result: success,
	}
	return res, err
}

func (impl *ApplicationServiceServerImpl) TemplateChart(ctx context.Context, in *client.InstallReleaseRequest) (*client.TemplateChartResponse, error) {
	releaseIdentifier := in.ReleaseIdentifier
	impl.Logger.Infow("Template chart request", "clusterName", releaseIdentifier.ClusterConfig.ClusterName, "releaseName", releaseIdentifier.ReleaseName,
		"namespace", releaseIdentifier.ReleaseNamespace)

	impl.ChartRepositoryLocker.Lock(in.ChartRepository.Name)
	defer impl.ChartRepositoryLocker.Unlock(in.ChartRepository.Name)

	manifest, err := impl.HelmAppService.TemplateChart(ctx, in)
	if err != nil {
		impl.Logger.Errorw("Error in Template chart request", "err", err)
	}
	impl.Logger.Info("Template chart request served")

	res := &client.TemplateChartResponse{
		GeneratedManifest: manifest,
	}

	return res, err
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
			Notes:        req.ChartMetadata.Notes,
		},
		ResourceTreeResponse: &client.ResourceTreeResponse{
			Nodes:       resourceNodes,
			PodMetadata: podMetadatas,
		},
		EnvironmentDetails: req.EnvironmentDetails,
	}
	return appDetail
}
