// file: internal/database/iface_user.go
// version: 1.0.0
// guid: ca96abf5-5353-428c-aa7f-903b91a481e8

package database

// UserReader is the read-only user slice.
type UserReader interface {
	GetUserByID(id string) (*User, error)
	GetUserByUsername(username string) (*User, error)
	GetUserByEmail(email string) (*User, error)
	ListUsers() ([]User, error)
	CountUsers() (int, error)
}

// UserWriter is the write-only user slice.
type UserWriter interface {
	CreateUser(username, email, passwordHashAlgo, passwordHash string, roles []string, status string) (*User, error)
	UpdateUser(user *User) error
}

// UserStore combines both halves.
type UserStore interface {
	UserReader
	UserWriter
}
