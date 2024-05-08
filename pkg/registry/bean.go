package registry

import "helm.sh/helm/v3/pkg/registry"

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
	INSECURE_CONNETION_STRING = "insecure"
	SECURE_WITH_CERT_STRING   = "secure-with-cert"
)

const (
	REGISTRY_CREDENTIAL_BASE_PATH = "registry-credentials"
)
