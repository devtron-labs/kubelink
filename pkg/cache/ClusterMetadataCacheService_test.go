package cache

import (
	clustercache "github.com/argoproj/gitops-engine/pkg/cache"
	client2 "github.com/devtron-labs/authenticator/client"
	"github.com/devtron-labs/common-lib/utils"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	repository "github.com/devtron-labs/kubelink/pkg/cluster"
	"github.com/devtron-labs/kubelink/pkg/sql"
	"github.com/go-pg/pg"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"sync"
	"testing"
)

func getDbConnAndLoggerService(t *testing.T) (*zap.SugaredLogger, *pg.DB) {
	cfg, _ := sql.GetConfig()
	logger, err := utils.NewSugardLogger()
	assert.Nil(t, err)
	dbConnection, err := sql.NewDbConnection(cfg, logger)
	assert.Nil(t, err)

	return logger, dbConnection
}

func TestHelmAppServiceImpl_SyncCache(t *testing.T) {
	t.Run("test1", func(t *testing.T) {
		logger, dbConnection := getDbConnAndLoggerService(t)
		runTimeConfig, _ := client2.GetRuntimeConfig()
		k8sUtil := k8sUtils.NewK8sUtil(logger, runTimeConfig)
		clusterCacheConfig, err := GetClusterCacheConfig()
		assert.Nil(t, err)
		impl := &ClusterCacheImpl{
			logger:             logger,
			clusterCacheConfig: clusterCacheConfig,
			clusterRepository:  repository.NewClusterRepositoryImpl(dbConnection, logger),
			k8sUtil:            k8sUtil,
			clustersCache:      make(map[int]clustercache.ClusterCache),
			k8sInformer:        nil,
			rwMutex:            sync.RWMutex{},
		}
		err = impl.SyncCache()
		assert.Nil(t, err)
	})

}
