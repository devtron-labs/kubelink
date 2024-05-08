package registry

import (
	client "github.com/devtron-labs/kubelink/grpc"
)

type SettingsGetter interface {
	GetRegistrySettings(registryCredential *client.RegistryCredential) (*Settings, error)
}

type SettingsFactory interface {
	GetSettings(registryCredential *client.RegistryCredential) SettingsGetter
}

type SettingsFactoryImpl struct {
	DefaultSettings DefaultSettingsGetter
}

func NewSettingsFactoryImpl(
	DefaultSettings DefaultSettingsGetter,
) *SettingsFactoryImpl {
	return &SettingsFactoryImpl{
		DefaultSettings: DefaultSettings,
	}
}

func (impl SettingsFactoryImpl) GetSettings(registryCredential *client.RegistryCredential) SettingsGetter {
	return impl.DefaultSettings
}
