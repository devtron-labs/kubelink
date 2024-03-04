package service

import (
	"context"
	"fmt"
	"github.com/devtron-labs/kubelink/bean"
	"github.com/devtron-labs/kubelink/grpc"
	"github.com/devtron-labs/kubelink/internals/lock"
	"github.com/uptrace/opentelemetry-go-extra/otelzap"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ApplicationServiceServerImpl struct {
	client.UnimplementedApplicationServiceServer
	Logger                *otelzap.SugaredLogger
	ChartRepositoryLocker *lock.ChartRepositoryLocker
	HelmAppService        HelmAppService
}

func (impl *ApplicationServiceServerImpl) MustEmbedUnimplementedApplicationServiceServer() {
	panic("implement me")
}

func NewApplicationServiceServerImpl(logger *otelzap.SugaredLogger, chartRepositoryLocker *lock.ChartRepositoryLocker,
	HelmAppService HelmAppService) *ApplicationServiceServerImpl {
	return &ApplicationServiceServerImpl{
		Logger:                logger,
		ChartRepositoryLocker: chartRepositoryLocker,
		HelmAppService:        HelmAppService,
	}
}

func (impl *ApplicationServiceServerImpl) InstallReleaseWithCustomChart(ctx context.Context, req *client.HelmInstallCustomRequest) (*client.HelmInstallCustomResponse, error) {
	res := &client.HelmInstallCustomResponse{}
	impl.Logger.Ctx(ctx).Infow("helm install request", "releaseIdentifier", req.ReleaseIdentifier, "values", req.ValuesYaml)
	// handling for running helm Install operation with context in Devtron app
	switch req.RunInCtx {
	case true:
		flag, err := impl.HelmAppService.InstallReleaseWithCustomChart(ctx, req)
		if err != nil {
			impl.Logger.Ctx(ctx).Errorw("Error in HelmInstallCustom  request", "err", err)
			return nil, err
		}
		res.Success = flag
	case false:
		flag, err := impl.HelmAppService.InstallReleaseWithCustomChart(context.Background(), req)
		if err != nil {
			impl.Logger.Ctx(ctx).Errorw("Error in HelmInstallCustom  request", "err", err)
			return nil, err
		}
		res.Success = flag
	}
	return res, nil
}

func (impl *ApplicationServiceServerImpl) ListApplications(req *client.AppListRequest, res client.ApplicationService_ListApplicationsServer) error {
	ctx := res.Context()
	impl.Logger.Ctx(ctx).Infow("List Application Request")
	span := trace.SpanFromContext(ctx)
	fmt.Println("SPAN: " + span.SpanContext().TraceID().String())
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
		impl.Logger.Ctx(ctx).Errorw("Error in fetching application list", "err", err)
		return err
	}
	impl.Logger.Ctx(ctx).Infow("List Application Request served")
	return nil
}

func (impl *ApplicationServiceServerImpl) GetAppDetail(ctx context.Context, req *client.AppDetailRequest) (*client.AppDetail, error) {
	impl.Logger.Ctx(ctx).Infow("App detail request", "clusterName", req.ClusterConfig.ClusterName, "releaseName", req.ReleaseName,
		"namespace", req.Namespace)
	span := trace.SpanFromContext(ctx)
	fmt.Println("SPAN: " + span.SpanContext().TraceID().String())
	helmAppDetail, err := impl.HelmAppService.BuildAppDetail(req)
	if err != nil {
		if helmAppDetail != nil && !helmAppDetail.ReleaseExists {
			// This error (release not exists for this app) is being used in orchestrator so please don't edit it.
			return &client.AppDetail{ReleaseExist: false}, fmt.Errorf("release not exists for this app")
		}
		impl.Logger.Ctx(ctx).Errorw("Error in getting app detail", "clusterName", req.ClusterConfig.ClusterName, "releaseName", req.ReleaseName,
			"namespace", req.Namespace, "err", err)
		return nil, err
	}
	res := impl.AppDetailAdaptor(ctx, helmAppDetail)
	impl.Logger.Ctx(ctx).Infow("App Detail Request served")
	return res, nil
}

func (impl *ApplicationServiceServerImpl) GetResourceTreeForExternalResources(ctx context.Context, req *client.ExternalResourceTreeRequest) (*client.ResourceTreeResponse, error) {
	resourceTree, err := impl.HelmAppService.GetResourceTreeForExternalResources(req)
	if err != nil {
		impl.Logger.Ctx(ctx).Errorw("error in getting resource tree for external resources", "err", err)
		return nil, err
	}
	resourceTreeResponse := impl.ResourceTreeAdapter(ctx, resourceTree)
	return resourceTreeResponse, nil
}

