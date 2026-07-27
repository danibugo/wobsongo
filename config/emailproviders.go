package config

// Provider defines the supported email delivery backends.
type Provider string

const (
	ProviderMock    Provider = "mock"
	ProviderMailHog Provider = "mailhog"
	ProviderSMTP    Provider = "smtp"
)
