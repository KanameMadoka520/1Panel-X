package model

// Node is a registered remote 1Panel-X host agent (a "node") that core
// federates over mutual TLS. Secret material is never serialised to JSON (the
// frontend only ever sees the cosmetic/status subset via dto.NodeInfo).
type Node struct {
	BaseModel
	Name    string `gorm:"not null;unique" json:"name"`
	Addr    string `json:"addr"`
	Port    string `json:"port"`
	Status  string `json:"status"` // pending, online, offline, revoked
	Scope   string `json:"scope"`  // node
	GroupID uint   `json:"groupID"`
	Version string `json:"version"`

	// PKI / enrollment state (server-side only).
	ServerFingerprint string `json:"-"` // pinned SHA-256 of the node's leaf server cert
	ProxyID           string `json:"-"` // per-node secret written to the node's /etc/1panel/.nodeProxyID

	// Single-use enrollment token bookkeeping; the row is burned on redemption.
	EnrollNonce  string `json:"-"`
	EnrollExpire int64  `json:"-"`
	Enrolled     bool   `json:"-"`
}
