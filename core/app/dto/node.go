package dto

// NodeInfo is the cosmetic/status view returned by POST /core/nodes/list.
// Field names match the frontend `NodeItem` (api/interface/setting.ts) so the
// existing node switcher/drawer consume it unchanged. No secret material here.
type NodeInfo struct {
	ID          uint   `json:"id"`
	GroupID     uint   `json:"groupID"`
	GroupBelong string `json:"groupBelong"`
	Addr        string `json:"addr"`
	Status      string `json:"status"`
	Version     string `json:"version"`
	IsXpack     bool   `json:"isXpack"`
	IsBound     bool   `json:"isBound"`
	IsFavorite  bool   `json:"isFavorite"`
	Name        string `json:"name"`
}

// SimpleNodeInfo matches the frontend `SimpleNodeItem`. Live metrics require the
// node to be online (filled by the mTLS proxy in a later phase); community
// returns the static identity honestly with zeroed metrics until then.
type SimpleNodeInfo struct {
	ID                uint    `json:"id"`
	Name              string  `json:"name"`
	Addr              string  `json:"addr"`
	Description       string  `json:"description"`
	SystemVersion     string  `json:"systemVersion"`
	SecurityEntrance  string  `json:"securityEntrance"`
	CpuUsedPercent    float64 `json:"cpuUsedPercent"`
	CpuTotal          int     `json:"cpuTotal"`
	MemoryTotal       uint64  `json:"memoryTotal"`
	MemoryUsedPercent float64 `json:"memoryUsedPercent"`
}

// NodeCreate registers a node and mints its single-use enrollment token.
type NodeCreate struct {
	Name    string `json:"name" validate:"required"`
	Addr    string `json:"addr" validate:"required"`
	Port    string `json:"port" validate:"required"`
	GroupID uint   `json:"groupID"`
}

// NodeSearch is the (optional) filter for POST /core/nodes/list.
type NodeSearch struct {
	Type string `json:"type"`
}

// NodeEnrollToken is handed to the admin to paste into the joining node. It is
// what the node needs to bootstrap: the token plus where to reach core.
type NodeEnrollToken struct {
	NodeID     uint   `json:"nodeID"`
	Token      string `json:"token"`
	ExpireAt   int64  `json:"expireAt"`
	Addr       string `json:"addr"`
	Port       string `json:"port"`
	MasterHint string `json:"masterHint"` // optional core address hint for the node
}

// NodeEnrollRequest is posted BY a joining node (token-gated, no session).
type NodeEnrollRequest struct {
	Token string `json:"token" validate:"required"`
	CSR   string `json:"csr" validate:"required"` // PEM CERTIFICATE REQUEST
}

// NodeEnrollResponse is returned to the joining node on success.
type NodeEnrollResponse struct {
	ServerCert            string `json:"serverCert"`            // node leaf, signed by core CA
	CACert                string `json:"caCert"`                // core CA cert (node uses as ClientCAs)
	ProxyID               string `json:"proxyID"`               // write to /etc/1panel/.nodeProxyID
	CoreClientFingerprint string `json:"coreClientFingerprint"` // node pins this to trust core (N6)
}
