package paths

const (
	Home                            = "/"
	Healthz                         = "/healthz"
	Readyz                          = "/readyz"
	RobotsTxt                       = "/robots.txt"
	StaticPrefix                    = "/static/"
	StaticFaviconSVG                = StaticPrefix + "favicon.svg"
	StaticPicoCSS                   = StaticPrefix + "vendor/pico/pico.min.css"
	StaticStyles                    = StaticPrefix + "styles.css"
	StaticQRCodeJS                  = StaticPrefix + "vendor/qrcodejs/qrcode.js"
	StaticTOTPSetupJS               = StaticPrefix + "js/totp-setup.js"
	StaticPasskeyJS                 = StaticPrefix + "js/passkey.js"
	Login                           = "/login"
	LoginPasskeyBegin               = Login + "/passkey/begin"
	LoginPasskeyFinish              = Login + "/passkey/finish"
	Register                        = "/register"
	Logout                          = "/logout"
	Account                         = "/account"
	VerifyEmail                     = Account + "/verify-email"
	ConfirmEmail                    = Account + "/confirm-email"
	ForgotPassword                  = Account + "/forgot-password"
	ResetPassword                   = Account + "/reset-password"
	ResendVerification              = Account + "/resend-verification"
	VerifyEmailResend               = VerifyEmail + "/resend"
	ChangePassword                  = Account + "/change-password"
	ChangeEmail                     = Account + "/change-email"
	ConfirmEmailChange              = Account + "/confirm-email-change"
	AccountSessionsRevoke           = Account + "/sessions/revoke"
	AccountSessionsRevokeOthers     = Account + "/sessions/revoke-others"
	AccountDelete                   = Account + "/delete"
	AccountTwoFactor                = Account + "/two-factor"
	AccountTwoFactorSetup           = AccountTwoFactor + "/setup"
	AccountTwoFactorConfirm         = AccountTwoFactor + "/confirm"
	AccountTwoFactorDisable         = AccountTwoFactor + "/disable"
	AccountTwoFactorChallenge       = AccountTwoFactor + "/challenge"
	AccountTwoFactorRegenerateCodes = AccountTwoFactor + "/regenerate-codes"
	AccountPasskeys                 = Account + "/passkeys"
	AccountPasskeysRegisterBegin    = AccountPasskeys + "/register/begin"
	AccountPasskeysRegisterFinish   = AccountPasskeys + "/register/finish"
	AccountPasskeysRename           = AccountPasskeys + "/rename"
	AccountPasskeysDelete           = AccountPasskeys + "/delete"
	// Example feature (projects). Remove with the rest of the example.
	Projects       = "/projects"
	ProjectsDelete = Projects + "/delete"
)

// ponytail: kept deliberately. This map restates the const block above, but
// the two audiences are both real — the consts type Go routing, the map is the
// only way templates reach a route ({{ .Routes.Home }}). Accepted duplication,
// not a finding; do not re-flag in audits.
//
// TemplateRoutes exposes routes to HTML templates. Templates reach a map's
// entries with the same dot syntax used for struct fields ({{ .Routes.Home }}).
var TemplateRoutes = map[string]string{
	"Home":                            Home,
	"StaticFaviconSVG":                StaticFaviconSVG,
	"StaticPicoCSS":                   StaticPicoCSS,
	"StaticStyles":                    StaticStyles,
	"StaticQRCodeJS":                  StaticQRCodeJS,
	"StaticTOTPSetupJS":               StaticTOTPSetupJS,
	"StaticPasskeyJS":                 StaticPasskeyJS,
	"Account":                         Account,
	"Login":                           Login,
	"LoginPasskeyBegin":               LoginPasskeyBegin,
	"LoginPasskeyFinish":              LoginPasskeyFinish,
	"Register":                        Register,
	"Logout":                          Logout,
	"ForgotPassword":                  ForgotPassword,
	"ResetPassword":                   ResetPassword,
	"ResendVerification":              ResendVerification,
	"VerifyEmailResend":               VerifyEmailResend,
	"ChangePassword":                  ChangePassword,
	"ChangeEmail":                     ChangeEmail,
	"ConfirmEmailChange":              ConfirmEmailChange,
	"AccountSessionsRevoke":           AccountSessionsRevoke,
	"AccountSessionsRevokeOthers":     AccountSessionsRevokeOthers,
	"AccountDelete":                   AccountDelete,
	"AccountTwoFactor":                AccountTwoFactor,
	"AccountTwoFactorSetup":           AccountTwoFactorSetup,
	"AccountTwoFactorConfirm":         AccountTwoFactorConfirm,
	"AccountTwoFactorDisable":         AccountTwoFactorDisable,
	"AccountTwoFactorChallenge":       AccountTwoFactorChallenge,
	"AccountTwoFactorRegenerateCodes": AccountTwoFactorRegenerateCodes,
	"AccountPasskeys":                 AccountPasskeys,
	"AccountPasskeysRegisterBegin":    AccountPasskeysRegisterBegin,
	"AccountPasskeysRegisterFinish":   AccountPasskeysRegisterFinish,
	"AccountPasskeysRename":           AccountPasskeysRename,
	"AccountPasskeysDelete":           AccountPasskeysDelete,
	"Projects":                        Projects,
	"ProjectsDelete":                  ProjectsDelete,
}
