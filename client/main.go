package main

import (
	"context"
	"fmt"
	client2 "github.com/devtron-labs/kubelink/grpc"
	"google.golang.org/grpc"
	"io"
	"log"
)

func main() {
	fmt.Println("from client")
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithInsecure())
	conn, err := grpc.Dial("localhost:50051", opts...)
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	client := client2.NewApplicationServiceClient(conn)
	req := &client2.AppListRequest{}
	config := &client2.ClusterConfig{
		ApiServerUrl: "https://api.demo1.devtron.info",
		Token:       "",
		ClusterId:    1,
		ClusterName: "default_cluster",
	}
	req.Clusters = append(req.Clusters, config)
	/*client.Hibernate(context.Background(), &client2.HibernateRequest{
		ClusterConfig:    config,
		ObjectIdentifier: []*client2.ObjectIdentifier{&client2.ObjectIdentifier{
			Group:     "apps",
			Kind:      "Deployment",
			Version:   "v1",
			Name:      "my-envoy",
			Namespace: "nishant",
		}},
	})*/

	stream, err := client.ListApplications(context.Background(), req)
	if err != nil {
		panic(err)
	}
	for {
		deployedapp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("%v.ListFeatures(_) = _, %v", client, err)
		}

		log.Println("deployed app", deployedapp.String())
	}
	detail, err := client.GetAppDetail(context.Background(), &client2.AppDetailRequest{
		ClusterConfig: config,
		Namespace:     "nishant",
		ReleaseName:   "my-envoy",
	})

	if err != nil {
		panic(err)
	}
	fmt.Println(detail)

	info, err := client.GetValuesYaml(context.Background(), &client2.AppDetailRequest{
		ClusterConfig: config,
		Namespace:     "default",
		ReleaseName:   "envoy-1640692213",
	})
	fmt.Println(err)
	fmt.Println(info)
	r, err := client.UnHibernate(context.Background(), &client2.HibernateRequest{
		ClusterConfig: config,
		ObjectIdentifier: []*client2.ObjectIdentifier{&client2.ObjectIdentifier{
			Group:     "apps",
			Kind:      "Deployment",
			Version:   "v1",
			Name:      "my-envoy",
			Namespace: "nishant",
		}},
	})

	fmt.Println(err)
	fmt.Println(r)



}
