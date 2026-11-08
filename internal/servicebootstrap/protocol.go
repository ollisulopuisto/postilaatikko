// Package servicebootstrap defines the desktop-to-helper startup contract.
package servicebootstrap

// ProtocolVersion identifies the helper bootstrap response contract.
const ProtocolVersion = 1

// Status is the versioned result returned after the helper has checked and
// migrated the service-owned mailbox database.
type Status struct {
	ProtocolVersion  int    `json:"protocolVersion"`
	APIVersion       string `json:"apiVersion"`
	SchemaVersion    int    `json:"schemaVersion"`
	MaxSchemaVersion int    `json:"maxSchemaVersion"`
}
