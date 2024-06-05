package fluxInformer

import (
	"errors"
	"fmt"
	"github.com/caarlos0/env"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	"github.com/devtron-labs/kubelink/bean"
	"github.com/devtron-labs/kubelink/converter"
	repository "github.com/devtron-labs/kubelink/pkg/cluster"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"strconv"
	"sync"
	"time"
)

const (
	DEFAULT_CLUSTER                = "default_cluster"
	INFORMER_ALREADY_EXIST_MESSAGE = "INFORMER_ALREADY_EXIST"
)

// Define GroupVersionResource for Kustomization and HelmRelease
var (
	gvrKustomization = schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}
	gvrHelmRelease = schema.GroupVersionResource{
		Group:    "helm.toolkit.fluxcd.io",
		Version:  "v2beta1",
		Resource: "helmreleases",
	}
)

type FluxInformer interface {
	startFluxInformer(clusterInfo bean.ClusterInfo) error
	syncFluxInformer(clusterId int) error
	stopFluxInformer(clusterName string, clusterId int)
	startFluxInformerAndPopulateCache(clusterId int) error
	GetAllFluxAppByClusterId(clusterId int) []*unstructured.Unstructured
	CheckFluxAppExists(clusterId int32, fluxAppIdentifier string) bool
	GetClusterDynamicClientSet(clusterInfo bean.ClusterInfo) (*dynamic.DynamicClient, error)
}

type FluxAppConfig struct {
	EnableFluxAppCache bool `env:"ENABLE_FLUX_APP_CACHE" envDefault:"true"`
}

func GetFluxAppConfig() (*FluxAppConfig, error) {
	cfg := &FluxAppConfig{}
	err := env.Parse(cfg)
	return cfg, err
}

type FluxInformerImpl struct {
	logger                *zap.SugaredLogger
	FluxAppListClusterMap map[int]map[string]*unstructured.Unstructured
	mutex                 sync.Mutex
	informerStopper       map[int]chan struct{}
	clusterRepository     repository.ClusterRepository
	fluxAppConfig         *FluxAppConfig
	k8sUtil               k8sUtils.K8sService
	converter             converter.ClusterBeanConverter
}

func NewFluxInformerImpl(logger *zap.SugaredLogger, clusterRepository repository.ClusterRepository,
	fluxAppConfig *FluxAppConfig, k8sUtil k8sUtils.K8sService, converter converter.ClusterBeanConverter) *FluxInformerImpl {
	informerFactory := &FluxInformerImpl{
		logger:            logger,
		clusterRepository: clusterRepository,
		fluxAppConfig:     fluxAppConfig,
		k8sUtil:           k8sUtil,
		converter:         converter,
	}
	informerFactory.FluxAppListClusterMap = make(map[int]map[string]*unstructured.Unstructured)
	informerFactory.informerStopper = make(map[int]chan struct{})
	if fluxAppConfig.EnableFluxAppCache {
		go func() {
			err := informerFactory.BuildFluxInformerForAllClusters()
			if err != nil {
				logger.Errorw("err", err)
			}
		}()
	}
	return informerFactory
}

