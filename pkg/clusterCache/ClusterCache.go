package clusterCache

import (
	"errors"
	"fmt"
	clustercache "github.com/argoproj/gitops-engine/pkg/cache"
	"github.com/caarlos0/env"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	"github.com/devtron-labs/common-lib/utils/k8s/health"
	"github.com/devtron-labs/kubelink/bean"
	repository "github.com/devtron-labs/kubelink/pkg/cluster"
	"github.com/devtron-labs/kubelink/pkg/k8sInformer"
	"github.com/devtron-labs/kubelink/pkg/util/argo"
	"go.uber.org/zap"
	"golang.org/x/sync/semaphore"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sync"
)

type ClusterCache interface {
	//SyncClusterCache(clusterInfo bean.ClusterInfo) (clustercache.ClusterCache, error)
	GetClusterCacheByClusterId(clusterId int) (clustercache.ClusterCache, error)
}

type ClusterCacheConfig struct {
	ClusterIdList []int `env:"CLUSTER_ID_LIST" envSeparator:","`
}

func GetClusterCacheConfig() (*ClusterCacheConfig, error) {
	cfg := &ClusterCacheConfig{}
	err := env.Parse(cfg)
	return cfg, err
}

type ClusterCacheImpl struct {
	logger             *zap.SugaredLogger
	clusterCacheConfig *ClusterCacheConfig
	clusterRepository  repository.ClusterRepository
	k8sUtil            *k8sUtils.K8sUtil
	clustersCache      map[int]clustercache.ClusterCache
	rwMutex            sync.RWMutex
}

func NewClusterCacheImpl(logger *zap.SugaredLogger, clusterCacheConfig *ClusterCacheConfig,
	clusterRepository repository.ClusterRepository, k8sUtil *k8sUtils.K8sUtil) *ClusterCacheImpl {

	clustersCache := make(map[int]clustercache.ClusterCache)
	clusterCacheImpl := &ClusterCacheImpl{
		logger:             logger,
		clusterCacheConfig: clusterCacheConfig,
		clusterRepository:  clusterRepository,
		k8sUtil:            k8sUtil,
		clustersCache:      clustersCache,
	}

	if len(clusterCacheConfig.ClusterIdList) > 0 {
		err := clusterCacheImpl.SyncCache()
		if err != nil {
			return nil
		}
	}
	return clusterCacheImpl
}

func (impl *ClusterCacheImpl) getClusterInfoByClusterId(clusterId int) (*bean.ClusterInfo, error) {
	model, err := impl.clusterRepository.FindById(clusterId)
	if err != nil {
		impl.logger.Errorw("error in getting cluster from db by cluster id", "clusterId", clusterId)
		return nil, err
	}
	clusterInfo := k8sInformer.GetClusterInfo(model)
	return clusterInfo, nil
}

func (impl *ClusterCacheImpl) SyncCache() error {
	for _, clusterId := range impl.clusterCacheConfig.ClusterIdList {
		clusterInfo, err := impl.getClusterInfoByClusterId(clusterId)
		if err != nil {
			impl.logger.Errorw("error in getting clusterInfo by cluster id", "clusterId", clusterId)
			continue
		}

		go impl.SyncClusterCache(*clusterInfo)
	}
	return nil
}

func (impl *ClusterCacheImpl) SyncClusterCache(clusterInfo bean.ClusterInfo) (clustercache.ClusterCache, error) {
	impl.logger.Infow("cluster cache sync started..", "clusterId", clusterInfo.ClusterId)
	c, err := impl.getClusterCache(clusterInfo)
	if err != nil {
		impl.logger.Errorw("failed to get cluster info for", "clusterId", clusterInfo.ClusterId, "error", err)
		return c, err
	}
	err = c.EnsureSynced()
	if err != nil {
		impl.logger.Errorw("error in syncing cluster cache", "clusterId", clusterInfo.ClusterId, "sync-error", err)
		return c, err
	}
	impl.rwMutex.Lock()
	impl.clustersCache[clusterInfo.ClusterId] = c
	impl.rwMutex.Unlock()
	return c, nil
}

func (impl *ClusterCacheImpl) getClusterCache(clusterInfo bean.ClusterInfo) (clustercache.ClusterCache, error) {
	cache, err := impl.GetClusterCacheByClusterId(clusterInfo.ClusterId)
	if err == nil {
		return cache, nil
	}
	clusterConfig := clusterInfo.GetClusterConfig()
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(clusterConfig)
	if err != nil {
		impl.logger.Errorw("error in getting rest config", "err", err, "clusterName", clusterConfig.ClusterName)
		return cache, err
	}
	cache = clustercache.NewClusterCache(restConfig, getClusterCacheOptions()...)
	return cache, nil
}

