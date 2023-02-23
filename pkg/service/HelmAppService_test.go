package service

import (
	"context"
	client "github.com/devtron-labs/kubelink/grpc"
	"github.com/devtron-labs/kubelink/internal/logger"
	k8sUtils "github.com/devtron-labs/kubelink/pkg/util/k8s"
	"go.uber.org/zap"
	"math/rand"
	"reflect"
	"testing"
)

func TestHelmAppServiceImpl_HelmInstallCustom(t *testing.T) {
	type fields struct {
		logger     *zap.SugaredLogger
		k8sService K8sService
		randSource rand.Source
	}
	type args struct {
		request *client.HelmInstallCustomRequest
	}
	f := fields{
		logger:     &zap.SugaredLogger{},
		k8sService: NewK8sServiceImpl(&zap.SugaredLogger{}),
		randSource: rand.NewSource(10),
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    bool
		wantErr bool
	}{
		{name: "june6-test-1", fields: f, args: struct {
			request *client.HelmInstallCustomRequest
		}{request: &client.HelmInstallCustomRequest{
			ChartContent: &client.ChartContent{Content: []byte("Here is a content....")},
			ReleaseIdentifier: &client.ReleaseIdentifier{
				ReleaseName:      "june6-test-1",
				ReleaseNamespace: "test",
				ClusterConfig:    &client.ClusterConfig{ClusterName: k8sUtils.DEFAULT_CLUSTER},
			},
		}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			impl := HelmAppServiceImpl{
				logger:     tt.fields.logger,
				k8sService: tt.fields.k8sService,
				randSource: tt.fields.randSource,
			}
			got, err := impl.InstallReleaseWithCustomChart(tt.args.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("HelmInstallCustom() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("HelmInstallCustom() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHelmAppServiceImpl_GetDeploymentHistory(t *testing.T) {
	type fields struct {
		logger     *zap.SugaredLogger
		k8sService K8sService
		randSource rand.Source
	}
	type args struct {
		req *client.AppDetailRequest
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    bool
		wantErr bool
	}{
		{name: "test1", fields: fields{}, args: struct{ req *client.AppDetailRequest }{req: &client.AppDetailRequest{
			ReleaseName:   "viki-9jun-4-devtron-demo",
			Namespace:     "devtron-demo",
			ClusterConfig: &client.ClusterConfig{ClusterName: k8sUtils.DEFAULT_CLUSTER},
		}}, want: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			impl := HelmAppServiceImpl{
				logger:     tt.fields.logger,
				k8sService: tt.fields.k8sService,
				randSource: tt.fields.randSource,
			}
			got, err := impl.GetDeploymentHistory(tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetDeploymentHistory() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetDeploymentHistory() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHelmAppServiceImpl_GetNotes(t *testing.T) {
	type fields struct {
		logger     *zap.SugaredLogger
		k8sService K8sService
		randSource rand.Source
	}
	type args struct {
		ctx                   context.Context
		installReleaseRequest *client.InstallReleaseRequest
	}
	logger := logger.NewSugaredLogger()
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    string
		wantErr bool
	}{

		{name: "Test1", fields: fields{logger,
			NewK8sServiceImpl(logger),
			rand.NewSource(1)}, args: args{context.Background(), &client.InstallReleaseRequest{
			ChartName:    "nginx",
			ChartVersion: "13.2.25",
			ValuesYaml:   "",
			ChartRepository: &client.ChartRepository{
				Name:     "my-repo",
				Url:      "https://charts.bitnami.com/bitnami",
				Username: "CGFHGGJHB",
				Password: "HGHGJGJHBJ",
			},
			ReleaseIdentifier: &client.ReleaseIdentifier{
				ReleaseName:      "ngnix",
				ReleaseNamespace: "default",
				ClusterConfig:    &client.ClusterConfig{ClusterName: "", Token: ""},
			},
		}}, want: `CHART NAME: nginx
CHART VERSION: 13.2.25
APP VERSION: 1.23.3

** Please be patient while the chart is being deployed **
NGINX can be accessed through the following DNS name from within your cluster:

    ngnix-nginx.default.svc.cluster.local (port 80)

To access NGINX from outside the cluster, follow the steps below:

1. Get the NGINX URL by running these commands:

  NOTE: It may take a few minutes for the LoadBalancer IP to be available.
        Watch the status with: 'kubectl get svc --namespace default -w ngnix-nginx'

    export SERVICE_PORT=$(kubectl get --namespace default -o jsonpath="{.spec.ports[0].port}" services ngnix-nginx)
    export SERVICE_IP=$(kubectl get svc --namespace default ngnix-nginx -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
    echo "http://${SERVICE_IP}:${SERVICE_PORT}"
`, wantErr: true},
		{name: "Test2", fields: fields{logger,
			NewK8sServiceImpl(logger),
			rand.NewSource(1)}, args: args{context.Background(), &client.InstallReleaseRequest{
			ChartName:    "karpenter",
			ChartVersion: "0.16.3",
			ValuesYaml:   "",
			ChartRepository: &client.ChartRepository{
				Name:     "karpenter",
				Url:      "https://charts.karpenter.sh/",
				Username: "CGFHGGJHB",
				Password: "HGHGJGJHBJ",
			},
			ReleaseIdentifier: &client.ReleaseIdentifier{
				ReleaseName:      "karpenter",
				ReleaseNamespace: "default",
				ClusterConfig:    &client.ClusterConfig{ClusterName: "", Token: ""},
			},
		}}, want: "", wantErr: true},
		{name: "Test3", fields: fields{logger,
			NewK8sServiceImpl(logger),
			rand.NewSource(1)}, args: args{context.Background(), &client.InstallReleaseRequest{
			ChartName:    "apache",
			ChartVersion: "9.2.15",
			ValuesYaml:   "",
			ChartRepository: &client.ChartRepository{
				Name:     "my-repo",
				Url:      "https://charts.bitnami.com/bitnami",
				Username: "CGFHGGJHB",
				Password: "HGHGJGJHBJ",
			},
			ReleaseIdentifier: &client.ReleaseIdentifier{
				ReleaseName:      "apache",
				ReleaseNamespace: "default",
				ClusterConfig:    &client.ClusterConfig{ClusterName: "", Token: ""},
			},
		}}, want: `CHART NAME: apache
CHART VERSION: 9.2.15
APP VERSION: 2.4.55

** Please be patient while the chart is being deployed **

1. Get the Apache URL by running:

** Please ensure an external IP is associated to the apache service before proceeding **
** Watch the status using: kubectl get svc --namespace default -w apache **

  export SERVICE_IP=$(kubectl get svc --namespace default apache --template "{{ range (index .status.loadBalancer.ingress 0) }}{{ . }}{{ end }}")
  echo URL            : http://$SERVICE_IP/


WARNING: You did not provide a custom web application. Apache will be deployed with a default page. Check the README section "Deploying your custom web application" in https://github.com/bitnami/charts/blob/main/bitnami/apache/README.md#deploying-a-custom-web-application.


`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			impl := HelmAppServiceImpl{
				logger:     tt.fields.logger,
				k8sService: tt.fields.k8sService,
				randSource: tt.fields.randSource,
			}
			got, err := impl.GetNotes(tt.args.ctx, tt.args.installReleaseRequest)
			if (err != nil) == tt.wantErr {
				t.Errorf("GetNotes() error = %v, wantErr %v", err, tt.wantErr)
				//if err.Error() != tt.want {
				//	//println("hello found error " + err.Error())
				//	t.Errorf("GetNotes() got = %v, want %v", got, tt.want)
				//} else {
				//	//return true
				//
				//}
				//println("hello found error " + err.Error())
				return

			} else {
				if got != tt.want {
					t.Errorf("GetNotes() got = %v, want %v", got, tt.want)
				}
			}

		})
	}
}

//func TestHelmAppServiceImpl_GetNotes1(t *testing.T) {
//	type fields struct {
//		logger     *zap.SugaredLogger
//		k8sService K8sService
//		randSource rand.Source
//	}
//	type args struct {
//		ctx                   context.Context
//		installReleaseRequest *client.InstallReleaseRequest
//	}
//	tests := []struct {
//		name    string
//		fields  fields
//		args    args
//		want    string
//		wantErr bool
//	}{
//		// TODO: Add test cases.
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			impl := HelmAppServiceImpl{
//				logger:     tt.fields.logger,
//				k8sService: tt.fields.k8sService,
//				randSource: tt.fields.randSource,
//			}
//			got, err := impl.GetNotes(tt.args.ctx, tt.args.installReleaseRequest)
//			if (err != nil) != tt.wantErr {
//				t.Errorf("GetNotes() error = %v, wantErr %v", err, tt.wantErr)
//				return
//			}
//			if got != tt.want {
//				t.Errorf("GetNotes() got = %v, want %v", got, tt.want)
//			}
//		})
//	}
//}

//{name: "Test4", fields: fields{logger,
//	NewK8sServiceImpl(logger),
//	rand.NewSource(1)}, args: struct{ installReleaseRequest *client.InstallReleaseRequest }{installReleaseRequest: &client.InstallReleaseRequest{
//	ChartName:    "apachedemo",
//	ChartVersion: "9.2.15",
//	ValuesYaml:   "",
//	ChartRepository: &client.ChartRepository{
//		Name:     "my-repo",
//		Url:      "https://charts.bitnami.com/bitnami",
//		Username: "CGFHGGJHB",
//		Password: "HGHGJGJHBJ",
//	},
//	ReleaseIdentifier: &client.ReleaseIdentifier{
//		ReleaseName:      "apache",
//		ReleaseNamespace: "default",
//		ClusterConfig:    &client.ClusterConfig{ClusterName: "", Token: ""},
//	},
//}}, want: "chart \"apachedemo\" matching 9.2.15 not found in my-repo index. (try 'helm repo update'): no chart name found", wantErr: true},
