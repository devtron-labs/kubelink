package registry

import (
	client "github.com/devtron-labs/kubelink/grpc"
	"helm.sh/helm/v3/pkg/registry"
)

type ClientGetter interface {
	GetRegistryClient(registryCredential *client.RegistryCredential) (*registry.Client, error)
	GetRegistryHostURl(OCIRegistryRequest *client.RegistryCredential) (string, error)
}

type ClientGetterImpl struct {
}

func NewClientGetterImpl() *ClientGetterImpl {
	return &ClientGetterImpl{}
}

func (c *ClientGetterImpl) GetRegistryClient(registryCredential *client.RegistryCredential) (*registry.Client, error) {
	registryClient, err := registry.NewClient()
	if err != nil {
		return nil, err
	}
	return registryClient, nil
}

func (c *ClientGetterImpl) GetRegistryHostURl(registryCredential *client.RegistryCredential) (string, error) {
	//for enterprise -  registry url will be modified if server connection method is ss
	return registryCredential.RegistryUrl, nil
}