func getClusterCacheOptions() []clustercache.UpdateSettingsFunc {
	clusterCacheOpts := []clustercache.UpdateSettingsFunc{
		clustercache.SetListSemaphore(semaphore.NewWeighted(clusterCacheListSemaphoreSize)),
		clustercache.SetListPageSize(clusterCacheListPageSize),
		clustercache.SetListPageBufferSize(clusterCacheListPageBufferSize),
		clustercache.SetWatchResyncTimeout(clusterCacheWatchResyncDuration),
		clustercache.SetClusterSyncRetryTimeout(clusterSyncRetryTimeoutDuration),
		clustercache.SetResyncTimeout(clusterCacheResyncDuration),
		clustercache.SetPopulateResourceInfoHandler(func(un *unstructured.Unstructured, isRoot bool) (interface{}, bool) {
			fmt.Println("resource updated")
			gvk := un.GroupVersionKind()
			res := &bean.ResourceNode{
				Port:            GetPorts(un, gvk),
				ResourceVersion: un.GetResourceVersion(),
				NetworkingInfo: &bean.ResourceNetworkingInfo{
					Labels: un.GetLabels(),
				},
			}

			if k8sUtils.IsService(gvk) && un.GetName() == k8sUtils.DEVTRON_SERVICE_NAME && k8sUtils.IsDevtronApp(res.NetworkingInfo.Labels) {

				res.Health = &bean.HealthStatus{
					Status: bean.HealthStatusHealthy,
				}
			} else {
				if healthCheck := health.GetHealthCheckFunc(gvk); healthCheck != nil {
					health, err := healthCheck(un)
					if err != nil {
						res.Health = &bean.HealthStatus{
							Status:  bean.HealthStatusUnknown,
							Message: err.Error(),
						}
					} else if health != nil {
						res.Health = &bean.HealthStatus{
							Status:  string(health.Status),
							Message: health.Message,
						}
					}
				}
			}
			if k8sUtils.IsPod(gvk) {
				infoItems, _ := argo.PopulatePodInfo(un)
				res.Info = infoItems
			}
			// hibernate set starts
			if un.GetOwnerReferences() == nil {
				// set CanBeHibernated
				replicas, found, _ := unstructured.NestedInt64(un.UnstructuredContent(), "spec", "replicas")
				if found {
					res.CanBeHibernated = true
				}

				// set IsHibernated
				annotations := un.GetAnnotations()
				if annotations != nil {
					if val, ok := annotations[hibernateReplicaAnnotation]; ok {
						if val != "0" && replicas == 0 {
							res.IsHibernated = true
						}
					}
				}
			}
			// hibernate set ends

			return res, false
		}),
		clustercache.SetRetryOptions(clusterCacheAttemptLimit, clusterCacheRetryUseBackoff, isRetryableError),
	}
	return clusterCacheOpts
}

func GetPorts(manifest *unstructured.Unstructured, gvk schema.GroupVersionKind) []int64 {
	ports := make([]int64, 0)
	if k8sUtils.IsService(gvk) || gvk.Kind == "Service" {
		if manifest.Object["spec"] != nil {
			spec := manifest.Object["spec"].(map[string]interface{})
			if spec["ports"] != nil {
				portList := spec["ports"].([]interface{})
				for _, portItem := range portList {
					if portItem.(map[string]interface{}) != nil {
						_portNumber := portItem.(map[string]interface{})["port"]
						portNumber := _portNumber.(int64)
						if portNumber != 0 {
							ports = append(ports, portNumber)
						}
					}
				}
			}
		}
	}
	if manifest.Object["kind"] == "EndpointSlice" {
		if manifest.Object["ports"] != nil {
			endPointsSlicePorts := manifest.Object["ports"].([]interface{})
			for _, val := range endPointsSlicePorts {
				_portNumber := val.(map[string]interface{})["port"]
				portNumber := _portNumber.(int64)
				if portNumber != 0 {
					ports = append(ports, portNumber)
				}
			}
		}
	}
	if gvk.Kind == "Endpoints" {
		if manifest.Object["subsets"] != nil {
			subsets := manifest.Object["subsets"].([]interface{})
			for _, subset := range subsets {
				subsetObj := subset.(map[string]interface{})
				if subsetObj != nil {
					portsIfs := subsetObj["ports"].([]interface{})
					for _, portsIf := range portsIfs {
						portsIfObj := portsIf.(map[string]interface{})
						if portsIfObj != nil {
							port := portsIfObj["port"].(int64)
							ports = append(ports, port)
						}
					}
				}
			}
		}
	}
	return ports
}

func (impl *ClusterCacheImpl) GetClusterCacheByClusterId(clusterId int) (clustercache.ClusterCache, error) {
	impl.rwMutex.RLock()
	defer impl.rwMutex.RUnlock()
	if clusterCache, found := impl.clustersCache[clusterId]; found {
		return clusterCache, nil
	}
	return nil, errors.New("cluster cache not yet synced for this cluster id")
}