func (impl *ApplicationServiceServerImpl) GetAppStatus(ctx context.Context, req *client.AppDetailRequest) (*client.AppStatus, error) {
	impl.Logger.Ctx(ctx).Infow("App detail request", "clusterName", req.ClusterConfig.ClusterName, "releaseName", req.ReleaseName,
		"namespace", req.Namespace)

	helmAppStatus, err := impl.HelmAppService.FetchApplicationStatus(req)
	if err != nil {
		impl.Logger.Ctx(ctx).Errorw("Error in getting app detail", "clusterName", req.ClusterConfig.ClusterName, "releaseName", req.ReleaseName,
			"namespace", req.Namespace, "err", err)
		return nil, err
	}
	return helmAppStatus, nil
}

func (impl *ApplicationServiceServerImpl) Hibernate(ctx context.Context, in *client.HibernateRequest) (*client.HibernateResponse, error) {
	impl.Logger.Ctx(ctx).Infow("Hibernate request")
	res, err := impl.HelmAppService.ScaleObjects(ctx, in.ClusterConfig, in.ObjectIdentifier, true)
	if err != nil {
		impl.Logger.Ctx(ctx).Errorw("Error in Hibernating", "err", err)
	}
	impl.Logger.Ctx(ctx).Infow("Hibernate request served")
	return res, err
}

func (impl *ApplicationServiceServerImpl) UnHibernate(ctx context.Context, in *client.HibernateRequest) (*client.HibernateResponse, error) {
	impl.Logger.Ctx(ctx).Infow("UnHibernate request")
	res, err := impl.HelmAppService.ScaleObjects(ctx, in.ClusterConfig, in.GetObjectIdentifier(), false)
	if err != nil {
		impl.Logger.Ctx(ctx).Errorw("Error in UnHibernating", "err", err)
	}
	impl.Logger.Ctx(ctx).Infow("UnHibernate request served")
	return res, err
}

func (impl *ApplicationServiceServerImpl) GetDeploymentHistory(ctx context.Context, in *client.AppDetailRequest) (*client.HelmAppDeploymentHistory, error) {
	impl.Logger.Ctx(ctx).Infow("Deployment history request", "clusterName", in.ClusterConfig.ClusterName, "releaseName", in.ReleaseName,
		"namespace", in.Namespace)

	res, err := impl.HelmAppService.GetDeploymentHistory(in)
	if err != nil {
		impl.Logger.Ctx(ctx).Errorw("Error in Deployment history request", "err", err)
	}
	impl.Logger.Ctx(ctx).Infow("Deployment history request request served")
	return res, err
}

func (impl *ApplicationServiceServerImpl) GetValuesYaml(ctx context.Context, in *client.AppDetailRequest) (*client.ReleaseInfo, error) {
	impl.Logger.Ctx(ctx).Infow("Values Yaml request", "clusterName", in.ClusterConfig.ClusterName, "releaseName", in.ReleaseName,
		"namespace", in.Namespace)

	res, err := impl.HelmAppService.GetHelmAppValues(in)
	if err != nil {
		impl.Logger.Ctx(ctx).Errorw("Error in Values Yaml request", "err", err)
	}
	impl.Logger.Ctx(ctx).Infow("Values Yaml request served")
	return res, err
}

func (impl *ApplicationServiceServerImpl) GetDesiredManifest(ctx context.Context, in *client.ObjectRequest) (*client.DesiredManifestResponse, error) {
	impl.Logger.Ctx(ctx).Infow("Desired Manifest request", "clusterName", in.ClusterConfig.ClusterName, "releaseName", in.ReleaseName,
		"namespace", in.ReleaseNamespace, "objectName", in.ObjectIdentifier.Name)

	res, err := impl.HelmAppService.GetDesiredManifest(in)
	if err != nil {
		impl.Logger.Ctx(ctx).Errorw("Error in Desired Manifest request", "err", err)
	}
	impl.Logger.Ctx(ctx).Infow("Desired Manifest request served")

	return res, err
}

