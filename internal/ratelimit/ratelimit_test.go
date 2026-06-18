package ratelimit

import "testing"

func TestResolveFillsDefaultsAndAppliesOverrides(t *testing.T) {
	resolved := Resolve(Policies{
		// Override only MaxRequests; Window must fall back to the default.
		Login: {MaxRequests: 99},
	})

	if len(resolved) != len(Names) {
		t.Fatalf("resolved has %d policies, want %d", len(resolved), len(Names))
	}
	if got := resolved[Login]; got.MaxRequests != 99 || got.Window != Defaults[Login].Window {
		t.Fatalf("Login = %+v, want MaxRequests=99 with default window %v", got, Defaults[Login].Window)
	}
	if resolved[Register] != Defaults[Register] {
		t.Fatalf("Register = %+v, want default %+v", resolved[Register], Defaults[Register])
	}
}

func TestEveryNameHasADefault(t *testing.T) {
	if len(Names) != len(Defaults) {
		t.Fatalf("Names has %d entries, Defaults has %d", len(Names), len(Defaults))
	}
	for _, name := range Names {
		policy, ok := Defaults[name]
		if !ok {
			t.Fatalf("no default for %q", name)
		}
		if policy.MaxRequests <= 0 || policy.Window <= 0 {
			t.Fatalf("default for %q is not positive: %+v", name, policy)
		}
	}
}

func TestEnvPrefix(t *testing.T) {
	cases := map[Name]string{
		Login:                   "RATE_LIMIT_LOGIN",
		LoginPerIP:              "RATE_LIMIT_LOGIN_PER_IP",
		PublicResendVerifyPerIP: "RATE_LIMIT_PUBLIC_RESEND_VERIFICATION_PER_IP",
		TOTPRegenerateCodes:     "RATE_LIMIT_TOTP_REGENERATE_CODES",
	}
	for name, want := range cases {
		if got := EnvPrefix(name); got != want {
			t.Fatalf("EnvPrefix(%q) = %q, want %q", name, got, want)
		}
	}
}
