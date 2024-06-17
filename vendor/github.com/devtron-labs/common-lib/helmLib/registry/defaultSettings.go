package registry

import (
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

func (s *DefaultSettingsGetterImpl) GetRegistrySettings(config *Configuration) (*Settings, error) {

	registryClient, err := s.getRegistryClient(config)
	if err != nil {
		s.logger.Error("error in getting registry client", "registryUrl", config.RegistryUrl, "err", err)
		return nil, err
	}

	return &Settings{
		RegistryClient:         registryClient,
		RegistryHostURL:        config.RegistryUrl,
		RegistryConnectionType: REGISTRY_CONNECTION_TYPE_DIRECT,
		HttpClient:             nil,
		Header:                 nil,
	}, nil
}

func (s *DefaultSettingsGetterImpl) getRegistryClient(config *Configuration) (*registry.Client, error) {

	httpClient, err := GetHttpClient(config)
	if err != nil {
		s.logger.Errorw("error in getting http client", "registryName", config.RegistryId, "err", err)
		return nil, err
	}

	clientOptions := []registry.ClientOption{registry.ClientOptHTTPClient(httpClient)}
	if config.RegistryConnectionType == INSECURE_CONNECTION {
		clientOptions = append(clientOptions, registry.ClientOptPlainHTTP())
	}

	registryClient, err := registry.NewClient(clientOptions...)
	if err != nil {
		s.logger.Errorw("error in getting registryClient", "registryName", config.RegistryId, "err", err)
		return nil, err
	}

	if config != nil && !config.IsPublicRegistry {
		registryClient, err = GetLoggedInClient(registryClient, config)
		if err != nil {
			return nil, err
		}
	}
	return registryClient, nil
}