func (impl *ApplicationServiceServerImpl) UninstallRelease(ctx context.Context, in *client.ReleaseIdentifier) (*client.UninstallReleaseResponse, error) {
	impl.Logger.Ctx(ctx).Infow("Uninstall release request", "clusterName", in.ClusterConfig.ClusterName, "releaseName", in.ReleaseName,
		"namespace", in.ReleaseNamespace)
	res, err := impl.HelmAppService.UninstallRelease(in)
	if err != nil {
		//This case occurs when we uninstall a release using the (CLI) and then try to delete  cd from UI.
		isReleaseInstalled, releaseErr := impl.HelmAppService.IsReleaseInstalled(context.Background(), in)
		if releaseErr != nil {
			impl.Logger.Ctx(ctx).Errorw("error in checking if release is installed or not")
			return nil, status.Error(codes.Internal, releaseErr.Error())
		}
		if !isReleaseInstalled {
			impl.Logger.Ctx(ctx).Errorw("error, no release found", "ReleaseIdentifier", in)
			return nil, status.Error(codes.NotFound, fmt.Sprintf(" release not found for '%s'", in.ReleaseName))
		}
		impl.Logger.Ctx(ctx).Errorw("Error in Uninstall release request", "err", err)
		return nil, err
	}
	impl.Logger.Ctx(ctx).Infow("Uninstall release request served")

	return res, err
}

func (impl *ApplicationServiceServerImpl) UpgradeRelease(ctx context.Context, in *client.UpgradeReleaseRequest) (*client.UpgradeReleaseResponse, error) {
	releaseIdentifier := in.ReleaseIdentifier
	impl.Logger.Ctx(ctx).Infow("Upgrade release request", "clusterName", releaseIdentifier.ClusterConfig.ClusterName, "releaseName", releaseIdentifier.ReleaseName,
		"namespace", releaseIdentifier.ReleaseNamespace)

	res, err := impl.HelmAppService.UpgradeRelease(ctx, in)
	if err != nil {
		impl.Logger.Ctx(ctx).Errorw("Error in Upgrade release request", "err", err)
		return res, err
	}
	impl.Logger.Ctx(ctx).Infow("Upgrade release request served")

	return res, err
}

func (impl *ApplicationServiceServerImpl) GetDeploymentDetail(ctx context.Context, in *client.DeploymentDetailRequest) (*client.DeploymentDetailResponse, error) {
	releaseIdentifier := in.ReleaseIdentifier
	impl.Logger.Ctx(ctx).Infow("Deployment detail request", "clusterName", releaseIdentifier.ClusterConfig.ClusterName, "releaseName", releaseIdentifier.ReleaseName,
		"namespace", releaseIdentifier.ReleaseNamespace, "deploymentVersion", in.DeploymentVersion)

	res, err := impl.HelmAppService.GetDeploymentDetail(in)
	if err != nil {
		impl.Logger.Ctx(ctx).Errorw("Error in Deployment detail request", "err", err)
	}
	impl.Logger.Ctx(ctx).Infow("Deployment detail request served")

	return res, err
}

func (impl *ApplicationServiceServerImpl) InstallRelease(ctx context.Context, in *client.InstallReleaseRequest) (*client.InstallReleaseResponse, error) {
	releaseIdentifier := in.ReleaseIdentifier
	impl.Logger.Ctx(ctx).Infow("Install release request", "clusterName", releaseIdentifier.ClusterConfig.ClusterName, "releaseName", releaseIdentifier.ReleaseName,
		"namespace", releaseIdentifier.ReleaseNamespace)

	if in.ChartRepository != nil {
		impl.ChartRepositoryLocker.Lock(in.ChartRepository.Name)
		defer impl.ChartRepositoryLocker.Unlock(in.ChartRepository.Name)
	}

	res, err := impl.HelmAppService.InstallRelease(ctx, in)
	if err != nil {
		impl.Logger.Ctx(ctx).Errorw("Error in Install release request", "err", err)
	}
	impl.Logger.Ctx(ctx).Infow("Install release request served")

	return res, err
}

func (impl *ApplicationServiceServerImpl) UpgradeReleaseWithChartInfo(ctx context.Context, in *client.InstallReleaseRequest) (*client.UpgradeReleaseResponse, error) {
	releaseIdentifier := in.ReleaseIdentifier
	impl.Logger.Ctx(ctx).Infow("Upgrade release with chart Info request", "clusterName", releaseIdentifier.ClusterConfig.ClusterName, "releaseName", releaseIdentifier.ReleaseName,
		"namespace", releaseIdentifier.ReleaseNamespace)

	if in.ChartRepository != nil {
		impl.ChartRepositoryLocker.Lock(in.ChartRepository.Name)
		defer impl.ChartRepositoryLocker.Unlock(in.ChartRepository.Name)
	}
	res, err := impl.HelmAppService.UpgradeReleaseWithChartInfo(ctx, in)
	if err != nil {
		impl.Logger.Ctx(ctx).Errorw("Error in Upgrade release request with Chart Info", "err", err)
	}
	impl.Logger.Ctx(ctx).Infow("Upgrade release with chart Info request served")

	return res, err
}

