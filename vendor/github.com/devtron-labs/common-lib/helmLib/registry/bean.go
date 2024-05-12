package registry

import (
	"github.com/devtron-labs/common-lib/utils/remoteConnection/bean"
	"helm.sh/helm/v3/pkg/registry"
)

type Configuration struct {
	RegistryId                string
	RegistryUrl               string
	Username                  string
	Password                  string
	AwsAccessKey              string
	AwsSecretKey              string
	AwsRegion                 string
	RegistryConnectionType    string //secure, insecure, secure-with-cert
	RegistryCertificateString string
	RegistryCAFilePath        string
	RegistryType              string
	IsPublicRegistry          bool
	RemoteConnectionConfig    *bean.RemoteConnectionConfigBean
}

type Settings struct {
	RegistryClient  *registry.Client
	RegistryHostURL string
}

const (
	REGISTRY_TYPE_ECR                     = "ecr"
	REGISTRYTYPE_GCR                      = "gcr"
	REGISTRYTYPE_ARTIFACT_REGISTRY        = "artifact-registry"
	JSON_KEY_USERNAME              string = "_json_key"
)

const (
	INSECURE_CONNECTION = "insecure"
	SECURE_WITH_CERT    = "secure-with-cert"
)

const (
	REGISTRY_CREDENTIAL_BASE_PATH = "registry-credentials"
)
