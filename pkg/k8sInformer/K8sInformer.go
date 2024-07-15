/*
 * Copyright (c) 2024. Devtron Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package k8sInformer

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	k8sUtils "github.com/devtron-labs/common-lib/utils/k8s"
	"github.com/devtron-labs/kubelink/bean"
	globalConfig "github.com/devtron-labs/kubelink/config"
	"github.com/devtron-labs/kubelink/converter"
	client "github.com/devtron-labs/kubelink/grpc"
	repository "github.com/devtron-labs/kubelink/pkg/cluster"
	"github.com/devtron-labs/kubelink/pkg/util"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
	"helm.sh/helm/v3/pkg/release"
	"io/ioutil"
	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"log"
	"reflect"
	"strconv"
	"sync"
	"time"
)

const (
	HELM_RELEASE_SECRET_TYPE         = "helm.sh/release.v1"
	CLUSTER_MODIFY_EVENT_SECRET_TYPE = "cluster.request/modify"
	DEFAULT_CLUSTER                  = "default_cluster"
	INFORMER_ALREADY_EXIST_MESSAGE   = "INFORMER_ALREADY_EXIST"
	ADD                              = "add"
	UPDATE                           = "update"
	DELETE                           = "delete"
)

type K8sInformer interface {
	startInformer(clusterInfo bean.ClusterInfo) error
	syncInformer(clusterId int) error
	stopInformer(clusterName string, clusterId int)
	startInformerAndPopulateCache(clusterId int) error
	GetAllReleaseByClusterId(clusterId int) []*client.DeployedAppDetail
	CheckReleaseExists(clusterId int32, releaseIdentifier string) bool
	GetClusterClientSet(clusterInfo bean.ClusterInfo) (*kubernetes.Clientset, error)
	RegisterListener(listener ClusterSecretUpdateListener)
}

type ClusterSecretUpdateListener interface {
	OnStateChange(clusterId int, action string)
}

type K8sInformerImpl struct {
	logger             *zap.SugaredLogger
	HelmListClusterMap map[int]map[string]*client.DeployedAppDetail
	mutex              sync.Mutex
	informerStopper    map[int]chan struct{}
	clusterRepository  repository.ClusterRepository
	helmReleaseConfig  *globalConfig.HelmReleaseConfig
	k8sUtil            k8sUtils.K8sService
	listeners          []ClusterSecretUpdateListener
	converter          converter.ClusterBeanConverter
}

func Newk8sInformerImpl(logger *zap.SugaredLogger, clusterRepository repository.ClusterRepository,
	helmReleaseConfig *globalConfig.HelmReleaseConfig, k8sUtil k8sUtils.K8sService, converter converter.ClusterBeanConverter) *K8sInformerImpl {
	informerFactory := &K8sInformerImpl{
		logger:            logger,
		clusterRepository: clusterRepository,
		helmReleaseConfig: helmReleaseConfig,
		k8sUtil:           k8sUtil,
		converter:         converter,
	}
	informerFactory.HelmListClusterMap = make(map[int]map[string]*client.DeployedAppDetail)
	informerFactory.informerStopper = make(map[int]chan struct{})
	if helmReleaseConfig.EnableHelmReleaseCache {
		go func() {
			err := informerFactory.BuildInformerForAllClusters()
			if err != nil {
				// restarting kubelink service
				log.Panic(err)
			}
		}()
	}
	return informerFactory
}

func (impl *K8sInformerImpl) OnStateChange(clusterId int, action string) {
	impl.logger.Infow("syncing informer on cluster config update/delete", "action", action, "clusterId", clusterId)
	switch action {
	case UPDATE:
		err := impl.syncInformer(clusterId)
		if err != nil && err != errors.New(INFORMER_ALREADY_EXIST_MESSAGE) {
			impl.logger.Error("error in updating informer for cluster", "id", clusterId, "err", err)
			return
		}
	case DELETE:
		deleteClusterInfo, err := impl.clusterRepository.FindByIdWithActiveFalse(clusterId)
		if err != nil {
			impl.logger.Error("Error in fetching cluster by id", "cluster-id ", clusterId)
			return
		}
		impl.stopInformer(deleteClusterInfo.ClusterName, deleteClusterInfo.Id)
		if err != nil {
			impl.logger.Error("error in updating informer for cluster", "id", clusterId, "err", err)
			return
		}
	}
}

func (impl *K8sInformerImpl) RegisterListener(listener ClusterSecretUpdateListener) {
	impl.logger.Infof("registering listener %s", reflect.TypeOf(listener))
	impl.listeners = append(impl.listeners, listener)
}

func decodeRelease(data string) (*release.Release, error) {
	// base64 decode string
	b64 := base64.StdEncoding
	b, err := b64.DecodeString(data)
	if err != nil {
		return nil, err
	}

	var magicGzip = []byte{0x1f, 0x8b, 0x08}

	// For backwards compatibility with releases that were stored before
	// compression was introduced we skip decompression if the
	// gzip magic header is not found
	if len(b) > 3 && bytes.Equal(b[0:3], magicGzip) {
		r, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		b2, err := ioutil.ReadAll(r)
		if err != nil {
			return nil, err
		}
		b = b2
	}

	var rls release.Release
	// unmarshal release object bytes
	if err := json.Unmarshal(b, &rls); err != nil {
		return nil, err
	}
	return &rls, nil
}

func (impl *K8sInformerImpl) BuildInformerForAllClusters() error {
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
		err := impl.startInformer(*clusterInfo)
		if err != nil {
			impl.logger.Error("error in starting informer for cluster ", "cluster-name ", clusterInfo.ClusterName, "err", err)
			return err
		}
		return nil
	}

	for _, model := range models {
		clusterInfo := impl.converter.GetClusterInfo(model)
		err := impl.startInformer(*clusterInfo)
		if err != nil {
			impl.logger.Error("error in starting informer for cluster ", "cluster-name ", clusterInfo.ClusterName, "err", err)
			return err
		}

	}

	return nil
}

func (impl *K8sInformerImpl) GetClusterClientSet(clusterInfo bean.ClusterInfo) (*kubernetes.Clientset, error) {
	clusterConfig := impl.converter.GetClusterConfig(&clusterInfo)
	restConfig, err := impl.k8sUtil.GetRestConfigByCluster(clusterConfig)
	if err != nil {
		impl.logger.Error("error in getting rest config", "err", err, "clusterName", clusterConfig.ClusterName)
		return nil, err
	}
	httpClientFor, err := rest.HTTPClientFor(restConfig)
	if err != nil {
		impl.logger.Error("error occurred while overriding k8s client", "reason", err)
		return nil, err
	}
	clusterClient, err := kubernetes.NewForConfigAndClient(restConfig, httpClientFor)
	if err != nil {
		impl.logger.Error("error in create k8s config", "err", err)
		return nil, err
	}
	return clusterClient, nil
}

func (impl *K8sInformerImpl) startInformer(clusterInfo bean.ClusterInfo) error {
	clusterClient, err := impl.GetClusterClientSet(clusterInfo)
	if err != nil {
		impl.logger.Error("error in GetClusterClientSet", "clusterName", clusterInfo.ClusterName, "err", err)
		return err
	}

	// for default cluster adding an extra informer, this informer will add informer on new clusters
	if clusterInfo.ClusterName == DEFAULT_CLUSTER {
		impl.logger.Debug("Starting informer, reading new cluster request for default cluster")
		labelOptions := kubeinformers.WithTweakListOptions(func(opts *metav1.ListOptions) {
			//kubectl  get  secret --field-selector type==cluster.request/modify --all-namespaces
			opts.FieldSelector = "type==cluster.request/modify"
		})
		informerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(clusterClient, 15*time.Minute, labelOptions)
		stopper := make(chan struct{})
		secretInformer := informerFactory.Core().V1().Secrets()
		secretInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				impl.logger.Debug("Event received in cluster secret Add informer", "time", time.Now())
				if secretObject, ok := obj.(*coreV1.Secret); ok {
					if secretObject.Type != CLUSTER_MODIFY_EVENT_SECRET_TYPE {
						return
					}
					data := secretObject.Data
					action := data["action"]
					id := string(data["cluster_id"])
					id_int, _ := strconv.Atoi(id)

					if string(action) == ADD {
						err = impl.startInformerAndPopulateCache(id_int)
						if err != nil && err != errors.New(INFORMER_ALREADY_EXIST_MESSAGE) {
							impl.logger.Debug("error in adding informer for cluster", "id", id_int, "err", err)
							return
						}
					}
					if string(action) == UPDATE {
						err = impl.syncInformer(id_int)
						if err != nil && err != errors.New(INFORMER_ALREADY_EXIST_MESSAGE) {
							impl.logger.Debug("error in updating informer for cluster", "id", clusterInfo.ClusterId, "name", clusterInfo.ClusterName, "err", err)
							return
						}
					}
				}
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				impl.logger.Debug("Event received in cluster secret update informer", "time", time.Now())
				if secretObject, ok := newObj.(*coreV1.Secret); ok {
					if secretObject.Type != CLUSTER_MODIFY_EVENT_SECRET_TYPE {
						return
					}
					data := secretObject.Data
					action := data["action"]
					id := string(data["cluster_id"])
					id_int, _ := strconv.Atoi(id)

					if string(action) == ADD {
						err = impl.startInformerAndPopulateCache(clusterInfo.ClusterId)
						if err != nil && err != errors.New(INFORMER_ALREADY_EXIST_MESSAGE) {
							impl.logger.Error("error in adding informer for cluster", "id", id_int, "err", err)
							return
						}
					}
					if string(action) == UPDATE {
						impl.OnStateChange(id_int, string(action))
						impl.informOtherListeners(id_int, string(action))
					}
				}
			},
			DeleteFunc: func(obj interface{}) {
				impl.logger.Debug("Event received in secret delete informer", "time", time.Now())
				if secretObject, ok := obj.(*coreV1.Secret); ok {
					if secretObject.Type != CLUSTER_MODIFY_EVENT_SECRET_TYPE {
						return
					}
					data := secretObject.Data
					action := data["action"]
					id := string(data["cluster_id"])
					id_int, _ := strconv.Atoi(id)

					if string(action) == DELETE {
						impl.OnStateChange(id_int, string(action))
						impl.informOtherListeners(id_int, string(action))
					}
				}
			},
		})
		informerFactory.Start(stopper)
		//impl.informerStopper[clusterInfo.ClusterName+"_second_informer"] = stopper

	}
	// these informers will be used to populate helm release cache

	err = impl.startInformerAndPopulateCache(clusterInfo.ClusterId)
	if err != nil && err != errors.New(INFORMER_ALREADY_EXIST_MESSAGE) {
		impl.logger.Error("error in creating informer for new cluster", "err", err)
		return err
	}

	return nil
}

func (impl *K8sInformerImpl) informOtherListeners(clusterId int, action string) {
	for _, listener := range impl.listeners {
		listener.OnStateChange(clusterId, action)
	}
}

func (impl *K8sInformerImpl) syncInformer(clusterId int) error {

	clusterInfo, err := impl.clusterRepository.FindById(clusterId)
	if err != nil {
		impl.logger.Error("error in fetching cluster info by id", "err", err)
		return err
	}
	//before creating new informer for cluster, close existing one
	impl.logger.Debug("stopping informer for cluster - ", "cluster-name", clusterInfo.ClusterName, "cluster-id", clusterInfo.Id)
	impl.stopInformer(clusterInfo.ClusterName, clusterInfo.Id)
	impl.logger.Debug("informer stopped", "cluster-name", clusterInfo.ClusterName, "cluster-id", clusterInfo.Id)
	//create new informer for cluster with new config
	err = impl.startInformerAndPopulateCache(clusterId)
	if err != nil {
		impl.logger.Error("error in starting informer for ", "cluster name", clusterInfo.ClusterName)
		return err
	}
	return nil
}

func (impl *K8sInformerImpl) stopInformer(clusterName string, clusterId int) {
	stopper := impl.informerStopper[clusterId]
	if stopper != nil {
		close(stopper)
		delete(impl.informerStopper, clusterId)
	}
	return
}

func (impl *K8sInformerImpl) startInformerAndPopulateCache(clusterId int) error {

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
	httpClientFor, err := rest.HTTPClientFor(restConfig)
	if err != nil {
		impl.logger.Error("error occurred while overriding k8s client", "reason", err)
		return err
	}
	clusterClient, err := kubernetes.NewForConfigAndClient(restConfig, httpClientFor)
	if err != nil {
		impl.logger.Error("error in create k8s config", "err", err)
		return err
	}

	impl.mutex.Lock()
	impl.HelmListClusterMap[clusterId] = make(map[string]*client.DeployedAppDetail)
	impl.mutex.Unlock()

	labelOptions := kubeinformers.WithTweakListOptions(func(opts *metav1.ListOptions) {
		//kubectl  get  secret --field-selector type==helm.sh/release.v1 -l status=deployed  --all-namespaces
		opts.LabelSelector = "status!=superseded"
		opts.FieldSelector = "type==helm.sh/release.v1"
	})
	informerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(clusterClient, 15*time.Minute, labelOptions)
	stopper := make(chan struct{})
	secretInformer := informerFactory.Core().V1().Secrets()
	secretInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			impl.logger.Debug("Event received in Helm secret add informer", "time", time.Now())
			if secretObject, ok := obj.(*coreV1.Secret); ok {

				if secretObject.Type != HELM_RELEASE_SECRET_TYPE {
					return
				}
				releaseDTO, err := decodeRelease(string(secretObject.Data["release"]))
				if err != nil {
					impl.logger.Error("error in decoding release")
				}
				appDetail := &client.DeployedAppDetail{
					AppId:        util.GetAppId(int32(clusterModel.Id), releaseDTO),
					AppName:      releaseDTO.Name,
					ChartName:    releaseDTO.Chart.Name(),
					ChartAvatar:  releaseDTO.Chart.Metadata.Icon,
					LastDeployed: timestamppb.New(releaseDTO.Info.LastDeployed.Time),
					EnvironmentDetail: &client.EnvironmentDetails{
						ClusterId:   int32(clusterModel.Id),
						ClusterName: clusterModel.ClusterName,
						Namespace:   releaseDTO.Namespace,
					},
				}
				impl.mutex.Lock()
				defer impl.mutex.Unlock()
				impl.HelmListClusterMap[clusterId][impl.getUniqueReleaseKey(&ReleaseDto{releaseDTO}, clusterModel.Id)] = appDetail
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			impl.logger.Debug("Event received in Helm secret update informer", "time", time.Now())
			if secretObject, ok := oldObj.(*coreV1.Secret); ok {
				if secretObject.Type != HELM_RELEASE_SECRET_TYPE {
					return
				}
				releaseDTO, err := decodeRelease(string(secretObject.Data["release"]))
				if err != nil {
					impl.logger.Error("error in decoding release")
				}
				appDetail := &client.DeployedAppDetail{
					AppId:        util.GetAppId(int32(clusterModel.Id), releaseDTO),
					AppName:      releaseDTO.Name,
					ChartName:    releaseDTO.Chart.Name(),
					ChartAvatar:  releaseDTO.Chart.Metadata.Icon,
					LastDeployed: timestamppb.New(releaseDTO.Info.LastDeployed.Time),
					EnvironmentDetail: &client.EnvironmentDetails{
						ClusterId:   int32(clusterModel.Id),
						ClusterName: clusterModel.ClusterName,
						Namespace:   releaseDTO.Namespace,
					},
				}
				impl.mutex.Lock()
				defer impl.mutex.Unlock()
				// adding cluster id with release name and namespace because there can be case when two cluster or two namespaces have release with same name
				impl.HelmListClusterMap[clusterId][impl.getUniqueReleaseKey(&ReleaseDto{releaseDTO}, clusterModel.Id)] = appDetail
			}
		},
		DeleteFunc: func(obj interface{}) {
			impl.logger.Debug("Event received in Helm secret delete informer", "time", time.Now())
			if secretObject, ok := obj.(*coreV1.Secret); ok {
				if secretObject.Type != HELM_RELEASE_SECRET_TYPE {
					return
				}
				releaseDTO, err := decodeRelease(string(secretObject.Data["release"]))
				if err != nil {
					impl.logger.Error("error in decoding release")
				}
				impl.mutex.Lock()
				defer impl.mutex.Unlock()
				delete(impl.HelmListClusterMap[clusterId], impl.getUniqueReleaseKey(&ReleaseDto{releaseDTO}, clusterModel.Id))
			}
		},
	})
	informerFactory.Start(stopper)
	impl.logger.Info("informer started for cluster: ", "cluster_id", clusterModel.Id, "cluster_name", clusterModel.ClusterName)
	impl.informerStopper[clusterId] = stopper
	return nil
}

func (impl *K8sInformerImpl) getUniqueReleaseKey(release *ReleaseDto, clusterId int) string {
	return release.getUniqueReleaseIdentifier() + "_" + strconv.Itoa(clusterId)
}

func (impl *K8sInformerImpl) GetAllReleaseByClusterId(clusterId int) []*client.DeployedAppDetail {

	var deployedAppDetailList []*client.DeployedAppDetail
	releaseMap := impl.HelmListClusterMap[clusterId]
	for _, v := range releaseMap {
		deployedAppDetailList = append(deployedAppDetailList, v)
	}
	return deployedAppDetailList
}

func (impl *K8sInformerImpl) CheckReleaseExists(clusterId int32, releaseIdentifier string) bool {
	releaseMap := impl.HelmListClusterMap[int(clusterId)]
	_, ok := releaseMap[releaseIdentifier]
	if ok {
		return true
	}
	return false
}