func (impl *ApplicationServiceServerImpl) IsReleaseInstalled(ctx context.Context, releaseIdentifier *client.ReleaseIdentifier) (*client.BooleanResponse, error) {
	impl.Logger.Ctx(ctx).Infow("IsReleaseInstalled request", "clusterName", releaseIdentifier.ClusterConfig.ClusterName, "releaseName", releaseIdentifier.ReleaseName,
		"namespace", releaseIdentifier.ReleaseNamespace)

	isInstalled, err := impl.HelmAppService.IsReleaseInstalled(ctx, releaseIdentifier)
	if err != nil {
		impl.Logger.Ctx(ctx).Errorw("Error in IsReleaseInstalled request", "err", err)
	}
	impl.Logger.Ctx(ctx).Infow("IsReleaseInstalled request served")

	res := &client.BooleanResponse{
		Result: isInstalled,
	}
	return res, err
}

func (impl *ApplicationServiceServerImpl) RollbackRelease(ctx context.Context, in *client.RollbackReleaseRequest) (*client.BooleanResponse, error) {
	releaseIdentifier := in.ReleaseIdentifier
	impl.Logger.Ctx(ctx).Infow("Rollback release request", "clusterName", releaseIdentifier.ClusterConfig.ClusterName, "releaseName", releaseIdentifier.ReleaseName,
		"namespace", releaseIdentifier.ReleaseNamespace, "version", in.Version)

	success, err := impl.HelmAppService.RollbackRelease(in)
	if err != nil {
		impl.Logger.Ctx(ctx).Errorw("Error in Rollback release request", "err", err)
	}
	impl.Logger.Ctx(ctx).Infow("Rollback release request served")

	res := &client.BooleanResponse{
		Result: success,
	}
	return res, err
}

func (impl *ApplicationServiceServerImpl) TemplateChart(ctx context.Context, in *client.InstallReleaseRequest) (*client.TemplateChartResponse, error) {
	releaseIdentifier := in.ReleaseIdentifier
	impl.Logger.Ctx(ctx).Infow("Template chart request", "clusterName", releaseIdentifier.ClusterConfig.ClusterName, "releaseName", releaseIdentifier.ReleaseName,
		"namespace", releaseIdentifier.ReleaseNamespace)
	if in.ChartRepository != nil {
		impl.ChartRepositoryLocker.Lock(in.ChartRepository.Name)
		defer impl.ChartRepositoryLocker.Unlock(in.ChartRepository.Name)
	}
	manifest, err := impl.HelmAppService.TemplateChart(ctx, in)
	if err != nil {
		impl.Logger.Ctx(ctx).Errorw("Error in Template chart request", "err", err)
	}
	impl.Logger.Ctx(ctx).Infow("Template chart request served")

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

func (impl *ApplicationServiceServerImpl) ResourceTreeAdapter(ctx context.Context, req *bean.ResourceTreeResponse) *client.ResourceTreeResponse {
	var resourceNodes []*client.ResourceNode
	for _, node := range req.Nodes {
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
			Port:       node.Port,
			ParentRefs: resourceRefResult(node.ParentRefs),
			NetworkingInfo: &client.ResourceNetworkingInfo{
				Labels: node.NetworkingInfo.Labels,
			},
			ResourceVersion: node.ResourceVersion,
			Health:          healthStatus,
			IsHibernated:    node.IsHibernated,
			CanBeHibernated: node.CanBeHibernated,
			Info:            impl.buildInfoItems(ctx, node.Info),
			CreatedAt:       node.CreatedAt,
		}
		resourceNodes = append(resourceNodes, resourceNode)
	}

	podMetadatas := make([]*client.PodMetadata, 0, len(req.PodMetadata))
	for _, pm := range req.PodMetadata {
		ephemeralContainers := make([]*client.EphemeralContainerData, 0, len(pm.EphemeralContainers))
		for _, ec := range pm.EphemeralContainers {
			ephemeralContainers = append(ephemeralContainers, &client.EphemeralContainerData{
				Name:       ec.Name,
				IsExternal: ec.IsExternal,
			})
		}
		podMetadata := &client.PodMetadata{
			Name:                pm.Name,
			Uid:                 pm.UID,
			Containers:          pm.Containers,
			InitContainers:      pm.InitContainers,
			EphemeralContainers: ephemeralContainers,
			IsNew:               pm.IsNew,
		}
		podMetadatas = append(podMetadatas, podMetadata)
	}
	resourceTreeResponse := &client.ResourceTreeResponse{
		Nodes:       resourceNodes,
		PodMetadata: podMetadatas,
	}
	return resourceTreeResponse
}

