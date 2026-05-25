package auth

import "time"

// NewServiceForTest constructs a Service with custom store implementations.
// Only test code should use this; production should call NewService.
func NewServiceForTest(users UserStore, refresh RefreshStore, jwtSecret string, now func() time.Time) *Service {
	return newServiceImpl(users, refresh, jwtSecret, now)
}

// UserStore is the public alias for userStore so external test packages can
// implement it.
type UserStore = userStore

// RefreshStore is the public alias for refreshStore for the same reason.
type RefreshStore = refreshStore
