package services

// EmailVerificationPolicy centralizes how verification affects behavior.
type EmailVerificationPolicy interface {
	Required() bool
	RequiresEmailChangeVerification() bool
	UserIsVerified(user User) bool
}

type requiredEmailVerificationPolicy struct{}

type optionalEmailVerificationPolicy struct{}

func NewEmailVerificationPolicy(required bool) EmailVerificationPolicy {
	if required {
		return requiredEmailVerificationPolicy{}
	}
	return optionalEmailVerificationPolicy{}
}

func DefaultEmailVerificationPolicy() EmailVerificationPolicy {
	return requiredEmailVerificationPolicy{}
}

func (requiredEmailVerificationPolicy) Required() bool {
	return true
}

func (requiredEmailVerificationPolicy) RequiresEmailChangeVerification() bool {
	return true
}

func (requiredEmailVerificationPolicy) UserIsVerified(user User) bool {
	return user.EmailVerifiedAt.Valid
}

func (optionalEmailVerificationPolicy) Required() bool {
	return false
}

func (optionalEmailVerificationPolicy) RequiresEmailChangeVerification() bool {
	return false
}

func (optionalEmailVerificationPolicy) UserIsVerified(User) bool {
	return true
}
