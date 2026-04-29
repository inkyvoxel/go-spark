package features

type Flags struct {
	Auth              bool
	PasswordReset     bool
	EmailOutbox       bool
	EmailVerification bool
	EmailChange       bool
	Worker            bool
	Cleanup           bool
}

var Enabled = Flags{
	Auth:              true,
	PasswordReset:     true,
	EmailOutbox:       true,
	EmailVerification: true,
	EmailChange:       true,
	Worker:            true,
	Cleanup:           true,
}
