// Code generated by Wire. DO NOT EDIT.

//go:generate go run github.com/google/wire/cmd/wire
//go:build !wireinject
// +build !wireinject

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
	"github.com/devtron-labs/kubelink/pkg/cluster"
	"github.com/devtron-labs/kubelink/pkg/k8sInformer"
	"github.com/devtron-labs/kubelink/pkg/service"
	"github.com/devtron-labs/kubelink/pkg/sql"
)

// Injectors from Wire.go:

func InitializeApp() (*App, error) {
	sugaredLogger := logger.NewSugaredLogger()
	chartRepositoryLocker := lock.NewChartRepositoryLocker(sugaredLogger)
	helmReleaseConfig, err := service.GetHelmReleaseConfig()
	if err != nil {
		return nil, err
	}
	k8sServiceImpl, err := service.NewK8sServiceImpl(sugaredLogger, helmReleaseConfig)
	if err != nil {
		return nil, err
	}
	config, err := sql.GetConfig()
	if err != nil {
		return nil, err
	}
	db, err := sql.NewDbConnection(config, sugaredLogger)
	if err != nil {
		return nil, err
	}
	clusterRepositoryImpl := repository.NewClusterRepositoryImpl(db, sugaredLogger)
	k8sInformerHelmReleaseConfig, err := k8sInformer.GetHelmReleaseConfig()
	if err != nil {
		return nil, err
	}
	runtimeConfig, err := client.GetRuntimeConfig()
	if err != nil {
		return nil, err
	}
	k8sK8sServiceImpl := k8s.NewK8sUtil(sugaredLogger, runtimeConfig)
	clusterBeanConverterImpl := converter.NewConverterImpl()
	k8sInformerImpl := k8sInformer.Newk8sInformerImpl(sugaredLogger, clusterRepositoryImpl, k8sInformerHelmReleaseConfig, k8sK8sServiceImpl, clusterBeanConverterImpl)
	defaultSettingsGetterImpl := registry.NewDefaultSettingsGetter(sugaredLogger)
	settingsFactoryImpl := registry.NewSettingsFactoryImpl(defaultSettingsGetterImpl)
	helmAppServiceImpl, err := service.NewHelmAppServiceImpl(sugaredLogger, k8sServiceImpl, k8sInformerImpl, helmReleaseConfig, k8sK8sServiceImpl, clusterBeanConverterImpl, clusterRepositoryImpl, settingsFactoryImpl)
	if err != nil {
		return nil, err
	}
	applicationServiceServerImpl := service.NewApplicationServiceServerImpl(sugaredLogger, chartRepositoryLocker, helmAppServiceImpl)
	monitoringRouter := monitoring.NewMonitoringRouter(sugaredLogger)
	routerImpl := router.NewRouter(sugaredLogger, monitoringRouter)
	app := NewApp(sugaredLogger, applicationServiceServerImpl, routerImpl, k8sInformerImpl, db)
	return app, nil
}
