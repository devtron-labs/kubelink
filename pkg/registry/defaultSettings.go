package registry

import (
	client "github.com/devtron-labs/kubelink/grpc"
	"go.uber.org/zap"
	"helm.sh/helm/v3/pkg/registry"
)

type DefaultSettingsGetter interface {
	SettingsGetter
}

type DefaultSettingsGetterImpl struct {
	logger *zap.SugaredLogger
}

func NewDefaultSettingsGetter(logger *zap.SugaredLogger) *DefaultSettingsGetterImpl {
	return &DefaultSettingsGetterImpl{
		logger: logger,
	}
}

func (s *DefaultSettingsGetterImpl) GetRegistrySettings(registryCredential *client.RegistryCredential) (*Settings, error) {

	registryClient, err := s.getRegistryClient(registryCredential)
	if err != nil {
		s.logger.Error("error in getting registry client", "registryUrl", registryCredential.RegistryUrl, "err", err)
		return nil, err
	}

	return &Settings{
		RegistryClient:  registryClient,
		RegistryHostURL: registryCredential.RegistryUrl,
	}, nil
}

func (s *DefaultSettingsGetterImpl) getRegistryClient(registryCredential *client.RegistryCredential) (*registry.Client, error) {
	registryClient, err := registry.NewClient()
	if err != nil {
		return nil, err
	}
	if registryCredential != nil && !registryCredential.IsPublic {
		err = OCIRegistryLogin(registryClient, registryCredential)
		if err != nil {
			return nil, err
		}
	}
	return registryClient, nil
}
