package service

import (
	"context"
	"fmt"
	client "github.com/devtron-labs/kubelink/grpc"
	"github.com/devtron-labs/kubelink/internal/logger"
	k8sUtils "github.com/devtron-labs/kubelink/pkg/util/k8s"
	"github.com/stretchr/testify/assert"
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
		{name: "Test4", fields: fields{logger,
			NewK8sServiceImpl(logger),
			rand.NewSource(1)}, args: args{context.Background(), &client.InstallReleaseRequest{
			ChartName:    "apachedemo",
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
		}}, want: "chart \"apachedemo\" matching 9.2.15 not found in my-repo index. (try 'helm repo update'): no chart name found", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			impl := HelmAppServiceImpl{
				logger:     tt.fields.logger,
				k8sService: tt.fields.k8sService,
				randSource: tt.fields.randSource,
			}
			got, err := impl.GetNotes(tt.args.ctx, tt.args.installReleaseRequest)
			if err != nil {
				got = err.Error()
			}
			if got != tt.want {
				t.Errorf("GetNotes() got = %v, want %v", got, tt.want)
			}

		})
	}
}

func TestHelmAppServiceImpl_GetNotes1(t *testing.T) {
	type fields struct {
		logger     *zap.SugaredLogger
		k8sService K8sService
		randSource rand.Source
	}
	type args struct {
		ctx     context.Context
		request *client.InstallReleaseRequest
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
			ChartVersion: "13.2.30",
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
CHART VERSION: 13.2.30
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
			ChartVersion: "9.2.19",
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
CHART VERSION: 9.2.19
APP VERSION: 2.4.56

** Please be patient while the chart is being deployed **

1. Get the Apache URL by running:

** Please ensure an external IP is associated to the apache service before proceeding **
** Watch the status using: kubectl get svc --namespace default -w apache **

  export SERVICE_IP=$(kubectl get svc --namespace default apache --template "{{ range (index .status.loadBalancer.ingress 0) }}{{ . }}{{ end }}")
  echo URL            : http://$SERVICE_IP/


WARNING: You did not provide a custom web application. Apache will be deployed with a default page. Check the README section "Deploying your custom web application" in https://github.com/bitnami/charts/blob/main/bitnami/apache/README.md#deploying-a-custom-web-application.


`, wantErr: true},
		{name: "Test4", fields: fields{logger,
			NewK8sServiceImpl(logger),
			rand.NewSource(1)}, args: args{context.Background(), &client.InstallReleaseRequest{
			ChartName:    "apachedemo",
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
		}}, want: "chart \"apachedemo\" not found in https://charts.bitnami.com/bitnami repository", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			impl := HelmAppServiceImpl{
				logger:     tt.fields.logger,
				k8sService: tt.fields.k8sService,
				randSource: tt.fields.randSource,
			}
			got, err := impl.GetNotes(tt.args.ctx, tt.args.request)
			if err != nil {
				got = err.Error()
			}
			if got != tt.want {
				t.Errorf("GetNotes() got = %v, want = %v", got, tt.want)
			}

		})
	}
}

func TestHelmAppServiceImpl_HelmInstall(t *testing.T) {

	type fields struct {
		logger     *zap.SugaredLogger
		k8sService K8sService
		randSource rand.Source
	}
	type args struct {
		ctx     context.Context
		request *client.InstallReleaseRequest
	}
	f := fields{
		logger:     logger.NewSugaredLogger(),
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
		{name: "test for public oci chart",
			fields: f,
			args: args{ctx: context.Background(), request: &client.InstallReleaseRequest{
				ChartName:    "memcached",
				ChartVersion: "6.2.6",
				ReleaseIdentifier: &client.ReleaseIdentifier{
					ReleaseName:      fmt.Sprintf("%s-%d", "test", rand.Int()),
					ReleaseNamespace: "test",
					ClusterConfig:    &client.ClusterConfig{ClusterName: k8sUtils.DEFAULT_CLUSTER},
				},
				IsOCIRepo: true,
				RegistryCredential: &client.RegistryCredential{
					RegistryUrl:  "registry-1.docker.io",
					Username:     "",
					Password:     "",
					AwsRegion:    "",
					AccessKey:    "",
					SecretKey:    "",
					RegistryType: "",
					RepoName:     "bitnamicharts/memcached",
					IsPublic:     true,
				},
			}}},
		{name: "test for private oci chart",
			fields: f,
			args: args{ctx: context.Background(), request: &client.InstallReleaseRequest{
				ChartName:    "memcached-oci",
				ChartVersion: "1.0.2431-DEPLOY",
				ReleaseIdentifier: &client.ReleaseIdentifier{
					ReleaseName:      fmt.Sprintf("%s-%d", "test", rand.Int()),
					ReleaseNamespace: "test",
					ClusterConfig:    &client.ClusterConfig{ClusterName: k8sUtils.DEFAULT_CLUSTER},
				},
				IsOCIRepo: true,
				RegistryCredential: &client.RegistryCredential{
					RegistryUrl:  "registry-1.docker.io",
					Username:     "ayushm10",
					Password:     "dckr_pat_FQIiOMMQvCxdT2Q_MbdI-K5F7hY",
					AwsRegion:    "",
					AccessKey:    "",
					SecretKey:    "",
					RegistryType: "",
					RepoName:     "ayushm10/chart",
					IsPublic:     false,
				},
			}}},
		{name: "test for public index.yaml helm chart",
			fields: f,
			args: args{ctx: context.Background(), request: &client.InstallReleaseRequest{
				ChartName:    "memcached",
				ChartVersion: "6.5.6",
				ReleaseIdentifier: &client.ReleaseIdentifier{
					ReleaseName:      fmt.Sprintf("%s-%d", "test", rand.Int()),
					ReleaseNamespace: "test",
					ClusterConfig:    &client.ClusterConfig{ClusterName: k8sUtils.DEFAULT_CLUSTER},
				},
				IsOCIRepo:          false,
				RegistryCredential: nil,
				ChartRepository: &client.ChartRepository{
					Name: "memcached",
					Url:  "https://charts.bitnami.com/bitnami",
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
			got, err := impl.InstallRelease(tt.args.ctx, tt.args.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("HelmInstallCustom() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Nil(t, err)
			assert.Equal(t, got.Success, true)

			// clean installed app
			defer impl.UninstallRelease(tt.args.request.ReleaseIdentifier)
		})
	}
}

func TestHelmAppServiceImpl_HelmTemplate(t *testing.T) {

	type fields struct {
		logger     *zap.SugaredLogger
		k8sService K8sService
		randSource rand.Source
	}
	type args struct {
		ctx     context.Context
		request *client.InstallReleaseRequest
	}
	f := fields{
		logger:     logger.NewSugaredLogger(),
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
		{name: "Template function test: public oci chart",
			fields: f,
			args: args{ctx: context.Background(), request: &client.InstallReleaseRequest{
				ChartName:    "memcached",
				ChartVersion: "6.2.6",
				ValuesYaml:   "# Copyright VMware, Inc.\n# SPDX-License-Identifier: APACHE-2.0\n\n## @section Global parameters\n## Global Docker image parameters\n## Please, note that this will override the image parameters, including dependencies, configured to use the global value\n## Current available global Docker image parameters: imageRegistry, imagePullSecrets and storageClass\n\n## @param global.imageRegistry Global Docker image registry\n## @param global.imagePullSecrets Global Docker registry secret names as an array\n## @param global.storageClass Global StorageClass for Persistent Volume(s)\n##\nglobal:\n  imageRegistry: \"\"\n  ## E.g.\n  ## imagePullSecrets:\n  ##   - myRegistryKeySecretName\n  ##\n  imagePullSecrets: []\n  storageClass: \"\"\n\n## @section Common parameters\n\n## @param kubeVersion Override Kubernetes version\n##\nkubeVersion: \"\"\n## @param nameOverride String to partially override common.names.fullname template (will maintain the release name)\n##\nnameOverride: \"\"\n## @param fullnameOverride String to fully override common.names.fullname template\n##\nfullnameOverride: \"\"\n## @param clusterDomain Kubernetes Cluster Domain\n##\nclusterDomain: cluster.local\n## @param extraDeploy Extra objects to deploy (evaluated as a template)\n##\nextraDeploy: []\n## @param commonLabels Add labels to all the deployed resources\n##\ncommonLabels: {}\n## @param commonAnnotations Add annotations to all the deployed resources\n##\ncommonAnnotations: {}\n\n## Enable diagnostic mode in the deployment/statefulset\n##\ndiagnosticMode:\n  ## @param diagnosticMode.enabled Enable diagnostic mode (all probes will be disabled and the command will be overridden)\n  ##\n  enabled: false\n  ## @param diagnosticMode.command Command to override all containers in the deployment/statefulset\n  ##\n  command:\n    - sleep\n  ## @param diagnosticMode.args Args to override all containers in the deployment/statefulset\n  ##\n  args:\n    - infinity\n\n## @section Memcached parameters\n\n## Bitnami Memcached image version\n## ref: https://hub.docker.com/r/bitnami/memcached/tags/\n## @param image.registry Memcached image registry\n## @param image.repository Memcached image repository\n## @param image.tag Memcached image tag (immutable tags are recommended)\n## @param image.digest Memcached image digest in the way sha256:aa.... Please note this parameter, if set, will override the tag\n## @param image.pullPolicy Memcached image pull policy\n## @param image.pullSecrets Specify docker-registry secret names as an array\n## @param image.debug Specify if debug values should be set\n##\nimage:\n  registry: docker.io\n  repository: bitnami/memcached\n  tag: 1.6.21-debian-11-r38\n  digest: \"\"\n  ## Specify a imagePullPolicy\n  ## Defaults to 'Always' if image tag is 'latest', else set to 'IfNotPresent'\n  ## ref: https://kubernetes.io/docs/user-guide/images/#pre-pulling-images\n  ##\n  pullPolicy: IfNotPresent\n  ## Optionally specify an array of imagePullSecrets.\n  ## Secrets must be manually created in the namespace.\n  ## ref: https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/\n  ## e.g:\n  ## pullSecrets:\n  ##   - myRegistryKeySecretName\n  ##\n  pullSecrets: []\n  ## Set to true if you would like to see extra information on logs\n  ##\n  debug: false\n## @param architecture Memcached architecture. Allowed values: standalone or high-availability\n##\narchitecture: standalone\n## Authentication parameters\n## ref: https://github.com/bitnami/containers/tree/main/bitnami/memcached#creating-the-memcached-admin-user\n##\nauth:\n  ## @param auth.enabled Enable Memcached authentication\n  ##\n  enabled: false\n  ## @param auth.username Memcached admin user\n  ##\n  username: \"\"\n  ## @param auth.password Memcached admin password\n  ##\n  password: \"\"\n  ## @param auth.existingPasswordSecret Existing secret with Memcached credentials (must contain a value for `memcached-password` key)\n  ##\n  existingPasswordSecret: \"\"\n## @param command Override default container command (useful when using custom images)\n##\ncommand: []\n## @param args Override default container args (useful when using custom images)\n## e.g:\n## args:\n##   - /run.sh\n##   - -m <maxMemoryLimit>\n##   - -I <maxItemSize>\n##   - -vv\n##\nargs: []\n## @param extraEnvVars Array with extra environment variables to add to Memcached nodes\n## e.g:\n## extraEnvVars:\n##   - name: FOO\n##     value: \"bar\"\n##\nextraEnvVars: []\n## @param extraEnvVarsCM Name of existing ConfigMap containing extra env vars for Memcached nodes\n##\nextraEnvVarsCM: \"\"\n## @param extraEnvVarsSecret Name of existing Secret containing extra env vars for Memcached nodes\n##\nextraEnvVarsSecret: \"\"\n\n## @section Deployment/Statefulset parameters\n\n## @param replicaCount Number of Memcached nodes\n##\nreplicaCount: 1\n## @param containerPorts.memcached Memcached container port\n##\ncontainerPorts:\n  memcached: 11211\n## Configure extra options for Memcached containers' liveness, readiness and startup probes\n## ref: https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/#configure-probes\n## @param livenessProbe.enabled Enable livenessProbe on Memcached containers\n## @param livenessProbe.initialDelaySeconds Initial delay seconds for livenessProbe\n## @param livenessProbe.periodSeconds Period seconds for livenessProbe\n## @param livenessProbe.timeoutSeconds Timeout seconds for livenessProbe\n## @param livenessProbe.failureThreshold Failure threshold for livenessProbe\n## @param livenessProbe.successThreshold Success threshold for livenessProbe\n##\nlivenessProbe:\n  enabled: true\n  initialDelaySeconds: 30\n  periodSeconds: 10\n  timeoutSeconds: 5\n  failureThreshold: 6\n  successThreshold: 1\n## @param readinessProbe.enabled Enable readinessProbe on Memcached containers\n## @param readinessProbe.initialDelaySeconds Initial delay seconds for readinessProbe\n## @param readinessProbe.periodSeconds Period seconds for readinessProbe\n## @param readinessProbe.timeoutSeconds Timeout seconds for readinessProbe\n## @param readinessProbe.failureThreshold Failure threshold for readinessProbe\n## @param readinessProbe.successThreshold Success threshold for readinessProbe\n##\nreadinessProbe:\n  enabled: true\n  initialDelaySeconds: 5\n  periodSeconds: 5\n  timeoutSeconds: 3\n  failureThreshold: 6\n  successThreshold: 1\n## @param startupProbe.enabled Enable startupProbe on Memcached containers\n## @param startupProbe.initialDelaySeconds Initial delay seconds for startupProbe\n## @param startupProbe.periodSeconds Period seconds for startupProbe\n## @param startupProbe.timeoutSeconds Timeout seconds for startupProbe\n## @param startupProbe.failureThreshold Failure threshold for startupProbe\n## @param startupProbe.successThreshold Success threshold for startupProbe\n##\nstartupProbe:\n  enabled: false\n  initialDelaySeconds: 30\n  periodSeconds: 10\n  timeoutSeconds: 1\n  failureThreshold: 15\n  successThreshold: 1\n## @param customLivenessProbe Custom livenessProbe that overrides the default one\n##\ncustomLivenessProbe: {}\n## @param customReadinessProbe Custom readinessProbe that overrides the default one\n##\ncustomReadinessProbe: {}\n## @param customStartupProbe Custom startupProbe that overrides the default one\n##\ncustomStartupProbe: {}\n## @param lifecycleHooks for the Memcached container(s) to automate configuration before or after startup\n##\nlifecycleHooks: {}\n## Memcached resource requests and limits\n## ref: https://kubernetes.io/docs/user-guide/compute-resources/\n## @param resources.limits The resources limits for the Memcached containers\n## @param resources.requests.memory The requested memory for the Memcached containers\n## @param resources.requests.cpu The requested cpu for the Memcached containers\n##\nresources:\n  limits: {}\n  requests:\n    memory: 256Mi\n    cpu: 250m\n## Configure Pods Security Context\n## ref: https://kubernetes.io/docs/tasks/configure-pod-container/security-context/#set-the-security-context-for-a-pod\n## @param podSecurityContext.enabled Enabled Memcached pods' Security Context\n## @param podSecurityContext.fsGroup Set Memcached pod's Security Context fsGroup\n##\npodSecurityContext:\n  enabled: true\n  fsGroup: 1001\n## Configure Container Security Context\n## ref: https://kubernetes.io/docs/tasks/configure-pod-container/security-context/#set-the-security-context-for-a-container\n## @param containerSecurityContext.enabled Enabled Memcached containers' Security Context\n## @param containerSecurityContext.runAsUser Set Memcached containers' Security Context runAsUser\n## @param containerSecurityContext.runAsNonRoot Set Memcached containers' Security Context runAsNonRoot\n##\ncontainerSecurityContext:\n  enabled: true\n  runAsUser: 1001\n  runAsNonRoot: true\n## @param hostAliases Add deployment host aliases\n## https://kubernetes.io/docs/concepts/services-networking/add-entries-to-pod-etc-hosts-with-host-aliases/\n##\nhostAliases: []\n## @param podLabels Extra labels for Memcached pods\n## ref: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/\n##\npodLabels: {}\n## @param podAnnotations Annotations for Memcached pods\n## ref: https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/\n##\npodAnnotations: {}\n## @param podAffinityPreset Pod affinity preset. Ignored if `affinity` is set. Allowed values: `soft` or `hard`\n## ref: https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#inter-pod-affinity-and-anti-affinity\n##\npodAffinityPreset: \"\"\n## @param podAntiAffinityPreset Pod anti-affinity preset. Ignored if `affinity` is set. Allowed values: `soft` or `hard`\n## Ref: https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#inter-pod-affinity-and-anti-affinity\n##\npodAntiAffinityPreset: soft\n## Node affinity preset\n## Ref: https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#node-affinity\n##\nnodeAffinityPreset:\n  ## @param nodeAffinityPreset.type Node affinity preset type. Ignored if `affinity` is set. Allowed values: `soft` or `hard`\n  ##\n  type: \"\"\n  ## @param nodeAffinityPreset.key Node label key to match Ignored if `affinity` is set.\n  ## E.g.\n  ## key: \"kubernetes.io/e2e-az-name\"\n  ##\n  key: \"\"\n  ## @param nodeAffinityPreset.values Node label values to match. Ignored if `affinity` is set.\n  ## E.g.\n  ## values:\n  ##   - e2e-az1\n  ##   - e2e-az2\n  ##\n  values: []\n## @param affinity Affinity for pod assignment\n## Ref: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#affinity-and-anti-affinity\n## Note: podAffinityPreset, podAntiAffinityPreset, and nodeAffinityPreset will be ignored when it's set\n##\naffinity: {}\n## @param nodeSelector Node labels for pod assignment\n## Ref: https://kubernetes.io/docs/user-guide/node-selection/\n##\nnodeSelector: {}\n## @param tolerations Tolerations for pod assignment\n## Ref: https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/\n##\ntolerations: []\n## @param topologySpreadConstraints Topology Spread Constraints for pod assignment spread across your cluster among failure-domains. Evaluated as a template\n## Ref: https://kubernetes.io/docs/concepts/workloads/pods/pod-topology-spread-constraints/#spread-constraints-for-pods\n##\ntopologySpreadConstraints: []\n## @param podManagementPolicy StatefulSet controller supports relax its ordering guarantees while preserving its uniqueness and identity guarantees. There are two valid pod management policies: `OrderedReady` and `Parallel`\n## ref: https://kubernetes.io/docs/tutorials/stateful-application/basic-stateful-set/#pod-management-policy\n##\npodManagementPolicy: Parallel\n## @param priorityClassName Name of the existing priority class to be used by Memcached pods, priority class needs to be created beforehand\n## Ref: https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption/\n##\npriorityClassName: \"\"\n## @param schedulerName Kubernetes pod scheduler registry\n## https://kubernetes.io/docs/tasks/administer-cluster/configure-multiple-schedulers/\n##\nschedulerName: \"\"\n## @param terminationGracePeriodSeconds In seconds, time the given to the memcached pod needs to terminate gracefully\n## ref: https://kubernetes.io/docs/concepts/workloads/pods/pod/#termination-of-pods\n##\nterminationGracePeriodSeconds: \"\"\n## @param updateStrategy.type Memcached statefulset strategy type\n## @param updateStrategy.rollingUpdate Memcached statefulset rolling update configuration parameters\n## ref: https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/#update-strategies\n##\nupdateStrategy:\n  type: RollingUpdate\n  rollingUpdate: {}\n## @param extraVolumes Optionally specify extra list of additional volumes for the Memcached pod(s)\n## Example Use Case: mount certificates to enable TLS\n## e.g:\n## extraVolumes:\n## - name: zookeeper-keystore\n##   secret:\n##     defaultMode: 288\n##     secretName: zookeeper-keystore\n## - name: zookeeper-truststore\n##   secret:\n##     defaultMode: 288\n##     secretName: zookeeper-truststore\n##\nextraVolumes: []\n## @param extraVolumeMounts Optionally specify extra list of additional volumeMounts for the Memcached container(s)\n## Example Use Case: mount certificates to enable TLS\n## e.g:\n## extraVolumeMounts:\n## - name: zookeeper-keystore\n##   mountPath: /certs/keystore\n##   readOnly: true\n## - name: zookeeper-truststore\n##   mountPath: /certs/truststore\n##   readOnly: true\n##\nextraVolumeMounts: []\n## @param sidecars Add additional sidecar containers to the Memcached pod(s)\n## e.g:\n## sidecars:\n##   - name: your-image-name\n##     image: your-image\n##     imagePullPolicy: Always\n##     ports:\n##       - name: portname\n##         containerPort: 1234\n##\nsidecars: []\n## @param initContainers Add additional init containers to the Memcached pod(s)\n## Example:\n## initContainers:\n##   - name: your-image-name\n##     image: your-image\n##     imagePullPolicy: Always\n##     ports:\n##       - name: portname\n##         containerPort: 1234\n##\ninitContainers: []\n## Memcached Autoscaling\n## @param autoscaling.enabled Enable memcached statefulset autoscaling (requires architecture: \"high-availability\")\n## @param autoscaling.minReplicas memcached statefulset autoscaling minimum number of replicas\n## @param autoscaling.maxReplicas memcached statefulset autoscaling maximum number of replicas\n## @param autoscaling.targetCPU memcached statefulset autoscaling target CPU percentage\n## @param autoscaling.targetMemory memcached statefulset autoscaling target CPU memory\n##\nautoscaling:\n  enabled: false\n  minReplicas: 3\n  maxReplicas: 6\n  targetCPU: 50\n  targetMemory: 50\n## Memcached Pod Disruption Budget\n## ref: https://kubernetes.io/docs/concepts/workloads/pods/disruptions/\n## @param pdb.create Deploy a pdb object for the Memcached pod\n## @param pdb.minAvailable Minimum available Memcached replicas\n## @param pdb.maxUnavailable Maximum unavailable Memcached replicas\n##\npdb:\n  create: false\n  minAvailable: \"\"\n  maxUnavailable: 1\n\n## @section Traffic Exposure parameters\n\nservice:\n  ## @param service.type Kubernetes Service type\n  ##\n  type: ClusterIP\n  ## @param service.ports.memcached Memcached service port\n  ##\n  ports:\n    memcached: 11211\n  ## Node ports to expose\n  ## NOTE: choose port between <30000-32767>\n  ## @param service.nodePorts.memcached Node port for Memcached\n  ##\n  nodePorts:\n    memcached: \"\"\n  ## @param service.sessionAffinity Control where client requests go, to the same pod or round-robin\n  ## Values: ClientIP or None\n  ## ref: https://kubernetes.io/docs/user-guide/services/\n  ##\n  sessionAffinity: None\n  ## @param service.sessionAffinityConfig Additional settings for the sessionAffinity\n  ## sessionAffinityConfig:\n  ##   clientIP:\n  ##     timeoutSeconds: 300\n  ##\n  sessionAffinityConfig: {}\n  ## @param service.clusterIP Memcached service Cluster IP\n  ## e.g.:\n  ## clusterIP: None\n  ##\n  clusterIP: \"\"\n  ## @param service.loadBalancerIP Memcached service Load Balancer IP\n  ## ref: https://kubernetes.io/docs/user-guide/services/#type-loadbalancer\n  ##\n  loadBalancerIP: \"\"\n  ## @param service.loadBalancerSourceRanges Memcached service Load Balancer sources\n  ## ref: https://kubernetes.io/docs/tasks/access-application-cluster/configure-cloud-provider-firewall/#restrict-access-for-loadbalancer-service\n  ## e.g:\n  ## loadBalancerSourceRanges:\n  ##   - 10.10.10.0/24\n  ##\n  loadBalancerSourceRanges: []\n  ## @param service.externalTrafficPolicy Memcached service external traffic policy\n  ## ref https://kubernetes.io/docs/tasks/access-application-cluster/create-external-load-balancer/#preserving-the-client-source-ip\n  ##\n  externalTrafficPolicy: Cluster\n  ## @param service.annotations Additional custom annotations for Memcached service\n  ##\n  annotations: {}\n  ## @param service.extraPorts Extra ports to expose in the Memcached service (normally used with the `sidecar` value)\n  ##\n  extraPorts: []\n\n## @section Other Parameters\n\n## Service account for Memcached to use.\n## ref: https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/\n##\nserviceAccount:\n  ## @param serviceAccount.create Enable creation of ServiceAccount for Memcached pod\n  ##\n  create: false\n  ## @param serviceAccount.name The name of the ServiceAccount to use.\n  ## If not set and create is true, a name is generated using the common.names.fullname template\n  ##\n  name: \"\"\n  ## @param serviceAccount.automountServiceAccountToken Allows auto mount of ServiceAccountToken on the serviceAccount created\n  ## Can be set to false if pods using this serviceAccount do not need to use K8s API\n  ##\n  automountServiceAccountToken: true\n  ## @param serviceAccount.annotations Additional custom annotations for the ServiceAccount\n  ##\n  annotations: {}\n\n## @section Persistence parameters\n\n## Enable persistence using Persistent Volume Claims\n## ref: https://kubernetes.io/docs/user-guide/persistent-volumes/\n##\npersistence:\n  ## @param persistence.enabled Enable Memcached data persistence using PVC. If false, use emptyDir\n  ##\n  enabled: false\n  ## @param persistence.storageClass PVC Storage Class for Memcached data volume\n  ## If defined, storageClassName: <storageClass>\n  ## If set to \"-\", storageClassName: \"\", which disables dynamic provisioning\n  ## If undefined (the default) or set to null, no storageClassName spec is\n  ##   set, choosing the default provisioner.  (gp2 on AWS, standard on\n  ##   GKE, AWS & OpenStack)\n  ##\n  storageClass: \"\"\n  ## @param persistence.accessModes PVC Access modes\n  ##\n  accessModes:\n    - ReadWriteOnce\n  ## @param persistence.size PVC Storage Request for Memcached data volume\n  ##\n  size: 8Gi\n  ## @param persistence.annotations Annotations for the PVC\n  ##\n  annotations: {}\n  ## @param persistence.labels Labels for the PVC\n  ##\n  labels: {}\n  ## @param persistence.selector Selector to match an existing Persistent Volume for Memcached's data PVC\n  ## If set, the PVC can't have a PV dynamically provisioned for it\n  ## E.g.\n  ## selector:\n  ##   matchLabels:\n  ##     app: my-app\n  ##\n  selector: {}\n\n## @section Volume Permissions parameters\n##\n\n## Init containers parameters:\n## volumePermissions: Change the owner and group of the persistent volume(s) mountpoint(s) to 'runAsUser:fsGroup' on each node\n##\nvolumePermissions:\n  ## @param volumePermissions.enabled Enable init container that changes the owner and group of the persistent volume\n  ##\n  enabled: false\n  ## @param volumePermissions.image.registry Init container volume-permissions image registry\n  ## @param volumePermissions.image.repository Init container volume-permissions image repository\n  ## @param volumePermissions.image.tag Init container volume-permissions image tag (immutable tags are recommended)\n  ## @param volumePermissions.image.digest Init container volume-permissions image digest in the way sha256:aa.... Please note this parameter, if set, will override the tag\n  ## @param volumePermissions.image.pullPolicy Init container volume-permissions image pull policy\n  ## @param volumePermissions.image.pullSecrets Init container volume-permissions image pull secrets\n  ##\n  image:\n    registry: docker.io\n    repository: bitnami/os-shell\n    tag: 11-debian-11-r16\n    digest: \"\"\n    pullPolicy: IfNotPresent\n    ## Optionally specify an array of imagePullSecrets.\n    ## Secrets must be manually created in the namespace.\n    ## ref: https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/\n    ## Example:\n    ## pullSecrets:\n    ##   - myRegistryKeySecretName\n    ##\n    pullSecrets: []\n  ## Init container resource requests and limits\n  ## ref: https://kubernetes.io/docs/user-guide/compute-resources/\n  ## @param volumePermissions.resources.limits Init container volume-permissions resource limits\n  ## @param volumePermissions.resources.requests Init container volume-permissions resource requests\n  ##\n  resources:\n    limits: {}\n    requests: {}\n  ## Init container' Security Context\n  ## Note: the chown of the data folder is done to containerSecurityContext.runAsUser\n  ## and not the below volumePermissions.containerSecurityContext.runAsUser\n  ## @param volumePermissions.containerSecurityContext.runAsUser User ID for the init container\n  ##\n  containerSecurityContext:\n    runAsUser: 0\n\n## Prometheus Exporter / Metrics\n##\nmetrics:\n  ## @param metrics.enabled Start a side-car prometheus exporter\n  ##\n  enabled: false\n  ## Bitnami Memcached Prometheus Exporter image\n  ## ref: https://hub.docker.com/r/bitnami/memcached-exporter/tags/\n  ## @param metrics.image.registry Memcached exporter image registry\n  ## @param metrics.image.repository Memcached exporter image repository\n  ## @param metrics.image.tag Memcached exporter image tag (immutable tags are recommended)\n  ## @param metrics.image.digest Memcached exporter image digest in the way sha256:aa.... Please note this parameter, if set, will override the tag\n  ## @param metrics.image.pullPolicy Image pull policy\n  ## @param metrics.image.pullSecrets Specify docker-registry secret names as an array\n  ##\n  image:\n    registry: docker.io\n    repository: bitnami/memcached-exporter\n    tag: 0.13.0-debian-11-r51\n    digest: \"\"\n    pullPolicy: IfNotPresent\n    ## Optionally specify an array of imagePullSecrets.\n    ## Secrets must be manually created in the namespace.\n    ## ref: https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/\n    ## e.g:\n    ## pullSecrets:\n    ##   - myRegistryKeySecretName\n    ##\n    pullSecrets: []\n  ## @param metrics.containerPorts.metrics Memcached Prometheus Exporter container port\n  ##\n  containerPorts:\n    metrics: 9150\n  ## Memcached Prometheus exporter container resource requests and limits\n  ## ref: https://kubernetes.io/docs/user-guide/compute-resources/\n  ## @param metrics.resources.limits Init container volume-permissions resource limits\n  ## @param metrics.resources.requests Init container volume-permissions resource requests\n  ##\n  resources:\n    limits: {}\n    requests: {}\n  ## Configure Metrics Container Security Context\n  ## ref: https://kubernetes.io/docs/tasks/configure-pod-container/security-context/#set-the-security-context-for-a-container\n  ## @param metrics.containerSecurityContext.enabled Enabled Metrics containers' Security Context\n  ## @param metrics.containerSecurityContext.runAsUser Set Metrics containers' Security Context runAsUser\n  ## @param metrics.containerSecurityContext.runAsNonRoot Set Metrics containers' Security Context runAsNonRoot\n  ##\n  containerSecurityContext:\n    enabled: true\n    runAsUser: 1001\n    runAsNonRoot: true\n  ## Configure extra options for Memcached Prometheus exporter containers' liveness, readiness and startup probes\n  ## ref: https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/#configure-probes\n  ## @param metrics.livenessProbe.enabled Enable livenessProbe on Memcached Prometheus exporter containers\n  ## @param metrics.livenessProbe.initialDelaySeconds Initial delay seconds for livenessProbe\n  ## @param metrics.livenessProbe.periodSeconds Period seconds for livenessProbe\n  ## @param metrics.livenessProbe.timeoutSeconds Timeout seconds for livenessProbe\n  ## @param metrics.livenessProbe.failureThreshold Failure threshold for livenessProbe\n  ## @param metrics.livenessProbe.successThreshold Success threshold for livenessProbe\n  ##\n  livenessProbe:\n    enabled: true\n    initialDelaySeconds: 15\n    periodSeconds: 10\n    timeoutSeconds: 5\n    failureThreshold: 3\n    successThreshold: 1\n  ## @param metrics.readinessProbe.enabled Enable readinessProbe on Memcached Prometheus exporter containers\n  ## @param metrics.readinessProbe.initialDelaySeconds Initial delay seconds for readinessProbe\n  ## @param metrics.readinessProbe.periodSeconds Period seconds for readinessProbe\n  ## @param metrics.readinessProbe.timeoutSeconds Timeout seconds for readinessProbe\n  ## @param metrics.readinessProbe.failureThreshold Failure threshold for readinessProbe\n  ## @param metrics.readinessProbe.successThreshold Success threshold for readinessProbe\n  ##\n  readinessProbe:\n    enabled: true\n    initialDelaySeconds: 5\n    periodSeconds: 10\n    timeoutSeconds: 3\n    failureThreshold: 3\n    successThreshold: 1\n  ## @param metrics.startupProbe.enabled Enable startupProbe on Memcached Prometheus exporter containers\n  ## @param metrics.startupProbe.initialDelaySeconds Initial delay seconds for startupProbe\n  ## @param metrics.startupProbe.periodSeconds Period seconds for startupProbe\n  ## @param metrics.startupProbe.timeoutSeconds Timeout seconds for startupProbe\n  ## @param metrics.startupProbe.failureThreshold Failure threshold for startupProbe\n  ## @param metrics.startupProbe.successThreshold Success threshold for startupProbe\n  ##\n  startupProbe:\n    enabled: false\n    initialDelaySeconds: 10\n    periodSeconds: 10\n    timeoutSeconds: 1\n    failureThreshold: 15\n    successThreshold: 1\n  ## @param metrics.customLivenessProbe Custom livenessProbe that overrides the default one\n  ##\n  customLivenessProbe: {}\n  ## @param metrics.customReadinessProbe Custom readinessProbe that overrides the default one\n  ##\n  customReadinessProbe: {}\n  ## @param metrics.customStartupProbe Custom startupProbe that overrides the default one\n  ##\n  customStartupProbe: {}\n  ## @param metrics.podAnnotations [object] Memcached Prometheus exporter pod Annotation and Labels\n  ## ref: https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/\n  ##\n  podAnnotations:\n    prometheus.io/scrape: \"true\"\n    prometheus.io/port: \"{{ .Values.metrics.containerPorts.metrics }}\"\n  ## Service configuration\n  ##\n  service:\n    ## @param metrics.service.ports.metrics Prometheus metrics service port\n    ##\n    ports:\n      metrics: 9150\n    ## @param metrics.service.clusterIP Static clusterIP or None for headless services\n    ## ref: https://kubernetes.io/docs/concepts/services-networking/service/#choosing-your-own-ip-address\n    ##\n    clusterIP: \"\"\n    ## @param metrics.service.sessionAffinity Control where client requests go, to the same pod or round-robin\n    ## Values: ClientIP or None\n    ## ref: https://kubernetes.io/docs/user-guide/services/\n    ##\n    sessionAffinity: None\n    ## @param metrics.service.annotations [object] Annotations for the Prometheus metrics service\n    ##\n    annotations:\n      prometheus.io/scrape: \"true\"\n      prometheus.io/port: \"{{ .Values.metrics.service.ports.metrics }}\"\n  ## Prometheus Operator ServiceMonitor configuration\n  ##\n  serviceMonitor:\n    ## @param metrics.serviceMonitor.enabled Create ServiceMonitor Resource for scraping metrics using Prometheus Operator\n    ##\n    enabled: false\n    ## @param metrics.serviceMonitor.namespace Namespace for the ServiceMonitor Resource (defaults to the Release Namespace)\n    ##\n    namespace: \"\"\n    ## @param metrics.serviceMonitor.interval Interval at which metrics should be scraped.\n    ## ref: https://github.com/coreos/prometheus-operator/blob/master/Documentation/api.md#endpoint\n    ##\n    interval: \"\"\n    ## @param metrics.serviceMonitor.scrapeTimeout Timeout after which the scrape is ended\n    ## ref: https://github.com/coreos/prometheus-operator/blob/master/Documentation/api.md#endpoint\n    ##\n    scrapeTimeout: \"\"\n    ## @param metrics.serviceMonitor.labels Additional labels that can be used so ServiceMonitor will be discovered by Prometheus\n    ##\n    labels: {}\n    ## @param metrics.serviceMonitor.selector Prometheus instance selector labels\n    ## ref: https://github.com/bitnami/charts/tree/main/bitnami/prometheus-operator#prometheus-configuration\n    ##\n    selector: {}\n    ## @param metrics.serviceMonitor.relabelings RelabelConfigs to apply to samples before scraping\n    ##\n    relabelings: []\n    ## @param metrics.serviceMonitor.metricRelabelings MetricRelabelConfigs to apply to samples before ingestion\n    ##\n    metricRelabelings: []\n    ## @param metrics.serviceMonitor.honorLabels Specify honorLabels parameter to add the scrape endpoint\n    ##\n    honorLabels: false\n    ## @param metrics.serviceMonitor.jobLabel The name of the label on the target service to use as the job name in prometheus.\n    ##\n    jobLabel: \"\"\n",
				ReleaseIdentifier: &client.ReleaseIdentifier{
					ReleaseName:      fmt.Sprintf("%s-%d", "test", rand.Int()),
					ReleaseNamespace: "test",
					ClusterConfig:    &client.ClusterConfig{ClusterName: k8sUtils.DEFAULT_CLUSTER},
				},
				IsOCIRepo: true,
				RegistryCredential: &client.RegistryCredential{
					RegistryUrl:  "registry-1.docker.io",
					Username:     "",
					Password:     "",
					AwsRegion:    "",
					AccessKey:    "",
					SecretKey:    "",
					RegistryType: "",
					RepoName:     "bitnamicharts/memcached",
					IsPublic:     true,
				},
			}}},
		{name: "Template function test: private oci chart",
			fields: f,
			args: args{ctx: context.Background(), request: &client.InstallReleaseRequest{
				ChartName:    "memcached-oci",
				ChartVersion: "1.0.5127",
				ValuesYaml:   "ConfigMaps:\n  enabled: false\n  maps: []\nConfigSecrets:\n  enabled: false\n  secrets: []\nContainerPort:\n- envoyPort: 8799\n  idleTimeout: 1800s\n  name: app\n  port: 8080\n  servicePort: 80\n  supportStreaming: false\n  useHTTP2: false\nEnvVariables: []\nEnvVariablesFromFieldPath:\n- fieldPath: metadata.name\n  name: POD_NAME\nGracePeriod: 30\nLivenessProbe:\n  Path: \"\"\n  command: []\n  failureThreshold: 3\n  httpHeaders: []\n  initialDelaySeconds: 20\n  periodSeconds: 10\n  port: 8080\n  scheme: \"\"\n  successThreshold: 1\n  tcp: false\n  timeoutSeconds: 5\nMaxSurge: 1\nMaxUnavailable: 0\nMinReadySeconds: 60\nReadinessProbe:\n  Path: \"\"\n  command: []\n  failureThreshold: 3\n  httpHeaders: []\n  initialDelaySeconds: 20\n  periodSeconds: 10\n  port: 8080\n  scheme: \"\"\n  successThreshold: 1\n  tcp: false\n  timeoutSeconds: 5\nSpec:\n  Affinity:\n    Key: null\n    Values: nodes\n    key: \"\"\nambassadorMapping:\n  ambassadorId: \"\"\n  cors: {}\n  enabled: false\n  hostname: devtron.example.com\n  labels: {}\n  prefix: /\n  retryPolicy: {}\n  rewrite: \"\"\n  tls:\n    context: \"\"\n    create: false\n    hosts: []\n    secretName: \"\"\napp: \"1\"\nappLabels: {}\nappMetrics: false\nargs:\n  enabled: false\n  value:\n  - /bin/sh\n  - -c\n  - touch /tmp/healthy; sleep 30; rm -rf /tmp/healthy; sleep 600\nautoPromotionSeconds: 30\nautoscaling:\n  MaxReplicas: 2\n  MinReplicas: 1\n  TargetCPUUtilizationPercentage: 90\n  TargetMemoryUtilizationPercentage: 80\n  annotations: {}\n  behavior: {}\n  enabled: false\n  extraMetrics: []\n  labels: {}\ncommand:\n  enabled: false\n  value: []\n  workingDir: {}\ncontainerExtraSpecs: {}\ncontainerSecurityContext: {}\ncontainerSpec:\n  lifecycle:\n    enabled: false\n    postStart:\n      httpGet:\n        host: example.com\n        path: /example\n        port: 90\n    preStop:\n      exec:\n        command:\n        - sleep\n        - \"10\"\ncontainers: []\ndbMigrationConfig:\n  enabled: false\ndeployment:\n  strategy:\n    rolling:\n      maxSurge: 25%\n      maxUnavailable: 1\ndeploymentType: ROLLING\nenv: \"4\"\nenvoyproxy:\n  configMapName: \"\"\n  image: quay.io/devtron/envoy:v1.14.1\n  lifecycle: {}\n  resources:\n    limits:\n      cpu: 50m\n      memory: 50Mi\n    requests:\n      cpu: 50m\n      memory: 50Mi\nhostAliases: []\nimage:\n  pullPolicy: IfNotPresent\nimagePullSecrets: []\ningress:\n  annotations: {}\n  className: \"\"\n  enabled: false\n  hosts:\n  - host: chart-example1.local\n    pathType: ImplementationSpecific\n    paths:\n    - /example1\n  labels: {}\n  tls: []\ningressInternal:\n  annotations: {}\n  className: \"\"\n  enabled: false\n  hosts:\n  - host: chart-example1.internal\n    pathType: ImplementationSpecific\n    paths:\n    - /example1\n  - host: chart-example2.internal\n    pathType: ImplementationSpecific\n    paths:\n    - /example2\n    - /example2/healthz\n  tls: []\ninitContainers: []\nistio:\n  enable: false\n  gateway:\n    annotations: {}\n    enabled: false\n    host: example.com\n    labels: {}\n    tls:\n      enabled: false\n      secretName: secret-name\n  virtualService:\n    annotations: {}\n    enabled: false\n    gateways: []\n    hosts: []\n    http:\n    - corsPolicy: {}\n      headers: {}\n      match:\n      - uri:\n          prefix: /v1\n      - uri:\n          prefix: /v2\n      retries:\n        attempts: 2\n        perTryTimeout: 3s\n      rewriteUri: /\n      route:\n      - destination:\n          host: service1\n          port: 80\n      timeout: 12s\n    - route:\n      - destination:\n          host: service2\n    labels: {}\nkedaAutoscaling:\n  advanced: {}\n  authenticationRef: {}\n  cooldownPeriod: 300\n  enabled: false\n  envSourceContainerName: \"\"\n  fallback: {}\n  idleReplicaCount: 0\n  maxReplicaCount: 2\n  minReplicaCount: 1\n  pollingInterval: 30\n  triggerAuthentication:\n    enabled: false\n    name: \"\"\n    spec: {}\n  triggers: []\nnodeSelector: {}\norchestrator.deploymant.algo: 1\npauseForSecondsBeforeSwitchActive: 30\npipelineName: cd-1-jues\npodAnnotations: {}\npodDisruptionBudget: {}\npodExtraSpecs: {}\npodLabels: {}\npodSecurityContext: {}\nprometheus:\n  release: monitoring\nprometheusRule:\n  additionalLabels: {}\n  enabled: false\n  namespace: \"\"\nrawYaml: []\nreleaseVersion: \"6\"\nreplicaCount: 1\nresources:\n  limits:\n    cpu: \"0.05\"\n    memory: 50Mi\n  requests:\n    cpu: \"0.01\"\n    memory: 10Mi\nrolloutAnnotations: {}\nrolloutLabels: {}\nsecret:\n  data: {}\n  enabled: false\nserver:\n  deployment:\n    image: ayushm10/samplerepo\n    image_tag: 042d04e7-1-1\nservice:\n  annotations: {}\n  loadBalancerSourceRanges: []\n  type: ClusterIP\nserviceAccount:\n  annotations: {}\n  create: false\n  name: \"\"\nservicemonitor:\n  additionalLabels: {}\ntolerations: []\ntopologySpreadConstraints: []\nvolumeMounts: []\nvolumes: []\nwaitForSecondsBeforeScalingDown: 30",
				ReleaseIdentifier: &client.ReleaseIdentifier{
					ReleaseName:      fmt.Sprintf("%s-%d", "test", rand.Int()),
					ReleaseNamespace: "test",
					ClusterConfig:    &client.ClusterConfig{ClusterName: k8sUtils.DEFAULT_CLUSTER},
				},
				IsOCIRepo: true,
				RegistryCredential: &client.RegistryCredential{
					RegistryUrl:  "registry-1.docker.io",
					Username:     "ayushm10",
					Password:     "dckr_pat_FQIiOMMQvCxdT2Q_MbdI-K5F7hY",
					AwsRegion:    "",
					AccessKey:    "",
					SecretKey:    "",
					RegistryType: "",
					RepoName:     "ayushm10/sample",
					IsPublic:     false,
				},
			}}},
		{name: "Template function test: public index.yaml helm chart",
			fields: f,
			args: args{ctx: context.Background(), request: &client.InstallReleaseRequest{
				ChartName:    "memcached",
				ChartVersion: "6.5.6",
				ValuesYaml:   "# Copyright VMware, Inc.\n# SPDX-License-Identifier: APACHE-2.0\n\n## @section Global parameters\n## Global Docker image parameters\n## Please, note that this will override the image parameters, including dependencies, configured to use the global value\n## Current available global Docker image parameters: imageRegistry, imagePullSecrets and storageClass\n\n## @param global.imageRegistry Global Docker image registry\n## @param global.imagePullSecrets Global Docker registry secret names as an array\n## @param global.storageClass Global StorageClass for Persistent Volume(s)\n##\nglobal:\n  imageRegistry: \"\"\n  ## E.g.\n  ## imagePullSecrets:\n  ##   - myRegistryKeySecretName\n  ##\n  imagePullSecrets: []\n  storageClass: \"\"\n\n## @section Common parameters\n\n## @param kubeVersion Override Kubernetes version\n##\nkubeVersion: \"\"\n## @param nameOverride String to partially override common.names.fullname template (will maintain the release name)\n##\nnameOverride: \"\"\n## @param fullnameOverride String to fully override common.names.fullname template\n##\nfullnameOverride: \"\"\n## @param clusterDomain Kubernetes Cluster Domain\n##\nclusterDomain: cluster.local\n## @param extraDeploy Extra objects to deploy (evaluated as a template)\n##\nextraDeploy: []\n## @param commonLabels Add labels to all the deployed resources\n##\ncommonLabels: {}\n## @param commonAnnotations Add annotations to all the deployed resources\n##\ncommonAnnotations: {}\n\n## Enable diagnostic mode in the deployment/statefulset\n##\ndiagnosticMode:\n  ## @param diagnosticMode.enabled Enable diagnostic mode (all probes will be disabled and the command will be overridden)\n  ##\n  enabled: false\n  ## @param diagnosticMode.command Command to override all containers in the deployment/statefulset\n  ##\n  command:\n    - sleep\n  ## @param diagnosticMode.args Args to override all containers in the deployment/statefulset\n  ##\n  args:\n    - infinity\n\n## @section Memcached parameters\n\n## Bitnami Memcached image version\n## ref: https://hub.docker.com/r/bitnami/memcached/tags/\n## @param image.registry Memcached image registry\n## @param image.repository Memcached image repository\n## @param image.tag Memcached image tag (immutable tags are recommended)\n## @param image.digest Memcached image digest in the way sha256:aa.... Please note this parameter, if set, will override the tag\n## @param image.pullPolicy Memcached image pull policy\n## @param image.pullSecrets Specify docker-registry secret names as an array\n## @param image.debug Specify if debug values should be set\n##\nimage:\n  registry: docker.io\n  repository: bitnami/memcached\n  tag: 1.6.21-debian-11-r38\n  digest: \"\"\n  ## Specify a imagePullPolicy\n  ## Defaults to 'Always' if image tag is 'latest', else set to 'IfNotPresent'\n  ## ref: https://kubernetes.io/docs/user-guide/images/#pre-pulling-images\n  ##\n  pullPolicy: IfNotPresent\n  ## Optionally specify an array of imagePullSecrets.\n  ## Secrets must be manually created in the namespace.\n  ## ref: https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/\n  ## e.g:\n  ## pullSecrets:\n  ##   - myRegistryKeySecretName\n  ##\n  pullSecrets: []\n  ## Set to true if you would like to see extra information on logs\n  ##\n  debug: false\n## @param architecture Memcached architecture. Allowed values: standalone or high-availability\n##\narchitecture: standalone\n## Authentication parameters\n## ref: https://github.com/bitnami/containers/tree/main/bitnami/memcached#creating-the-memcached-admin-user\n##\nauth:\n  ## @param auth.enabled Enable Memcached authentication\n  ##\n  enabled: false\n  ## @param auth.username Memcached admin user\n  ##\n  username: \"\"\n  ## @param auth.password Memcached admin password\n  ##\n  password: \"\"\n  ## @param auth.existingPasswordSecret Existing secret with Memcached credentials (must contain a value for `memcached-password` key)\n  ##\n  existingPasswordSecret: \"\"\n## @param command Override default container command (useful when using custom images)\n##\ncommand: []\n## @param args Override default container args (useful when using custom images)\n## e.g:\n## args:\n##   - /run.sh\n##   - -m <maxMemoryLimit>\n##   - -I <maxItemSize>\n##   - -vv\n##\nargs: []\n## @param extraEnvVars Array with extra environment variables to add to Memcached nodes\n## e.g:\n## extraEnvVars:\n##   - name: FOO\n##     value: \"bar\"\n##\nextraEnvVars: []\n## @param extraEnvVarsCM Name of existing ConfigMap containing extra env vars for Memcached nodes\n##\nextraEnvVarsCM: \"\"\n## @param extraEnvVarsSecret Name of existing Secret containing extra env vars for Memcached nodes\n##\nextraEnvVarsSecret: \"\"\n\n## @section Deployment/Statefulset parameters\n\n## @param replicaCount Number of Memcached nodes\n##\nreplicaCount: 1\n## @param containerPorts.memcached Memcached container port\n##\ncontainerPorts:\n  memcached: 11211\n## Configure extra options for Memcached containers' liveness, readiness and startup probes\n## ref: https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/#configure-probes\n## @param livenessProbe.enabled Enable livenessProbe on Memcached containers\n## @param livenessProbe.initialDelaySeconds Initial delay seconds for livenessProbe\n## @param livenessProbe.periodSeconds Period seconds for livenessProbe\n## @param livenessProbe.timeoutSeconds Timeout seconds for livenessProbe\n## @param livenessProbe.failureThreshold Failure threshold for livenessProbe\n## @param livenessProbe.successThreshold Success threshold for livenessProbe\n##\nlivenessProbe:\n  enabled: true\n  initialDelaySeconds: 30\n  periodSeconds: 10\n  timeoutSeconds: 5\n  failureThreshold: 6\n  successThreshold: 1\n## @param readinessProbe.enabled Enable readinessProbe on Memcached containers\n## @param readinessProbe.initialDelaySeconds Initial delay seconds for readinessProbe\n## @param readinessProbe.periodSeconds Period seconds for readinessProbe\n## @param readinessProbe.timeoutSeconds Timeout seconds for readinessProbe\n## @param readinessProbe.failureThreshold Failure threshold for readinessProbe\n## @param readinessProbe.successThreshold Success threshold for readinessProbe\n##\nreadinessProbe:\n  enabled: true\n  initialDelaySeconds: 5\n  periodSeconds: 5\n  timeoutSeconds: 3\n  failureThreshold: 6\n  successThreshold: 1\n## @param startupProbe.enabled Enable startupProbe on Memcached containers\n## @param startupProbe.initialDelaySeconds Initial delay seconds for startupProbe\n## @param startupProbe.periodSeconds Period seconds for startupProbe\n## @param startupProbe.timeoutSeconds Timeout seconds for startupProbe\n## @param startupProbe.failureThreshold Failure threshold for startupProbe\n## @param startupProbe.successThreshold Success threshold for startupProbe\n##\nstartupProbe:\n  enabled: false\n  initialDelaySeconds: 30\n  periodSeconds: 10\n  timeoutSeconds: 1\n  failureThreshold: 15\n  successThreshold: 1\n## @param customLivenessProbe Custom livenessProbe that overrides the default one\n##\ncustomLivenessProbe: {}\n## @param customReadinessProbe Custom readinessProbe that overrides the default one\n##\ncustomReadinessProbe: {}\n## @param customStartupProbe Custom startupProbe that overrides the default one\n##\ncustomStartupProbe: {}\n## @param lifecycleHooks for the Memcached container(s) to automate configuration before or after startup\n##\nlifecycleHooks: {}\n## Memcached resource requests and limits\n## ref: https://kubernetes.io/docs/user-guide/compute-resources/\n## @param resources.limits The resources limits for the Memcached containers\n## @param resources.requests.memory The requested memory for the Memcached containers\n## @param resources.requests.cpu The requested cpu for the Memcached containers\n##\nresources:\n  limits: {}\n  requests:\n    memory: 256Mi\n    cpu: 250m\n## Configure Pods Security Context\n## ref: https://kubernetes.io/docs/tasks/configure-pod-container/security-context/#set-the-security-context-for-a-pod\n## @param podSecurityContext.enabled Enabled Memcached pods' Security Context\n## @param podSecurityContext.fsGroup Set Memcached pod's Security Context fsGroup\n##\npodSecurityContext:\n  enabled: true\n  fsGroup: 1001\n## Configure Container Security Context\n## ref: https://kubernetes.io/docs/tasks/configure-pod-container/security-context/#set-the-security-context-for-a-container\n## @param containerSecurityContext.enabled Enabled Memcached containers' Security Context\n## @param containerSecurityContext.runAsUser Set Memcached containers' Security Context runAsUser\n## @param containerSecurityContext.runAsNonRoot Set Memcached containers' Security Context runAsNonRoot\n##\ncontainerSecurityContext:\n  enabled: true\n  runAsUser: 1001\n  runAsNonRoot: true\n## @param hostAliases Add deployment host aliases\n## https://kubernetes.io/docs/concepts/services-networking/add-entries-to-pod-etc-hosts-with-host-aliases/\n##\nhostAliases: []\n## @param podLabels Extra labels for Memcached pods\n## ref: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/\n##\npodLabels: {}\n## @param podAnnotations Annotations for Memcached pods\n## ref: https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/\n##\npodAnnotations: {}\n## @param podAffinityPreset Pod affinity preset. Ignored if `affinity` is set. Allowed values: `soft` or `hard`\n## ref: https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#inter-pod-affinity-and-anti-affinity\n##\npodAffinityPreset: \"\"\n## @param podAntiAffinityPreset Pod anti-affinity preset. Ignored if `affinity` is set. Allowed values: `soft` or `hard`\n## Ref: https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#inter-pod-affinity-and-anti-affinity\n##\npodAntiAffinityPreset: soft\n## Node affinity preset\n## Ref: https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#node-affinity\n##\nnodeAffinityPreset:\n  ## @param nodeAffinityPreset.type Node affinity preset type. Ignored if `affinity` is set. Allowed values: `soft` or `hard`\n  ##\n  type: \"\"\n  ## @param nodeAffinityPreset.key Node label key to match Ignored if `affinity` is set.\n  ## E.g.\n  ## key: \"kubernetes.io/e2e-az-name\"\n  ##\n  key: \"\"\n  ## @param nodeAffinityPreset.values Node label values to match. Ignored if `affinity` is set.\n  ## E.g.\n  ## values:\n  ##   - e2e-az1\n  ##   - e2e-az2\n  ##\n  values: []\n## @param affinity Affinity for pod assignment\n## Ref: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#affinity-and-anti-affinity\n## Note: podAffinityPreset, podAntiAffinityPreset, and nodeAffinityPreset will be ignored when it's set\n##\naffinity: {}\n## @param nodeSelector Node labels for pod assignment\n## Ref: https://kubernetes.io/docs/user-guide/node-selection/\n##\nnodeSelector: {}\n## @param tolerations Tolerations for pod assignment\n## Ref: https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/\n##\ntolerations: []\n## @param topologySpreadConstraints Topology Spread Constraints for pod assignment spread across your cluster among failure-domains. Evaluated as a template\n## Ref: https://kubernetes.io/docs/concepts/workloads/pods/pod-topology-spread-constraints/#spread-constraints-for-pods\n##\ntopologySpreadConstraints: []\n## @param podManagementPolicy StatefulSet controller supports relax its ordering guarantees while preserving its uniqueness and identity guarantees. There are two valid pod management policies: `OrderedReady` and `Parallel`\n## ref: https://kubernetes.io/docs/tutorials/stateful-application/basic-stateful-set/#pod-management-policy\n##\npodManagementPolicy: Parallel\n## @param priorityClassName Name of the existing priority class to be used by Memcached pods, priority class needs to be created beforehand\n## Ref: https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption/\n##\npriorityClassName: \"\"\n## @param schedulerName Kubernetes pod scheduler registry\n## https://kubernetes.io/docs/tasks/administer-cluster/configure-multiple-schedulers/\n##\nschedulerName: \"\"\n## @param terminationGracePeriodSeconds In seconds, time the given to the memcached pod needs to terminate gracefully\n## ref: https://kubernetes.io/docs/concepts/workloads/pods/pod/#termination-of-pods\n##\nterminationGracePeriodSeconds: \"\"\n## @param updateStrategy.type Memcached statefulset strategy type\n## @param updateStrategy.rollingUpdate Memcached statefulset rolling update configuration parameters\n## ref: https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/#update-strategies\n##\nupdateStrategy:\n  type: RollingUpdate\n  rollingUpdate: {}\n## @param extraVolumes Optionally specify extra list of additional volumes for the Memcached pod(s)\n## Example Use Case: mount certificates to enable TLS\n## e.g:\n## extraVolumes:\n## - name: zookeeper-keystore\n##   secret:\n##     defaultMode: 288\n##     secretName: zookeeper-keystore\n## - name: zookeeper-truststore\n##   secret:\n##     defaultMode: 288\n##     secretName: zookeeper-truststore\n##\nextraVolumes: []\n## @param extraVolumeMounts Optionally specify extra list of additional volumeMounts for the Memcached container(s)\n## Example Use Case: mount certificates to enable TLS\n## e.g:\n## extraVolumeMounts:\n## - name: zookeeper-keystore\n##   mountPath: /certs/keystore\n##   readOnly: true\n## - name: zookeeper-truststore\n##   mountPath: /certs/truststore\n##   readOnly: true\n##\nextraVolumeMounts: []\n## @param sidecars Add additional sidecar containers to the Memcached pod(s)\n## e.g:\n## sidecars:\n##   - name: your-image-name\n##     image: your-image\n##     imagePullPolicy: Always\n##     ports:\n##       - name: portname\n##         containerPort: 1234\n##\nsidecars: []\n## @param initContainers Add additional init containers to the Memcached pod(s)\n## Example:\n## initContainers:\n##   - name: your-image-name\n##     image: your-image\n##     imagePullPolicy: Always\n##     ports:\n##       - name: portname\n##         containerPort: 1234\n##\ninitContainers: []\n## Memcached Autoscaling\n## @param autoscaling.enabled Enable memcached statefulset autoscaling (requires architecture: \"high-availability\")\n## @param autoscaling.minReplicas memcached statefulset autoscaling minimum number of replicas\n## @param autoscaling.maxReplicas memcached statefulset autoscaling maximum number of replicas\n## @param autoscaling.targetCPU memcached statefulset autoscaling target CPU percentage\n## @param autoscaling.targetMemory memcached statefulset autoscaling target CPU memory\n##\nautoscaling:\n  enabled: false\n  minReplicas: 3\n  maxReplicas: 6\n  targetCPU: 50\n  targetMemory: 50\n## Memcached Pod Disruption Budget\n## ref: https://kubernetes.io/docs/concepts/workloads/pods/disruptions/\n## @param pdb.create Deploy a pdb object for the Memcached pod\n## @param pdb.minAvailable Minimum available Memcached replicas\n## @param pdb.maxUnavailable Maximum unavailable Memcached replicas\n##\npdb:\n  create: false\n  minAvailable: \"\"\n  maxUnavailable: 1\n\n## @section Traffic Exposure parameters\n\nservice:\n  ## @param service.type Kubernetes Service type\n  ##\n  type: ClusterIP\n  ## @param service.ports.memcached Memcached service port\n  ##\n  ports:\n    memcached: 11211\n  ## Node ports to expose\n  ## NOTE: choose port between <30000-32767>\n  ## @param service.nodePorts.memcached Node port for Memcached\n  ##\n  nodePorts:\n    memcached: \"\"\n  ## @param service.sessionAffinity Control where client requests go, to the same pod or round-robin\n  ## Values: ClientIP or None\n  ## ref: https://kubernetes.io/docs/user-guide/services/\n  ##\n  sessionAffinity: None\n  ## @param service.sessionAffinityConfig Additional settings for the sessionAffinity\n  ## sessionAffinityConfig:\n  ##   clientIP:\n  ##     timeoutSeconds: 300\n  ##\n  sessionAffinityConfig: {}\n  ## @param service.clusterIP Memcached service Cluster IP\n  ## e.g.:\n  ## clusterIP: None\n  ##\n  clusterIP: \"\"\n  ## @param service.loadBalancerIP Memcached service Load Balancer IP\n  ## ref: https://kubernetes.io/docs/user-guide/services/#type-loadbalancer\n  ##\n  loadBalancerIP: \"\"\n  ## @param service.loadBalancerSourceRanges Memcached service Load Balancer sources\n  ## ref: https://kubernetes.io/docs/tasks/access-application-cluster/configure-cloud-provider-firewall/#restrict-access-for-loadbalancer-service\n  ## e.g:\n  ## loadBalancerSourceRanges:\n  ##   - 10.10.10.0/24\n  ##\n  loadBalancerSourceRanges: []\n  ## @param service.externalTrafficPolicy Memcached service external traffic policy\n  ## ref https://kubernetes.io/docs/tasks/access-application-cluster/create-external-load-balancer/#preserving-the-client-source-ip\n  ##\n  externalTrafficPolicy: Cluster\n  ## @param service.annotations Additional custom annotations for Memcached service\n  ##\n  annotations: {}\n  ## @param service.extraPorts Extra ports to expose in the Memcached service (normally used with the `sidecar` value)\n  ##\n  extraPorts: []\n\n## @section Other Parameters\n\n## Service account for Memcached to use.\n## ref: https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/\n##\nserviceAccount:\n  ## @param serviceAccount.create Enable creation of ServiceAccount for Memcached pod\n  ##\n  create: false\n  ## @param serviceAccount.name The name of the ServiceAccount to use.\n  ## If not set and create is true, a name is generated using the common.names.fullname template\n  ##\n  name: \"\"\n  ## @param serviceAccount.automountServiceAccountToken Allows auto mount of ServiceAccountToken on the serviceAccount created\n  ## Can be set to false if pods using this serviceAccount do not need to use K8s API\n  ##\n  automountServiceAccountToken: true\n  ## @param serviceAccount.annotations Additional custom annotations for the ServiceAccount\n  ##\n  annotations: {}\n\n## @section Persistence parameters\n\n## Enable persistence using Persistent Volume Claims\n## ref: https://kubernetes.io/docs/user-guide/persistent-volumes/\n##\npersistence:\n  ## @param persistence.enabled Enable Memcached data persistence using PVC. If false, use emptyDir\n  ##\n  enabled: false\n  ## @param persistence.storageClass PVC Storage Class for Memcached data volume\n  ## If defined, storageClassName: <storageClass>\n  ## If set to \"-\", storageClassName: \"\", which disables dynamic provisioning\n  ## If undefined (the default) or set to null, no storageClassName spec is\n  ##   set, choosing the default provisioner.  (gp2 on AWS, standard on\n  ##   GKE, AWS & OpenStack)\n  ##\n  storageClass: \"\"\n  ## @param persistence.accessModes PVC Access modes\n  ##\n  accessModes:\n    - ReadWriteOnce\n  ## @param persistence.size PVC Storage Request for Memcached data volume\n  ##\n  size: 8Gi\n  ## @param persistence.annotations Annotations for the PVC\n  ##\n  annotations: {}\n  ## @param persistence.labels Labels for the PVC\n  ##\n  labels: {}\n  ## @param persistence.selector Selector to match an existing Persistent Volume for Memcached's data PVC\n  ## If set, the PVC can't have a PV dynamically provisioned for it\n  ## E.g.\n  ## selector:\n  ##   matchLabels:\n  ##     app: my-app\n  ##\n  selector: {}\n\n## @section Volume Permissions parameters\n##\n\n## Init containers parameters:\n## volumePermissions: Change the owner and group of the persistent volume(s) mountpoint(s) to 'runAsUser:fsGroup' on each node\n##\nvolumePermissions:\n  ## @param volumePermissions.enabled Enable init container that changes the owner and group of the persistent volume\n  ##\n  enabled: false\n  ## @param volumePermissions.image.registry Init container volume-permissions image registry\n  ## @param volumePermissions.image.repository Init container volume-permissions image repository\n  ## @param volumePermissions.image.tag Init container volume-permissions image tag (immutable tags are recommended)\n  ## @param volumePermissions.image.digest Init container volume-permissions image digest in the way sha256:aa.... Please note this parameter, if set, will override the tag\n  ## @param volumePermissions.image.pullPolicy Init container volume-permissions image pull policy\n  ## @param volumePermissions.image.pullSecrets Init container volume-permissions image pull secrets\n  ##\n  image:\n    registry: docker.io\n    repository: bitnami/os-shell\n    tag: 11-debian-11-r16\n    digest: \"\"\n    pullPolicy: IfNotPresent\n    ## Optionally specify an array of imagePullSecrets.\n    ## Secrets must be manually created in the namespace.\n    ## ref: https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/\n    ## Example:\n    ## pullSecrets:\n    ##   - myRegistryKeySecretName\n    ##\n    pullSecrets: []\n  ## Init container resource requests and limits\n  ## ref: https://kubernetes.io/docs/user-guide/compute-resources/\n  ## @param volumePermissions.resources.limits Init container volume-permissions resource limits\n  ## @param volumePermissions.resources.requests Init container volume-permissions resource requests\n  ##\n  resources:\n    limits: {}\n    requests: {}\n  ## Init container' Security Context\n  ## Note: the chown of the data folder is done to containerSecurityContext.runAsUser\n  ## and not the below volumePermissions.containerSecurityContext.runAsUser\n  ## @param volumePermissions.containerSecurityContext.runAsUser User ID for the init container\n  ##\n  containerSecurityContext:\n    runAsUser: 0\n\n## Prometheus Exporter / Metrics\n##\nmetrics:\n  ## @param metrics.enabled Start a side-car prometheus exporter\n  ##\n  enabled: false\n  ## Bitnami Memcached Prometheus Exporter image\n  ## ref: https://hub.docker.com/r/bitnami/memcached-exporter/tags/\n  ## @param metrics.image.registry Memcached exporter image registry\n  ## @param metrics.image.repository Memcached exporter image repository\n  ## @param metrics.image.tag Memcached exporter image tag (immutable tags are recommended)\n  ## @param metrics.image.digest Memcached exporter image digest in the way sha256:aa.... Please note this parameter, if set, will override the tag\n  ## @param metrics.image.pullPolicy Image pull policy\n  ## @param metrics.image.pullSecrets Specify docker-registry secret names as an array\n  ##\n  image:\n    registry: docker.io\n    repository: bitnami/memcached-exporter\n    tag: 0.13.0-debian-11-r51\n    digest: \"\"\n    pullPolicy: IfNotPresent\n    ## Optionally specify an array of imagePullSecrets.\n    ## Secrets must be manually created in the namespace.\n    ## ref: https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/\n    ## e.g:\n    ## pullSecrets:\n    ##   - myRegistryKeySecretName\n    ##\n    pullSecrets: []\n  ## @param metrics.containerPorts.metrics Memcached Prometheus Exporter container port\n  ##\n  containerPorts:\n    metrics: 9150\n  ## Memcached Prometheus exporter container resource requests and limits\n  ## ref: https://kubernetes.io/docs/user-guide/compute-resources/\n  ## @param metrics.resources.limits Init container volume-permissions resource limits\n  ## @param metrics.resources.requests Init container volume-permissions resource requests\n  ##\n  resources:\n    limits: {}\n    requests: {}\n  ## Configure Metrics Container Security Context\n  ## ref: https://kubernetes.io/docs/tasks/configure-pod-container/security-context/#set-the-security-context-for-a-container\n  ## @param metrics.containerSecurityContext.enabled Enabled Metrics containers' Security Context\n  ## @param metrics.containerSecurityContext.runAsUser Set Metrics containers' Security Context runAsUser\n  ## @param metrics.containerSecurityContext.runAsNonRoot Set Metrics containers' Security Context runAsNonRoot\n  ##\n  containerSecurityContext:\n    enabled: true\n    runAsUser: 1001\n    runAsNonRoot: true\n  ## Configure extra options for Memcached Prometheus exporter containers' liveness, readiness and startup probes\n  ## ref: https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/#configure-probes\n  ## @param metrics.livenessProbe.enabled Enable livenessProbe on Memcached Prometheus exporter containers\n  ## @param metrics.livenessProbe.initialDelaySeconds Initial delay seconds for livenessProbe\n  ## @param metrics.livenessProbe.periodSeconds Period seconds for livenessProbe\n  ## @param metrics.livenessProbe.timeoutSeconds Timeout seconds for livenessProbe\n  ## @param metrics.livenessProbe.failureThreshold Failure threshold for livenessProbe\n  ## @param metrics.livenessProbe.successThreshold Success threshold for livenessProbe\n  ##\n  livenessProbe:\n    enabled: true\n    initialDelaySeconds: 15\n    periodSeconds: 10\n    timeoutSeconds: 5\n    failureThreshold: 3\n    successThreshold: 1\n  ## @param metrics.readinessProbe.enabled Enable readinessProbe on Memcached Prometheus exporter containers\n  ## @param metrics.readinessProbe.initialDelaySeconds Initial delay seconds for readinessProbe\n  ## @param metrics.readinessProbe.periodSeconds Period seconds for readinessProbe\n  ## @param metrics.readinessProbe.timeoutSeconds Timeout seconds for readinessProbe\n  ## @param metrics.readinessProbe.failureThreshold Failure threshold for readinessProbe\n  ## @param metrics.readinessProbe.successThreshold Success threshold for readinessProbe\n  ##\n  readinessProbe:\n    enabled: true\n    initialDelaySeconds: 5\n    periodSeconds: 10\n    timeoutSeconds: 3\n    failureThreshold: 3\n    successThreshold: 1\n  ## @param metrics.startupProbe.enabled Enable startupProbe on Memcached Prometheus exporter containers\n  ## @param metrics.startupProbe.initialDelaySeconds Initial delay seconds for startupProbe\n  ## @param metrics.startupProbe.periodSeconds Period seconds for startupProbe\n  ## @param metrics.startupProbe.timeoutSeconds Timeout seconds for startupProbe\n  ## @param metrics.startupProbe.failureThreshold Failure threshold for startupProbe\n  ## @param metrics.startupProbe.successThreshold Success threshold for startupProbe\n  ##\n  startupProbe:\n    enabled: false\n    initialDelaySeconds: 10\n    periodSeconds: 10\n    timeoutSeconds: 1\n    failureThreshold: 15\n    successThreshold: 1\n  ## @param metrics.customLivenessProbe Custom livenessProbe that overrides the default one\n  ##\n  customLivenessProbe: {}\n  ## @param metrics.customReadinessProbe Custom readinessProbe that overrides the default one\n  ##\n  customReadinessProbe: {}\n  ## @param metrics.customStartupProbe Custom startupProbe that overrides the default one\n  ##\n  customStartupProbe: {}\n  ## @param metrics.podAnnotations [object] Memcached Prometheus exporter pod Annotation and Labels\n  ## ref: https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/\n  ##\n  podAnnotations:\n    prometheus.io/scrape: \"true\"\n    prometheus.io/port: \"{{ .Values.metrics.containerPorts.metrics }}\"\n  ## Service configuration\n  ##\n  service:\n    ## @param metrics.service.ports.metrics Prometheus metrics service port\n    ##\n    ports:\n      metrics: 9150\n    ## @param metrics.service.clusterIP Static clusterIP or None for headless services\n    ## ref: https://kubernetes.io/docs/concepts/services-networking/service/#choosing-your-own-ip-address\n    ##\n    clusterIP: \"\"\n    ## @param metrics.service.sessionAffinity Control where client requests go, to the same pod or round-robin\n    ## Values: ClientIP or None\n    ## ref: https://kubernetes.io/docs/user-guide/services/\n    ##\n    sessionAffinity: None\n    ## @param metrics.service.annotations [object] Annotations for the Prometheus metrics service\n    ##\n    annotations:\n      prometheus.io/scrape: \"true\"\n      prometheus.io/port: \"{{ .Values.metrics.service.ports.metrics }}\"\n  ## Prometheus Operator ServiceMonitor configuration\n  ##\n  serviceMonitor:\n    ## @param metrics.serviceMonitor.enabled Create ServiceMonitor Resource for scraping metrics using Prometheus Operator\n    ##\n    enabled: false\n    ## @param metrics.serviceMonitor.namespace Namespace for the ServiceMonitor Resource (defaults to the Release Namespace)\n    ##\n    namespace: \"\"\n    ## @param metrics.serviceMonitor.interval Interval at which metrics should be scraped.\n    ## ref: https://github.com/coreos/prometheus-operator/blob/master/Documentation/api.md#endpoint\n    ##\n    interval: \"\"\n    ## @param metrics.serviceMonitor.scrapeTimeout Timeout after which the scrape is ended\n    ## ref: https://github.com/coreos/prometheus-operator/blob/master/Documentation/api.md#endpoint\n    ##\n    scrapeTimeout: \"\"\n    ## @param metrics.serviceMonitor.labels Additional labels that can be used so ServiceMonitor will be discovered by Prometheus\n    ##\n    labels: {}\n    ## @param metrics.serviceMonitor.selector Prometheus instance selector labels\n    ## ref: https://github.com/bitnami/charts/tree/main/bitnami/prometheus-operator#prometheus-configuration\n    ##\n    selector: {}\n    ## @param metrics.serviceMonitor.relabelings RelabelConfigs to apply to samples before scraping\n    ##\n    relabelings: []\n    ## @param metrics.serviceMonitor.metricRelabelings MetricRelabelConfigs to apply to samples before ingestion\n    ##\n    metricRelabelings: []\n    ## @param metrics.serviceMonitor.honorLabels Specify honorLabels parameter to add the scrape endpoint\n    ##\n    honorLabels: false\n    ## @param metrics.serviceMonitor.jobLabel The name of the label on the target service to use as the job name in prometheus.\n    ##\n    jobLabel: \"\"\n",
				ReleaseIdentifier: &client.ReleaseIdentifier{
					ReleaseName:      fmt.Sprintf("%s-%d", "test", rand.Int()),
					ReleaseNamespace: "test",
					ClusterConfig:    &client.ClusterConfig{ClusterName: k8sUtils.DEFAULT_CLUSTER},
				},
				IsOCIRepo:          false,
				RegistryCredential: nil,
				ChartRepository: &client.ChartRepository{
					Name: "memcached",
					Url:  "https://charts.bitnami.com/bitnami",
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
			manifest, err := impl.TemplateChart(tt.args.ctx, tt.args.request)
			lenManifest := len(manifest)
			assert.Nil(t, err)
			assert.NotEqual(t, lenManifest, 0)
		})
	}
}
