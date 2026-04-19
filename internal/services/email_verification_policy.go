package services

import db "github.com/inkyvoxel/go-spark/internal/db/generated"

// EmailVerificationPolicy centralizes how verification affects behavior.
type EmailVerificationPolicy interface {
	Required() bool
	RequiresEmailChangeVerification() bool
	UserIsVerified(user db.User) bool
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

func (requiredEmailVerificationPolicy) UserIsVerified(user db.User) bool {
	return user.EmailVerifiedAt.Valid
}

func (optionalEmailVerificationPolicy) Required() bool {
	return false
}

func (optionalEmailVerificationPolicy) RequiresEmailChangeVerification() bool {
	return false
}

func (optionalEmailVerificationPolicy) UserIsVerified(db.User) bool {
	return true
}
