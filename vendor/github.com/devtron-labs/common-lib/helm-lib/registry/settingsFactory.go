package registry

type SettingsGetter interface {
	GetRegistrySettings(config *Configuration) (*Settings, error)
}

type SettingsFactory interface {
	GetSettings(config *Configuration) SettingsGetter
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

func (impl SettingsFactoryImpl) GetSettings(config *Configuration) SettingsGetter {
	return impl.DefaultSettings
}
