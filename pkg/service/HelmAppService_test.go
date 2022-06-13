package service

import (
	client "github.com/devtron-labs/kubelink/grpc"
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
				ClusterConfig: &client.ClusterConfig{ClusterName: k8sUtils.DEFAULT_CLUSTER,
				},
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
