package main

import (
	"flag"
	"fmt"
	client "github.com/devtron-labs/kubelink/grpc"
	"github.com/devtron-labs/kubelink/internal"
	"github.com/devtron-labs/kubelink/pkg/service"
	"go.uber.org/zap"
	_ "go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"net"
	"time"
)

const (
	clusterHost        = "https://api.demo1.devtron.info"
	clusterBaererToken = ""
	namespace          = "manish"
	releaseName        = "manish-chart-release"
)

func main() {
	config := zap.NewProductionConfig()
	log, err := config.Build()
	if err != nil {
		panic("erorr in building logger")
	}
	logger := log.Sugar()
	flag.Parse()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", 50051))
	if err != nil {
		logger.Fatalw("failed to listen: %v", "err", err)
	}
	opts := []grpc.ServerOption{
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionAge: 10 * time.Second,
		}),
	}
	grpcServer := grpc.NewServer(opts...)

	// create repository locker singleton
	chartRepositoryLocker := internal.NewChartRepositoryLocker(logger)

	client.RegisterApplicationServiceServer(grpcServer, service.NewApplicationServiceServerImpl(logger, chartRepositoryLocker))
	logger.Info("starting server ...................")
	err = grpcServer.Serve(lis)
	if err != nil {
		logger.Fatalw("failed to listen: %v", "err", err)

	}
}

func appDetail() bool {
	/*	appDetailRequest := &bean.HelmReleaseDetailRequest{
			ClusterHost:        clusterHost,
			ClusterBaererToken: clusterBaererToken,
			Namespace:          namespace,
			ReleaseName:        releaseName,
		}
		appDetail, err := service.BuildAppDetail(appDetailRequest)
		if err != nil {
			fmt.Println(err)
			return true
		}

		appDetailByteArr, err := json.Marshal(appDetail)
		if err != nil {
			fmt.Println(err)
			return true
		}

		appDetailJsonResp := string(appDetailByteArr)
		fmt.Println(appDetailJsonResp)*/
	// app detail ends

	// helm app values starts
	/*	appValuesRequest := &bean.HelmReleaseDetailRequest{
			ClusterHost:        clusterHost,
			ClusterBaererToken: clusterBaererToken,
			Namespace:          namespace,
			ReleaseName:        releaseName,
		}
		helmAppValues, err := service.GetHelmAppValues(appValuesRequest)
		if err != nil {
			fmt.Println(err)
			return true
		}*/

	/*helmAppValuesByteArr, err := json.Marshal(helmAppValues)
	if err != nil {
		fmt.Println(err)
		return true
	}
	helmAppValuesJsonResp := string(helmAppValuesByteArr)
	fmt.Println(helmAppValuesJsonResp)*/
	// helm app values ends

	//hibernate starts
	/*var hibernateRequests []*bean.HibernateRequest
	for _, node := range appDetail.ResourceTreeResponse.Nodes {
		if node.CanBeHibernated {
			hibernateRequest := &bean.HibernateRequest{
				Group:     node.Group,
				Kind:      node.Kind,
				Version:   node.Version,
				Name:      node.Name,
				Namespace: node.Namespace,
			}
			hibernateRequests = append(hibernateRequests, hibernateRequest)
		}
	}*/
	/*clusterConfig := &k8sUtils.ClusterConfig{
		Host:        clusterHost,
		BearerToken: clusterBaererToken,
	}*/
	/*_, err = service.Hibernate(clusterConfig, hibernateRequests)
	if err != nil {
		fmt.Println(err)
		return true
	}
	//hibernate ends

	//un-hibernate starts
	var unHibernateRequests []*bean.HibernateRequest
	for _, node := range appDetail.ResourceTreeResponse.Nodes {
		if node.IsHibernated {
			unHibernateRequest := &bean.HibernateRequest{
				Group:     node.Group,
				Kind:      node.Kind,
				Version:   node.Version,
				Name:      node.Name,
				Namespace: node.Namespace,
			}
			unHibernateRequests = append(unHibernateRequests, unHibernateRequest)
		}
	}
	_, err = service.UnHibernate(clusterConfig, unHibernateRequests)
	if err != nil {
		fmt.Println(err)
		return true
	}*/
	//un-hibernate ends

	// Release history starts
	/*appDeploymentHistoryRequest := &bean.HelmReleaseDetailRequest{
		ClusterHost:        clusterHost,
		ClusterBaererToken: clusterBaererToken,
		Namespace:          namespace,
		ReleaseName:        releaseName,
	}
	deploymentHistory, err := service.GetDeploymentHistory(appDeploymentHistoryRequest)
	if err != nil {
		fmt.Println(err)
		return true
	}*/

	/*	deploymentHistoryByteArr, err := json.Marshal(deploymentHistory)
		if err != nil {
			fmt.Println(err)
			return true
		}
		deploymentHistoryJsonResp := string(deploymentHistoryByteArr)
		fmt.Println(deploymentHistoryJsonResp)*/
	// Release history ends
	return false
}