func (impl *ApplicationServiceServerImpl) AppDetailAdaptor(ctx context.Context, req *bean.AppDetail) *client.AppDetail {
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
			Info:            impl.buildInfoItems(ctx, node.Info),
			CreatedAt:       node.CreatedAt,
			Port:            node.Port,
			IsHook:          node.IsHook,
			HookType:        node.HookType,
		}
		resourceNodes = append(resourceNodes, resourceNode)
	}

	podMetadatas := make([]*client.PodMetadata, 0, len(req.ResourceTreeResponse.PodMetadata))
	for _, pm := range req.ResourceTreeResponse.PodMetadata {
		ephemeralContainers := make([]*client.EphemeralContainerData, 0, len(pm.EphemeralContainers))
		for _, ec := range pm.EphemeralContainers {
			ephemeralContainers = append(ephemeralContainers, &client.EphemeralContainerData{
				Name:       ec.Name,
				IsExternal: ec.IsExternal,
			})
		}
		podMetadata := &client.PodMetadata{
			Name:                pm.Name,
			Uid:                 pm.UID,
			Containers:          pm.Containers,
			InitContainers:      pm.InitContainers,
			EphemeralContainers: ephemeralContainers,
			IsNew:               pm.IsNew,
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
		ReleaseExist:       true,
	}
	return appDetail
}

func (impl *ApplicationServiceServerImpl) buildInfoItems(ctx context.Context, infoItemBeans []bean.InfoItem) []*client.InfoItem {
	infoItems := make([]*client.InfoItem, 0, len(infoItemBeans))
	for _, infoItemBean := range infoItemBeans {
		switch infoItemBean.Value.(type) {
		case string:
			infoItems = append(infoItems, &client.InfoItem{Name: infoItemBean.Name, Value: infoItemBean.Value.(string)})
		default:
			// skip other types
			impl.Logger.Ctx(ctx).Debugw("ignoring other info item value types", "infoItem", infoItemBean.Value)
		}

	}
	return infoItems
}

func (impl *ApplicationServiceServerImpl) GetNotes(ctx context.Context, installReleaseRequest *client.InstallReleaseRequest) (*client.ChartNotesResponse, error) {
	releaseNote, err := impl.HelmAppService.GetNotes(ctx, installReleaseRequest)
	if err != nil {
		impl.Logger.Ctx(ctx).Errorw("Error in fetching Notes ", "err", err)
		return nil, err
	}
	chartNotesResponse := &client.ChartNotesResponse{Notes: releaseNote}
	return chartNotesResponse, nil
}

func (impl *ApplicationServiceServerImpl) UpgradeReleaseWithCustomChart(ctx context.Context, request *client.UpgradeReleaseRequest) (*client.UpgradeReleaseResponse, error) {
	resp := &client.UpgradeReleaseResponse{}
	// handling for running helm Upgrade operation with context in Devtron app
	switch request.RunInCtx {
	case true:
		response, err := impl.HelmAppService.UpgradeReleaseWithCustomChart(ctx, request)
		if err != nil {
			impl.Logger.Ctx(ctx).Errorw("Error in upgrade release with custom chart", "err", err)
			return nil, err
		}
		resp.Success = response
	case false:
		response, err := impl.HelmAppService.UpgradeReleaseWithCustomChart(context.Background(), request)
		if err != nil {
			impl.Logger.Ctx(ctx).Errorw("Error in upgrade release with custom chart", "err", err)
			return nil, err
		}
		resp.Success = response
	}
	return resp, nil
}

func (impl *ApplicationServiceServerImpl) ValidateOCIRegistry(ctx context.Context, OCIRegistryRequest *client.RegistryCredential) (*client.OCIRegistryResponse, error) {
	isValid, err := impl.HelmAppService.ValidateOCIRegistryLogin(ctx, OCIRegistryRequest)
	if err != nil {
		impl.Logger.Ctx(ctx).Errorw("Error in fetching Notes ", "err", err)
		return nil, err
	}
	return isValid, nil
}

func (impl *ApplicationServiceServerImpl) PushHelmChartToOCIRegistry(ctx context.Context, OCIRegistryRequest *client.OCIRegistryRequest) (*client.OCIRegistryResponse, error) {
	registryPushResponse, err := impl.HelmAppService.PushHelmChartToOCIRegistryRepo(ctx, OCIRegistryRequest)
	if err != nil {
		impl.Logger.Ctx(ctx).Errorw("Error in pushing helm chart ", "chartName", OCIRegistryRequest.ChartName, "err", err)
		return nil, err
	}
	return registryPushResponse, nil
}
