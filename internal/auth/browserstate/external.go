package browserstate

import (
	"time"

	"github.com/assurrussa/goauth"
)

// ExternalProof is verified provider time, never the time a response was received.
// Validity confirms the admitted immutable snapshot. Fresh provider proof needs
// explicit admission; neither issue time nor deadline slides on a read.
type ExternalProof struct {
	IssuedAt  time.Time `json:"issuedAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// ExternalBinding is encrypted server-side metadata, never a browser credential.
// LinkID pins the canonical link instance; AuthorityID pins the provider journal.
type ExternalBinding struct {
	Issuer            string           `json:"issuer"`
	Subject           string           `json:"subject"`
	LinkID            string           `json:"linkId"`
	AuthorityID       string           `json:"authorityId"`
	ProviderSessionID string           `json:"providerSessionId"`
	ClientID          string           `json:"clientId"`
	ProjectID         string           `json:"projectId"`
	ExternalAuthTime  time.Time        `json:"externalAuthTime"`
	Generation        uint64           `json:"generation"`
	LoginGeneration   uint64           `json:"loginGeneration"`
	Deadline          time.Time        `json:"deadline"`
	LocalSubject      goauth.SubjectID `json:"localSubject"`
	AbsoluteUntil     time.Time        `json:"absoluteUntil"`
	Proof             ExternalProof    `json:"proof"`
}
