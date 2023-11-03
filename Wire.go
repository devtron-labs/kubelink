//go:build wireinject
// +build wireinject

/*
 * Copyright (c) 2020 Devtron Labs
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package main

import (
	"github.com/devtron-labs/authenticator/client"
	"github.com/devtron-labs/common-lib/utils/k8s"
	"github.com/devtron-labs/kubelink/api/router"
	"github.com/devtron-labs/kubelink/converter"
	"github.com/devtron-labs/kubelink/internals/lock"
	"github.com/devtron-labs/kubelink/internals/logger"
	repository "github.com/devtron-labs/kubelink/pkg/cluster"
	"github.com/devtron-labs/kubelink/pkg/k8sInformer"
	"github.com/devtron-labs/kubelink/pkg/service"
	"github.com/devtron-labs/kubelink/pkg/sql"
	"github.com/devtron-labs/kubelink/pprof"
	"github.com/google/wire"
)

func InitializeApp() (*App, error) {
	wire.Build(
		NewApp,
		sql.PgSqlWireSet,
		logger.NewSugaredLogger,
		client.GetRuntimeConfig,
		k8s.NewK8sUtil,
		wire.Bind(new(k8s.K8sUtilIf), new(*k8s.K8sUtil)),
		lock.NewChartRepositoryLocker,
		service.NewK8sServiceImpl,
		wire.Bind(new(service.K8sService), new(*service.K8sServiceImpl)),
		converter.NewConverterImpl,
		wire.Bind(new(converter.Converter), new(*converter.ConverterImpl)),
		service.NewHelmAppServiceImpl,
		wire.Bind(new(service.HelmAppService), new(*service.HelmAppServiceImpl)),
		service.NewApplicationServiceServerImpl,
		router.NewRouter,
		pprof.NewPProfRestHandler,
		wire.Bind(new(pprof.PProfRestHandler), new(*pprof.PProfRestHandlerImpl)),
		pprof.NewPProfRouter,
		wire.Bind(new(pprof.PProfRouter), new(*pprof.PProfRouterImpl)),
		k8sInformer.Newk8sInformerImpl,
		wire.Bind(new(k8sInformer.K8sInformer), new(*k8sInformer.K8sInformerImpl)),
		repository.NewClusterRepositoryImpl,
		wire.Bind(new(repository.ClusterRepository), new(*repository.ClusterRepositoryImpl)),
		service.GetHelmReleaseConfig,
		k8sInformer.GetHelmReleaseConfig,
		//pubsub_lib.NewPubSubClientServiceImpl,
	)
	return &App{}, nil
}