func (impl *FluxInformerImpl) BuildFluxInformerForAllClusters() error {
	models, err := impl.clusterRepository.FindAllActive()
	if err != nil {
		impl.logger.Error("error in fetching clusters", "err", err)
		return err
	}

	if len(models) == 0 {
		clusterInfo := &bean.ClusterInfo{
			ClusterId:             1,
			ClusterName:           DEFAULT_CLUSTER,
			InsecureSkipTLSVerify: true,
		}
		err := impl.startFluxInformer(*clusterInfo)
		if err != nil {
			impl.logger.Error("error in starting informer for cluster ", "cluster-name ", clusterInfo.ClusterName, "err", err)
			return err
		}
		return nil
	}

	for _, model := range models {
		clusterInfo := impl.converter.GetClusterInfo(model)
		err := impl.startFluxInformer(*clusterInfo)
		if err != nil {
			impl.logger.Error("error in starting Flux informer for cluster ", "cluster-name ", clusterInfo.ClusterName, "err", err)
			return err
		}

	}
	return nil
}
func (impl *FluxInformerImpl) startFluxInformer(clusterInfo bean.ClusterInfo) error {
	clusterDynamicClient, err := impl.GetClusterDynamicClientSet(clusterInfo)
	if err != nil {
		impl.logger.Error("error in GetClusterClientSet", "clusterName", clusterInfo.ClusterName, "err", err)
		return err
	}

	if clusterInfo.ClusterName == DEFAULT_CLUSTER {
		impl.logger.Debug("Starting informer, reading new cluster request for default cluster")
		informerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(clusterDynamicClient, 15*time.Minute, metav1.NamespaceAll, nil)
		stopCh := make(chan struct{})

		setupInformer := func(gvr schema.GroupVersionResource) {
			informer := informerFactory.ForResource(gvr).Informer()
			informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					impl.handleAdd(obj, clusterInfo.ClusterId, gvr)
				},
				UpdateFunc: func(oldObj, newObj interface{}) {
					impl.handleUpdate(oldObj, newObj, clusterInfo.ClusterId, gvr)
				},
				DeleteFunc: func(obj interface{}) {
					impl.handleDelete(obj, clusterInfo.ClusterId, gvr)
				},
			})
			go informer.Run(stopCh)
		}

		setupInformer(gvrKustomization)
		setupInformer(gvrHelmRelease)

		impl.informerStopper[clusterInfo.ClusterId] = stopCh
	}

	err = impl.startFluxInformerAndPopulateCache(clusterInfo.ClusterId)
	if err != nil && err.Error() != INFORMER_ALREADY_EXIST_MESSAGE {
		impl.logger.Error("error in creating flux informer for new cluster", "err", err)
		return err
	}

	return nil
}
func (impl *FluxInformerImpl) syncFluxInformer(clusterId int) error {

	clusterInfo, err := impl.clusterRepository.FindById(clusterId)
	if err != nil {
		impl.logger.Error("error in fetching cluster info by id", "err", err)
		return err
	}
	//before creating new informer for cluster, close existing one
	impl.logger.Debug("stopping informer for cluster - ", "cluster-name", clusterInfo.ClusterName, "cluster-id", clusterInfo.Id)
	impl.stopFluxInformer(clusterInfo.ClusterName, clusterInfo.Id)
	impl.logger.Debug("informer stopped", "cluster-name", clusterInfo.ClusterName, "cluster-id", clusterInfo.Id)
	//create new informer for cluster with new config
	err = impl.startFluxInformerAndPopulateCache(clusterId)
	if err != nil {
		impl.logger.Error("error in starting informer for ", "cluster name", clusterInfo.ClusterName)
		return err
	}
	return nil
}
func (impl *FluxInformerImpl) stopFluxInformer(clusterName string, clusterId int) {
	stopper, ok := impl.informerStopper[clusterId]
	if ok {
		close(stopper)
		delete(impl.informerStopper, clusterId)
	}
	return
}
func (impl *FluxInformerImpl) GetClusterDynamicClientSet(clusterInfo bean.ClusterInfo) (*dynamic.DynamicClient, error) {
	clusterConfig := impl.converter.GetClusterConfig(&clusterInfo)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(clusterConfig)
	if err != nil {
		impl.logger.Error("error in getting rest config", "err", err, "clusterName", clusterConfig.ClusterName)
		return nil, err
	}
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		impl.logger.Error("error in create dynamic client", "err", err)
		return nil, err
	}
	return dynamicClient, nil
}
func (impl *FluxInformerImpl) startFluxInformerAndPopulateCache(clusterId int) error {
	clusterModel, err := impl.clusterRepository.FindById(clusterId)
	if err != nil {
		impl.logger.Error("error in fetching cluster by cluster ids")
		return err
	}

	if _, ok := impl.informerStopper[clusterId]; ok {
		impl.logger.Debug(fmt.Sprintf("informer for %s already exist", clusterModel.ClusterName))
		return errors.New(INFORMER_ALREADY_EXIST_MESSAGE)
	}

	impl.logger.Info("starting informer for cluster - ", "cluster-id ", clusterModel.Id, "cluster-name ", clusterModel.ClusterName)

	clusterInfo := impl.converter.GetClusterInfo(clusterModel)
	clusterConfig := impl.converter.GetClusterConfig(clusterInfo)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(clusterConfig)
	if err != nil {
		impl.logger.Error("error in getting rest config", "err", err, "clusterName", clusterConfig.ClusterName)
		return err
	}

	clusterDynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		impl.logger.Error("error in create k8s config", "err", err)
		return err
	}

	impl.mutex.Lock()
	impl.FluxAppListClusterMap[clusterId] = make(map[string]*unstructured.Unstructured)
	impl.mutex.Unlock()

	informerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(clusterDynamicClient, 15*time.Minute, metav1.NamespaceAll, nil)
	stopper := make(chan struct{})

	setupInformer := func(gvr schema.GroupVersionResource) {
		informer := informerFactory.ForResource(gvr).Informer()
		informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				impl.handleAdd(obj, clusterId, gvr)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				impl.handleUpdate(oldObj, newObj, clusterInfo.ClusterId, gvr)

			},
			DeleteFunc: func(obj interface{}) {
				impl.handleDelete(obj, clusterId, gvr)
			},
		})
		informer.Run(stopper)
	}
	setupInformer(gvrKustomization)
	setupInformer(gvrHelmRelease)

	impl.logger.Info("informer started for cluster: ", "cluster_id", clusterModel.Id, "cluster_name", clusterModel.ClusterName)
	impl.informerStopper[clusterId] = stopper
	return nil
}
func (impl *FluxInformerImpl) getUniqueFluxAppKey(fluxApp *FluxAppDto, clusterId int) string {
	return fluxApp.getUniqueAppIdentifier() + "_" + strconv.Itoa(clusterId)
}
func (impl *FluxInformerImpl) GetAllFluxAppByClusterId(clusterId int) []*unstructured.Unstructured {

	var FLuxAppDetailList []*unstructured.Unstructured
	AppsMap := impl.FluxAppListClusterMap[clusterId]
	for _, v := range AppsMap {
		FLuxAppDetailList = append(FLuxAppDetailList, v)
	}
	return FLuxAppDetailList
}
func (impl *FluxInformerImpl) CheckFluxAppExists(clusterId int32, fluxAppIdentifier string) bool {
	FluxAppMap := impl.FluxAppListClusterMap[int(clusterId)]
	_, ok := FluxAppMap[fluxAppIdentifier]
	if ok {
		return true
	}
	return false
}
func (impl *FluxInformerImpl) handleAdd(obj interface{}, clusterId int, gvr schema.GroupVersionResource) {
	impl.logger.Debug("Resource added", "resource", gvr.Resource, "time", time.Now())
	if unstructuredObj, ok := obj.(*unstructured.Unstructured); ok {
		impl.mutex.Lock()
		defer impl.mutex.Unlock()
		impl.FluxAppListClusterMap[clusterId][impl.getUniqueFluxAppKey(&FluxAppDto{unstructuredObj}, clusterId)] = unstructuredObj
	}
}
func (impl *FluxInformerImpl) handleUpdate(oldObj, newObj interface{}, clusterId int, gvr schema.GroupVersionResource) {
	impl.logger.Debug("Resource updated", "resource", gvr.Resource, "time", time.Now())
	if unstructuredObj, ok := newObj.(*unstructured.Unstructured); ok {
		impl.mutex.Lock()
		defer impl.mutex.Unlock()
		impl.FluxAppListClusterMap[clusterId][impl.getUniqueFluxAppKey(&FluxAppDto{oldObj.(*unstructured.Unstructured)}, clusterId)] = unstructuredObj
	}
}
func (impl *FluxInformerImpl) handleDelete(obj interface{}, clusterId int, gvr schema.GroupVersionResource) {
	impl.logger.Debug("Resource deleted", "resource", gvr.Resource, "time", time.Now())
	if unstructuredObj, ok := obj.(*unstructured.Unstructured); ok {
		impl.mutex.Lock()
		defer impl.mutex.Unlock()
		//just for keeping this
		delete(impl.FluxAppListClusterMap[clusterId], impl.getUniqueFluxAppKey(&FluxAppDto{unstructuredObj}, clusterId))
	}
}
