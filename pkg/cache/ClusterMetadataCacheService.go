package cache

import (
	"errors"
	clustercache "github.com/argoproj/gitops-engine/pkg/cache"
	"github.com/caarlos0/env"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	"github.com/devtron-labs/kubelink/bean"
	"github.com/devtron-labs/kubelink/converter"
	repository "github.com/devtron-labs/kubelink/pkg/cluster"
	"github.com/devtron-labs/kubelink/pkg/k8sInformer"
	"go.uber.org/zap"
	"sync"
	"time"
)

type ClusterCache interface {
	k8sInformer.ClusterSecretUpdateListener
	GetClusterCacheByClusterId(clusterId int) (clustercache.ClusterCache, error)
}

type ClusterCacheInfo struct {
	ClusterCache             clustercache.ClusterCache
	ClusterCacheLastSyncTime time.Time
}

type ClusterCacheConfig struct {
	ClusterIdList                 []int `env:"CLUSTER_ID_LIST" envSeparator:","`
	ClusterCacheListSemaphoreSize int64 `env:"CLUSTER_CACHE_LIST_SEMAPHORE_SIZE" envDefault:"2"`
	ClusterCacheListPageSize      int64 `env:"CLUSTER_CACHE_LIST_PAGE_SIZE" envDefault:"5"`
	ClusterSyncBatchSize          int   `env:"CLUSTER_SYNC_BATCH_SIZE" envDefault:"1"`
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
	k8sUtil            k8sUtils.K8sUtilIf
	clustersCache      map[int]ClusterCacheInfo
	rwMutex            sync.RWMutex
	k8sInformer        k8sInformer.K8sInformer
	converter          converter.ClusterBeanConverter
}

func NewClusterCacheImpl(logger *zap.SugaredLogger, clusterCacheConfig *ClusterCacheConfig,
	clusterRepository repository.ClusterRepository, k8sUtil k8sUtils.K8sUtilIf, k8sInformer k8sInformer.K8sInformer,
	converter converter.ClusterBeanConverter) *ClusterCacheImpl {

	clustersCache := make(map[int]ClusterCacheInfo)
	clusterCacheImpl := &ClusterCacheImpl{
		logger:             logger,
		clusterCacheConfig: clusterCacheConfig,
		clusterRepository:  clusterRepository,
		k8sUtil:            k8sUtil,
		clustersCache:      clustersCache,
		k8sInformer:        k8sInformer,
		converter:          converter,
	}

	if len(clusterCacheConfig.ClusterIdList) > 0 {
		k8sInformer.RegisterListener(clusterCacheImpl)
		err := clusterCacheImpl.SyncCache()
		if err != nil {
			return nil
		}
	}
	return clusterCacheImpl
}

func (impl *ClusterCacheImpl) getClusterInfoByClusterId(clusterId int) (*bean.ClusterInfo, time.Time, error) {
	model, err := impl.clusterRepository.FindById(clusterId)
	if err != nil {
		impl.logger.Errorw("error in getting cluster from db by cluster id", "clusterId", clusterId)
		return nil, time.Time{}, err
	}
	clusterInfo := impl.converter.GetClusterInfo(model)
	return clusterInfo, model.UpdatedOn, nil
}

func (impl *ClusterCacheImpl) InvalidateCache(clusterId int) {
	impl.rwMutex.Lock()
	defer impl.rwMutex.Unlock()

	// Check if the cache entry exists before invalidating and deleting
	if cacheEntry, ok := impl.clustersCache[clusterId]; ok {
		cacheEntry.ClusterCache.Invalidate()
		delete(impl.clustersCache, clusterId)
	}
}

