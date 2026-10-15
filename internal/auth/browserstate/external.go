package browserstate

import (
	"time"

	"github.com/assurrussa/goauth"
)

// ExternalProof is verified provider time, never the time a response was received.
// Validity confirms the admitted immutable snapshot. Fresh provider proof needs
// explicit admission; neither issue time nor deadline slides on a read.
type ExternalProof struct{ IssuedAt, ExpiresAt time.Time }

// ExternalBinding is server-side metadata, never a browser credential. LinkID
// pins the canonical link INSTANCE so unlink/relink cannot revive old sessions.
// AuthorityID references the host's encrypted durable provider-state journal.
type ExternalBinding struct {
	Issuer, Subject, LinkID, AuthorityID, ProviderSessionID string
	ClientID, ProjectID                                     string
	ExternalAuthTime                                        time.Time
	Generation                                              uint64
	LoginGeneration                                         uint64
	Deadline                                                time.Time
	LocalSubject                                            goauth.SubjectID
	AbsoluteUntil                                           time.Time
	Proof                                                   ExternalProof
}
