package cache

import (
	"errors"
	clustercache "github.com/argoproj/gitops-engine/pkg/cache"
	"github.com/caarlos0/env"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	"github.com/devtron-labs/kubelink/bean"
	repository "github.com/devtron-labs/kubelink/pkg/cluster"
	"github.com/devtron-labs/kubelink/pkg/k8sInformer"
	"go.uber.org/zap"
	"sync"
)

type ClusterCache interface {
	k8sInformer.ClusterSecretUpdateListener
	GetClusterCacheByClusterId(clusterId int) (clustercache.ClusterCache, error)
}

type ClusterCacheConfig struct {
	ClusterIdList                 []int `env:"CLUSTER_ID_LIST" envSeparator:","`
	ClusterCacheListSemaphoreSize int64 `env:"CLUSTER_CACHE_LIST_SEMAPHORE_SIZE" envDefault:"5"`
	ClusterCacheListPageSize      int64 `env:"CLUSTER_CACHE_LIST_PAGE_SIZE" envDefault:"10"`
	ClusterSyncBatchSize          int   `env:"CLUSTER_SYNC_BATCH_SIZE" envDefault:"2"`
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
	k8sInformer        k8sInformer.K8sInformer
}

func NewClusterCacheImpl(logger *zap.SugaredLogger, clusterCacheConfig *ClusterCacheConfig,
	clusterRepository repository.ClusterRepository, k8sUtil *k8sUtils.K8sUtil, k8sInformer k8sInformer.K8sInformer) *ClusterCacheImpl {

	clustersCache := make(map[int]clustercache.ClusterCache)
	clusterCacheImpl := &ClusterCacheImpl{
		logger:             logger,
		clusterCacheConfig: clusterCacheConfig,
		clusterRepository:  clusterRepository,
		k8sUtil:            k8sUtil,
		clustersCache:      clustersCache,
		k8sInformer:        k8sInformer,
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

func (impl *ClusterCacheImpl) getClusterInfoByClusterId(clusterId int) (*bean.ClusterInfo, error) {
	model, err := impl.clusterRepository.FindById(clusterId)
	if err != nil {
		impl.logger.Errorw("error in getting cluster from db by cluster id", "clusterId", clusterId)
		return nil, err
	}
	clusterInfo := k8sInformer.GetClusterInfo(model)
	return clusterInfo, nil
}

func (impl *ClusterCacheImpl) InvalidateCache(clusterId int) {
	impl.logger.Infow("invalidating cluster cache on cluster secret update/delete", "clusterId", clusterId)
	impl.rwMutex.Lock()
	impl.clustersCache[clusterId].Invalidate()
	impl.rwMutex.Unlock()

	delete(impl.clustersCache, clusterId)
}

func (impl *ClusterCacheImpl) OnStateChange(clusterId int, action string) {
	isValidClusterId := isInClusterIdList(clusterId, impl.clusterCacheConfig.ClusterIdList)
	if !isValidClusterId {
		return
	}
	//invalidate cache first on cluster secrets update
	impl.InvalidateCache(clusterId)
	switch action {
	case k8sInformer.UPDATE:
		clusterInfo, err := impl.getClusterInfoByClusterId(clusterId)
		if err != nil {
			impl.logger.Errorw("error in getting clusterInfo by cluster id", "clusterId", clusterId)
			return
		}
		impl.logger.Infow("syncing cluster cache on cluster config update", "clusterId", clusterId)
		impl.SyncClusterCache(clusterInfo)
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
				clusterInfo, err := impl.getClusterInfoByClusterId(impl.clusterCacheConfig.ClusterIdList[i+j])
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
	impl.clustersCache[clusterInfo.ClusterId] = cache
	impl.rwMutex.Unlock()
	return cache, nil
}

func (impl *ClusterCacheImpl) getClusterCache(clusterInfo *bean.ClusterInfo) (clustercache.ClusterCache, error) {
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
	cache = clustercache.NewClusterCache(restConfig, getClusterCacheOptions(impl.clusterCacheConfig)...)
	return cache, nil
}

func (impl *ClusterCacheImpl) GetClusterCacheByClusterId(clusterId int) (clustercache.ClusterCache, error) {
	impl.rwMutex.RLock()
	defer impl.rwMutex.RUnlock()
	if clusterCache, found := impl.clustersCache[clusterId]; found {
		return clusterCache, nil
	}
	return nil, errors.New("cluster cache not yet synced for this cluster id")
}