func (impl *ClusterCacheImpl) OnStateChange(clusterId int, action string) {
	isValidClusterId := isInClusterIdList(clusterId, impl.clusterCacheConfig.ClusterIdList)
	if !isValidClusterId {
		return
	}
	switch action {
	case k8sInformer.UPDATE:
		clusterInfo, lastUpdatedOn, err := impl.getClusterInfoByClusterId(clusterId)
		if err != nil {
			impl.logger.Errorw("error in getting clusterInfo by cluster id", "clusterId", clusterId)
			return
		}
		if lastUpdatedOn.Before(impl.clustersCache[clusterId].ClusterCacheLastSyncTime) {
			impl.logger.Debugw("last update for cluster secrets was before cluster cache sync time hence not syncing cluster cache again")
			return
		}
		//invalidate cache first before cache sync
		impl.logger.Infow("invalidating cache before syncing again on cluster config update event", "clusterId", clusterId)
		impl.InvalidateCache(clusterId)
		impl.logger.Infow("syncing cluster cache on cluster config update", "clusterId", clusterId)
		impl.SyncClusterCache(clusterInfo)
	case k8sInformer.DELETE:
		impl.logger.Infow("invalidating cache before syncing again on cluster config delete event", "clusterId", clusterId)
		impl.InvalidateCache(clusterId)
	}
}

func (impl *ClusterCacheImpl) SyncCache() error {
	requestsLength := len(impl.clusterCacheConfig.ClusterIdList)
	batchSize := impl.clusterCacheConfig.ClusterSyncBatchSize
	for i := 0; i < requestsLength; {
		//requests left to process
		remainingBatch := requestsLength - i
		if remainingBatch < batchSize {
			batchSize = remainingBatch
		}
		var wg sync.WaitGroup
		for j := 0; j < batchSize; j++ {
			wg.Add(1)
			go func(j int) {
				defer wg.Done()
				clusterInfo, _, err := impl.getClusterInfoByClusterId(impl.clusterCacheConfig.ClusterIdList[i+j])
				if err != nil {
					impl.logger.Errorw("error in getting clusterInfo by cluster id", "clusterId", impl.clusterCacheConfig.ClusterIdList[i+j])
					return
				}
				impl.SyncClusterCache(clusterInfo)
			}(j)
		}
		wg.Wait()
		i += batchSize
	}
	return nil
}

func (impl *ClusterCacheImpl) SyncClusterCache(clusterInfo *bean.ClusterInfo) (clustercache.ClusterCache, error) {
	impl.logger.Infow("cluster cache sync started..", "clusterId", clusterInfo.ClusterId)
	cache, err := impl.getClusterCache(clusterInfo)
	if err != nil {
		impl.logger.Errorw("failed to get cluster info for", "clusterId", clusterInfo.ClusterId, "error", err)
		return cache, err
	}
	err = cache.EnsureSynced()
	if err != nil {
		impl.logger.Errorw("error in syncing cluster cache", "clusterId", clusterInfo.ClusterId, "sync-error", err)
		return cache, err
	}
	impl.rwMutex.Lock()
	impl.clustersCache[clusterInfo.ClusterId] = ClusterCacheInfo{
		ClusterCache:             cache,
		ClusterCacheLastSyncTime: time.Now(),
	}
	impl.rwMutex.Unlock()
	return cache, nil
}

func (impl *ClusterCacheImpl) getClusterCache(clusterInfo *bean.ClusterInfo) (clustercache.ClusterCache, error) {
	cache, err := impl.GetClusterCacheByClusterId(clusterInfo.ClusterId)
	if err == nil {
		return cache, nil
	}
	clusterConfig := impl.converter.GetClusterConfig(clusterInfo)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(clusterConfig)
	if err != nil {
		impl.logger.Errorw("error in getting rest config", "err", err, "clusterName", clusterConfig.ClusterName)
		return cache, err
	}
	cache = clustercache.NewClusterCache(restConfig, getClusterCacheOptions(impl.clusterCacheConfig)...)
	return cache, nil
}

func (impl *ClusterCacheImpl) GetClusterCacheByClusterId(clusterId int) (clustercache.ClusterCache, error) {
	impl.rwMutex.RLock()
	defer impl.rwMutex.RUnlock()
	if clusterCacheInfo, found := impl.clustersCache[clusterId]; found {
		return clusterCacheInfo.ClusterCache, nil
	}
	return nil, errors.New("cluster cache not yet synced for this cluster id")
}
