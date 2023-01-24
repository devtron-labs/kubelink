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
	"github.com/devtron-labs/kubelink/internal/lock"
	"github.com/devtron-labs/kubelink/internal/logger"
	"github.com/devtron-labs/kubelink/pkg/service"
	"github.com/devtron-labs/kubelink/pprof"
	"github.com/google/wire"
)

func InitializeApp() (*App, error) {
	wire.Build(
		NewApp,
		logger.NewSugaredLogger,
		lock.NewChartRepositoryLocker,
		service.NewK8sServiceImpl,
		wire.Bind(new(service.K8sService), new(*service.K8sServiceImpl)),
		service.NewHelmAppServiceImpl,
		wire.Bind(new(service.HelmAppService), new(*service.HelmAppServiceImpl)),
		service.NewApplicationServiceServerImpl,
		pprof.NewPProfRouter,
	)
	return &App{}, nil
}
