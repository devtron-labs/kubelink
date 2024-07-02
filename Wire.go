//go:build wireinject
// +build wireinject

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

package main

import (
	"github.com/devtron-labs/authenticator/client"
	"github.com/devtron-labs/common-lib/helmLib/registry"
	"github.com/devtron-labs/common-lib/monitoring"
	"github.com/devtron-labs/common-lib/utils/k8s"
	"github.com/devtron-labs/kubelink/api/router"
	"github.com/devtron-labs/kubelink/converter"
	"github.com/devtron-labs/kubelink/internals/lock"
	"github.com/devtron-labs/kubelink/internals/logger"
	repository "github.com/devtron-labs/kubelink/pkg/cluster"
	"github.com/devtron-labs/kubelink/pkg/k8sInformer"
	"github.com/devtron-labs/kubelink/pkg/service"
	commonHelmService "github.com/devtron-labs/kubelink/pkg/service/commonHelmService"
	fluxService "github.com/devtron-labs/kubelink/pkg/service/fluxService"
	helmApplicationService "github.com/devtron-labs/kubelink/pkg/service/helmApplicationService"
	"github.com/devtron-labs/kubelink/pkg/sql"
	"github.com/google/wire"
)

func InitializeApp() (*App, error) {
	wire.Build(
		NewApp,
		sql.PgSqlWireSet,
		logger.NewSugaredLogger,
		client.GetRuntimeConfig,
		k8s.NewK8sUtil,
		wire.Bind(new(k8s.K8sService), new(*k8s.K8sServiceImpl)),
		lock.NewChartRepositoryLocker,
		commonHelmService.NewK8sServiceImpl,
		wire.Bind(new(commonHelmService.K8sService), new(*commonHelmService.K8sServiceImpl)),
		commonHelmService.NewCommonHelmServiceImpl,
		wire.Bind(new(commonHelmService.CommonHelmService), new(*commonHelmService.CommonHelmServiceImpl)),
		helmApplicationService.NewHelmAppServiceImpl,
		wire.Bind(new(helmApplicationService.HelmAppService), new(*helmApplicationService.HelmAppServiceImpl)),
		converter.NewConverterImpl,
		wire.Bind(new(converter.ClusterBeanConverter), new(*converter.ClusterBeanConverterImpl)),
		fluxService.NewFluxApplicationServiceImpl,
		wire.Bind(new(fluxService.FluxApplicationService), new(*fluxService.FluxApplicationServiceImpl)),
		service.NewApplicationServiceServerImpl,
		router.NewRouter,
		k8sInformer.Newk8sInformerImpl,
		wire.Bind(new(k8sInformer.K8sInformer), new(*k8sInformer.K8sInformerImpl)),
		repository.NewClusterRepositoryImpl,
		wire.Bind(new(repository.ClusterRepository), new(*repository.ClusterRepositoryImpl)),
		commonHelmService.GetHelmReleaseConfig,
		k8sInformer.GetHelmReleaseConfig,
		//pubsub_lib.NewPubSubClientServiceImpl,
		//cache.NewClusterCacheImpl,
		//wire.Bind(new(cache.ClusterCache), new(*cache.ClusterCacheImpl)),
		//cache.GetClusterCacheConfig,
		registry.NewSettingsFactoryImpl,
		wire.Bind(new(registry.SettingsFactory), new(*registry.SettingsFactoryImpl)),

		registry.NewDefaultSettingsGetter,
		wire.Bind(new(registry.DefaultSettingsGetter), new(*registry.DefaultSettingsGetterImpl)),

		monitoring.NewMonitoringRouter,
	)
	return &App{}, nil
}
