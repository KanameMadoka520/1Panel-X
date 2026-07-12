package constant

// Setting keys for the core-owned secure multi-node PKI material. Single source
// of truth shared by the node service and the multi-node proxy helper (kept in
// constant to avoid a service<->xpack import cycle).
const (
	NodeCACertKey       = "NodeCACert"         // CA cert PEM (public)
	NodeCAKeyKey        = "NodeCAKey"          // CA key PEM (encrypted)
	NodeEnrollSecretKey = "NodeEnrollSecret"   // HMAC enrollment secret, base64 (encrypted)
	NodeCoreCertKey     = "NodeCoreClientCert" // core client cert PEM (public)
	NodeCoreKeyKey      = "NodeCoreClientKey"  // core client key PEM (encrypted)
)

// Node lifecycle statuses.
const (
	NodeStatusPending   = "pending"
	NodeStatusEnrolling = "enrolling"
	NodeStatusOnline    = "online"
	NodeStatusOffline   = "offline"
	NodeStatusRevoked   = "revoked"
)
