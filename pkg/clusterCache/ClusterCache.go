package clusterCache

import (
	clustercache "github.com/argoproj/gitops-engine/pkg/cache"
	"github.com/caarlos0/env"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	"github.com/devtron-labs/kubelink/bean"
	repository "github.com/devtron-labs/kubelink/pkg/cluster"
	"github.com/devtron-labs/kubelink/pkg/k8sInformer"
	"go.uber.org/zap"
	"golang.org/x/sync/semaphore"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sync"
)

type ClusterCache interface {
	//SyncClusterCache(clusterInfo bean.ClusterInfo, liveStageCache LiveStateCache) (clustercache.ClusterCache, error)
}

type ClusterCacheConfig struct {
	ClusterIdList []int `env:"CLUSTER_ID_LIST" envSeparator:"," envDefault:"1,2,3"`
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
	mutex              sync.Mutex
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

func (impl *ClusterCacheImpl) SyncCache() error {
	for _, clusterId := range impl.clusterCacheConfig.ClusterIdList {
		model, err := impl.clusterRepository.FindById(clusterId)
		if err != nil {
			impl.logger.Errorw("error in getting cluster from db by cluster id", "clusterId", clusterId)
			continue
		}
		clusterInfo := k8sInformer.GetClusterInfo(model)

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
	impl.mutex.Lock()
	impl.clustersCache[clusterInfo.ClusterId] = c
	impl.mutex.Unlock()
	return c, nil
}

func (impl *ClusterCacheImpl) getClusterCache(clusterInfo bean.ClusterInfo) (clustercache.ClusterCache, error) {
	var cache clustercache.ClusterCache
	var ok bool
	cache, ok = impl.clustersCache[clusterInfo.ClusterId]
	if ok {
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
		//clustercache.SetSettings(cacheSettings.clusterSettings),
		//drop this for now,
		//clustercache.SetNamespaces(cluster.Namespaces),
		//clustercache.SetClusterResources(cluster.ClusterResources),
		clustercache.SetPopulateResourceInfoHandler(func(un *unstructured.Unstructured, isRoot bool) (interface{}, bool) {
			//fmt.Println("resource updated")
			//res := &ResourceInfo{}
			//	populateNodeInfo(un, res, resourceCustomLabels)
			//	c.lock.RLock()
			//	cacheSettings := c.cacheSettings
			//	c.lock.RUnlock()
			//
			//	res.Health, _ = health.GetResourceHealth(un, cacheSettings.clusterSettings.ResourceHealthOverride)
			//
			//	appName := c.resourceTracking.GetAppName(un, cacheSettings.appInstanceLabelKey, cacheSettings.trackingMethod)
			//	if isRoot && appName != "" {
			//		res.AppName = appName
			//	}
			//
			//	gvk := un.GroupVersionKind()
			//
			//	if cacheSettings.ignoreResourceUpdatesEnabled && shouldHashManifest(appName, gvk) {
			//		hash, err := generateManifestHash(un, nil, cacheSettings.resourceOverrides)
			//		if err != nil {
			//			log.Errorf("Failed to generate manifest hash: %v", err)
			//		} else {
			//			res.manifestHash = hash
			//		}
			//	}
			//
			//	// edge case. we do not label CRDs, so they miss the tracking label we inject. But we still
			//	// want the full resource to be available in our cache (to diff), so we store all CRDs
			//	return res, res.AppName != "" || gvk.Kind == kube.CustomResourceDefinitionKind
			return nil, true
		}),
		clustercache.SetRetryOptions(clusterCacheAttemptLimit, clusterCacheRetryUseBackoff, isRetryableError),
	}
	return clusterCacheOpts
}
