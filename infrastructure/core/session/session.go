package session

type AuthKey string

func (s AuthKey) String() string {
	return string(s)
}

const (
	AuthAdminKey  AuthKey = "sess_admin"
	GuestAdminKey AuthKey = "sess_guest"
)
