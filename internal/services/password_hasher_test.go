package services

import "testing"

func TestArgon2idHasherHashAndVerify(t *testing.T) {
	hasher := newArgon2idHasher(AuthOptions{
		Argon2idMemoryKiB:   64,
		Argon2idIterations:  1,
		Argon2idParallelism: 1,
		Argon2idSaltLength:  16,
		Argon2idKeyLength:   32,
	})

	hash, err := hasher.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}

	ok, err := hasher.Verify(hash, "correct horse battery staple")
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !ok {
		t.Fatal("Verify() = false, want true")
	}

	ok, err = hasher.Verify(hash, "wrong password")
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if ok {
		t.Fatal("Verify() = true, want false")
	}
}

func TestArgon2idHasherRejectsInvalidHashFormat(t *testing.T) {
	hasher := newArgon2idHasher(AuthOptions{})

	if _, err := hasher.Verify("$2a$10$not-argon", "password"); err == nil {
		t.Fatal("Verify() error = nil, want error")
	}
}
