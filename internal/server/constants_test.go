package server

import (
	"testing"

	"github.com/inkyvoxel/go-spark/internal/paths"
)

func TestRoutePathConstants(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "home", got: paths.Home, want: "/"},
		{name: "healthz", got: paths.Healthz, want: "/healthz"},
		{name: "static", got: paths.StaticPrefix, want: "/static/"},
		{name: "static pico css", got: paths.StaticPicoCSS, want: "/static/vendor/pico/pico.min.css"},
		{name: "static styles", got: paths.StaticStyles, want: "/static/styles.css"},
		{name: "static htmx", got: paths.StaticHTMX, want: "/static/vendor/htmx/htmx.min.js"},
		{name: "login", got: paths.Login, want: "/login"},
		{name: "register", got: paths.Register, want: "/register"},
		{name: "account", got: paths.Account, want: "/account"},
		{name: "verify-email", got: paths.VerifyEmail, want: "/account/verify-email"},
		{name: "confirm-email", got: paths.ConfirmEmail, want: "/account/confirm-email"},
		{name: "forgot-password", got: paths.ForgotPassword, want: "/account/forgot-password"},
		{name: "reset-password", got: paths.ResetPassword, want: "/account/reset-password"},
		{name: "resend-verification", got: paths.ResendVerification, want: "/account/resend-verification"},
		{name: "verify-email-resend", got: paths.VerifyEmailResend, want: "/account/verify-email/resend"},
		{name: "change-password", got: paths.ChangePassword, want: "/account/change-password"},
		{name: "change-email", got: paths.ChangeEmail, want: "/account/change-email"},
		{name: "confirm-email-change", got: paths.ConfirmEmailChange, want: "/account/confirm-email-change"},
	}

	for _, tt := range tests {
		if tt.got == "" {
			t.Fatalf("%s path is empty", tt.name)
		}
		if tt.got != tt.want {
			t.Fatalf("%s path = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestTemplateRoutesUseCanonicalPaths(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "home", got: paths.TemplateRoutes.Home, want: paths.Home},
		{name: "healthz", got: paths.TemplateRoutes.Healthz, want: paths.Healthz},
		{name: "static pico css", got: paths.TemplateRoutes.StaticPicoCSS, want: paths.StaticPicoCSS},
		{name: "static styles", got: paths.TemplateRoutes.StaticStyles, want: paths.StaticStyles},
		{name: "static htmx", got: paths.TemplateRoutes.StaticHTMX, want: paths.StaticHTMX},
		{name: "account", got: paths.TemplateRoutes.Account, want: paths.Account},
		{name: "login", got: paths.TemplateRoutes.Login, want: paths.Login},
		{name: "register", got: paths.TemplateRoutes.Register, want: paths.Register},
		{name: "logout", got: paths.TemplateRoutes.Logout, want: paths.Logout},
		{name: "forgot-password", got: paths.TemplateRoutes.ForgotPassword, want: paths.ForgotPassword},
		{name: "reset-password", got: paths.TemplateRoutes.ResetPassword, want: paths.ResetPassword},
		{name: "resend-verification", got: paths.TemplateRoutes.ResendVerification, want: paths.ResendVerification},
		{name: "verify-email-resend", got: paths.TemplateRoutes.VerifyEmailResend, want: paths.VerifyEmailResend},
		{name: "change-password", got: paths.TemplateRoutes.ChangePassword, want: paths.ChangePassword},
		{name: "change-email", got: paths.TemplateRoutes.ChangeEmail, want: paths.ChangeEmail},
		{name: "confirm-email-change", got: paths.TemplateRoutes.ConfirmEmailChange, want: paths.ConfirmEmailChange},
	}

	for _, tt := range tests {
		if tt.got == "" {
			t.Fatalf("%s template route is empty", tt.name)
		}
		if tt.got != tt.want {
			t.Fatalf("%s template route = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestQueryDerivedConstants(t *testing.T) {
	if loginPathWithStatusChanged == "" {
		t.Fatal("loginPathWithStatusChanged is empty")
	}

	wantLoginChanged := paths.Login + "?" + queryKeyStatus + "=" + statusChanged
	if loginPathWithStatusChanged != wantLoginChanged {
		t.Fatalf("loginPathWithStatusChanged = %q, want %q", loginPathWithStatusChanged, wantLoginChanged)
	}

	wantResendSent := paths.VerifyEmail + "?" + queryKeyResend + "=" + statusSent
	if got := withQueryParam(paths.VerifyEmail, queryKeyResend, statusSent); got != wantResendSent {
		t.Fatalf("verify-email sent URL = %q, want %q", got, wantResendSent)
	}
}
