package reconcile

const Oauth2proxyFinalizer = "oauth2proxy.oauth2proxy-operator.dexter0195.com/finalizer"

// Definitions to manage status conditions
const (
	// typeAvailableOAuth2Proxy represents the status of the Deployment reconciliation
	TypeAvailableOAuth2Proxy = "Available"
	// typeDegradedOAuth2Proxy represents the status used when the custom resource is deleted and the finalizer operations are must to occur.
	TypeDegradedOAuth2Proxy = "Degraded"
)
