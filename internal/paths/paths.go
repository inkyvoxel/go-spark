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
)

type TemplateRouteSet struct {
	Home                            string
	StaticFaviconSVG                string
	StaticPicoCSS                   string
	StaticStyles                    string
	StaticQRCodeJS                  string
	StaticTOTPSetupJS               string
	StaticPasskeyJS                 string
	Account                         string
	Login                           string
	LoginPasskeyBegin               string
	LoginPasskeyFinish              string
	Register                        string
	Logout                          string
	ForgotPassword                  string
	ResetPassword                   string
	ResendVerification              string
	VerifyEmailResend               string
	ChangePassword                  string
	ChangeEmail                     string
	ConfirmEmailChange              string
	AccountSessionsRevoke           string
	AccountSessionsRevokeOthers     string
	AccountDelete                   string
	AccountTwoFactor                string
	AccountTwoFactorSetup           string
	AccountTwoFactorConfirm         string
	AccountTwoFactorDisable         string
	AccountTwoFactorChallenge       string
	AccountTwoFactorRegenerateCodes string
	AccountPasskeys                 string
	AccountPasskeysRegisterBegin    string
	AccountPasskeysRegisterFinish   string
	AccountPasskeysRename           string
	AccountPasskeysDelete           string
}

var TemplateRoutes = TemplateRouteSet{
	Home:                            Home,
	StaticFaviconSVG:                StaticFaviconSVG,
	StaticPicoCSS:                   StaticPicoCSS,
	StaticStyles:                    StaticStyles,
	StaticQRCodeJS:                  StaticQRCodeJS,
	StaticTOTPSetupJS:               StaticTOTPSetupJS,
	StaticPasskeyJS:                 StaticPasskeyJS,
	Account:                         Account,
	Login:                           Login,
	LoginPasskeyBegin:               LoginPasskeyBegin,
	LoginPasskeyFinish:              LoginPasskeyFinish,
	Register:                        Register,
	Logout:                          Logout,
	ForgotPassword:                  ForgotPassword,
	ResetPassword:                   ResetPassword,
	ResendVerification:              ResendVerification,
	VerifyEmailResend:               VerifyEmailResend,
	ChangePassword:                  ChangePassword,
	ChangeEmail:                     ChangeEmail,
	ConfirmEmailChange:              ConfirmEmailChange,
	AccountSessionsRevoke:           AccountSessionsRevoke,
	AccountSessionsRevokeOthers:     AccountSessionsRevokeOthers,
	AccountDelete:                   AccountDelete,
	AccountTwoFactor:                AccountTwoFactor,
	AccountTwoFactorSetup:           AccountTwoFactorSetup,
	AccountTwoFactorConfirm:         AccountTwoFactorConfirm,
	AccountTwoFactorDisable:         AccountTwoFactorDisable,
	AccountTwoFactorChallenge:       AccountTwoFactorChallenge,
	AccountTwoFactorRegenerateCodes: AccountTwoFactorRegenerateCodes,
	AccountPasskeys:                 AccountPasskeys,
	AccountPasskeysRegisterBegin:    AccountPasskeysRegisterBegin,
	AccountPasskeysRegisterFinish:   AccountPasskeysRegisterFinish,
	AccountPasskeysRename:           AccountPasskeysRename,
	AccountPasskeysDelete:           AccountPasskeysDelete,
}
