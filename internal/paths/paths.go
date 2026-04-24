package paths

const (
	Home                        = "/"
	Healthz                     = "/healthz"
	Readyz                      = "/readyz"
	StaticPrefix                = "/static/"
	StaticPicoCSS               = StaticPrefix + "vendor/pico/pico.min.css"
	StaticStyles                = StaticPrefix + "styles.css"
	Login                       = "/login"
	Register                    = "/register"
	Logout                      = "/logout"
	Account                     = "/account"
	VerifyEmail                 = Account + "/verify-email"
	ConfirmEmail                = Account + "/confirm-email"
	ForgotPassword              = Account + "/forgot-password"
	ResetPassword               = Account + "/reset-password"
	ResendVerification          = Account + "/resend-verification"
	VerifyEmailResend           = VerifyEmail + "/resend"
	ChangePassword              = Account + "/change-password"
	ChangeEmail                 = Account + "/change-email"
	ConfirmEmailChange          = Account + "/confirm-email-change"
	AccountSessionsRevoke       = Account + "/sessions/revoke"
	AccountSessionsRevokeOthers = Account + "/sessions/revoke-others"
)

type TemplateRouteSet struct {
	Home                        string
	StaticPicoCSS               string
	StaticStyles                string
	Account                     string
	Login                       string
	Register                    string
	Logout                      string
	ForgotPassword              string
	ResetPassword               string
	ResendVerification          string
	VerifyEmailResend           string
	ChangePassword              string
	ChangeEmail                 string
	ConfirmEmailChange          string
	AccountSessionsRevoke       string
	AccountSessionsRevokeOthers string
}

var TemplateRoutes = TemplateRouteSet{
	Home:                        Home,
	StaticPicoCSS:               StaticPicoCSS,
	StaticStyles:                StaticStyles,
	Account:                     Account,
	Login:                       Login,
	Register:                    Register,
	Logout:                      Logout,
	ForgotPassword:              ForgotPassword,
	ResetPassword:               ResetPassword,
	ResendVerification:          ResendVerification,
	VerifyEmailResend:           VerifyEmailResend,
	ChangePassword:              ChangePassword,
	ChangeEmail:                 ChangeEmail,
	ConfirmEmailChange:          ConfirmEmailChange,
	AccountSessionsRevoke:       AccountSessionsRevoke,
	AccountSessionsRevokeOthers: AccountSessionsRevokeOthers,
}
