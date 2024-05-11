package registry

import (
	"go.uber.org/zap"
	"helm.sh/helm/v3/pkg/registry"
	"net/http"
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
		RegistryClient:  registryClient,
		RegistryHostURL: config.RegistryUrl,
	}, nil
}

func (s *DefaultSettingsGetterImpl) getRegistryClient(config *Configuration) (*registry.Client, error) {

	var caFilePath string
	var err error
	if config.RegistryConnectionType == SECURE_WITH_CERT {
		caFilePath, err = CreateCertificateFile(config.RegistryId, config.RegistryCertificateString)
		if err != nil {
			s.logger.Errorw("error in creating certificate file path", "registryName", config.RegistryId, "err", err)
			return nil, err
		}
	}

	config.RegistryCAFilePath = caFilePath
	httpClient, err := getHttpClient(config)
	if err != nil {
		s.logger.Errorw("error in getting http client", "registryName", config.RegistryId, "err", err)
		return nil, err
	}

	registryClient, err := registry.NewClient(registry.ClientOptHTTPClient(httpClient))
	if err != nil {
		s.logger.Errorw("error in getting registryClient", "registryName", config.RegistryId, "err", err)
		return nil, err
	}

	if config != nil && !config.IsPublicRegistry {
		err = OCIRegistryLogin(registryClient, config)
		if err != nil {
			return nil, err
		}
	}
	return registryClient, nil
}

func getHttpClient(config *Configuration) (*http.Client, error) {
	if len(config.RegistryCAFilePath) == 0 {
		caFilePath, err := CreateCertificateFile(config.RegistryId, config.RegistryCertificateString)
		if err != nil {
			return nil, err
		}
		config.RegistryCAFilePath = caFilePath
	}
	tlsConfig, err := GetTlsConfig(config)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsConfig},
	}
	return httpClient, nil
}
