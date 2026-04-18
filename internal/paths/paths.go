package paths

const (
	Home               = "/"
	Healthz            = "/healthz"
	StaticPrefix       = "/static/"
	StaticPicoCSS      = StaticPrefix + "vendor/pico/pico.min.css"
	StaticStyles       = StaticPrefix + "styles.css"
	StaticHTMX         = StaticPrefix + "vendor/htmx/htmx.min.js"
	Login              = "/login"
	Register           = "/register"
	Logout             = "/logout"
	Account            = "/account"
	VerifyEmail        = Account + "/verify-email"
	ConfirmEmail       = Account + "/confirm-email"
	ForgotPassword     = Account + "/forgot-password"
	ResetPassword      = Account + "/reset-password"
	ResendVerification = Account + "/resend-verification"
	VerifyEmailResend  = VerifyEmail + "/resend"
	ChangePassword     = Account + "/change-password"
	ChangeEmail        = Account + "/change-email"
	ConfirmEmailChange = Account + "/confirm-email-change"
)

type TemplateRouteSet struct {
	Home               string
	Healthz            string
	StaticPicoCSS      string
	StaticStyles       string
	StaticHTMX         string
	Account            string
	Login              string
	Register           string
	Logout             string
	ForgotPassword     string
	ResetPassword      string
	ResendVerification string
	VerifyEmailResend  string
	ChangePassword     string
	ChangeEmail        string
	ConfirmEmailChange string
}

var TemplateRoutes = TemplateRouteSet{
	Home:               Home,
	Healthz:            Healthz,
	StaticPicoCSS:      StaticPicoCSS,
	StaticStyles:       StaticStyles,
	StaticHTMX:         StaticHTMX,
	Account:            Account,
	Login:              Login,
	Register:           Register,
	Logout:             Logout,
	ForgotPassword:     ForgotPassword,
	ResetPassword:      ResetPassword,
	ResendVerification: ResendVerification,
	VerifyEmailResend:  VerifyEmailResend,
	ChangePassword:     ChangePassword,
	ChangeEmail:        ChangeEmail,
	ConfirmEmailChange: ConfirmEmailChange,
}
