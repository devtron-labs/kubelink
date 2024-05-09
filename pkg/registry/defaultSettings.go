package registry

import (
	"github.com/devtron-labs/common-lib/utils/remoteConnection/bean"
	client "github.com/devtron-labs/kubelink/grpc"
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

	registryConfig := ConvertToRegistryConfig(registryCredential)

	var caFilePath string
	var err error
	if registryConfig.RegistryConnectionType == SECURE_WITH_CERT {
		caFilePath, err = CreateCertificateFile(registryConfig.RegistryId, registryConfig.RegistryCertificateString)
		if err != nil {
			s.logger.Errorw("error in creating certificate file path", "registryName", registryConfig.RegistryId, "err", err)
			return nil, err
		}
	}

	httpClient, err := getHttpClient(registryConfig, caFilePath)
	if err != nil {
		s.logger.Errorw("error in getting http client", "registryName", registryConfig.RegistryId, "err", err)
	}

	registryClient, err := registry.NewClient(registry.ClientOptHTTPClient(httpClient))
	if err != nil {
		s.logger.Errorw("error in getting registryClient", "registryName", registryConfig.RegistryId, "err", err)
		return nil, err
	}

	if registryCredential != nil && !registryCredential.IsPublic {
		err = OCIRegistryLogin(registryClient, registryCredential, caFilePath)
		if err != nil {
			return nil, err
		}
	}
	return registryClient, nil
}

func getHttpClient(registryConfig *bean.RegistryConfig, caFilePath string) (*http.Client, error) {
	tlsConfig, err := GetTlsConfig(registryConfig, caFilePath)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsConfig},
	}
	return httpClient, nil
}
