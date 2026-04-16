package services

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	// OWASP Password Storage Cheat Sheet (Argon2id minimum):
	// m=19456 (19 MiB), t=2, p=1.
	defaultArgon2idMemoryKiB  uint32 = 19 * 1024
	defaultArgon2idIterations uint32 = 2
	defaultArgon2idThreads    uint8  = 1
	defaultArgon2idSaltLength uint32 = 16
	defaultArgon2idKeyLength  uint32 = 32
)

var errInvalidPasswordHash = errors.New("invalid password hash")

type passwordHasher interface {
	Hash(password string) (string, error)
	Verify(encodedHash, password string) (bool, error)
}

type argon2idHasher struct {
	memoryKiB  uint32
	iterations uint32
	threads    uint8
	saltLength uint32
	keyLength  uint32
}

func newArgon2idHasher(opts AuthOptions) *argon2idHasher {
	memoryKiB := opts.Argon2idMemoryKiB
	if memoryKiB == 0 {
		memoryKiB = defaultArgon2idMemoryKiB
	}

	iterations := opts.Argon2idIterations
	if iterations == 0 {
		iterations = defaultArgon2idIterations
	}

	threads := opts.Argon2idParallelism
	if threads == 0 {
		threads = defaultArgon2idThreads
	}

	saltLength := opts.Argon2idSaltLength
	if saltLength == 0 {
		saltLength = defaultArgon2idSaltLength
	}

	keyLength := opts.Argon2idKeyLength
	if keyLength == 0 {
		keyLength = defaultArgon2idKeyLength
	}

	return &argon2idHasher{
		memoryKiB:  memoryKiB,
		iterations: iterations,
		threads:    threads,
		saltLength: saltLength,
		keyLength:  keyLength,
	}
}

func (h *argon2idHasher) Hash(password string) (string, error) {
	salt := make([]byte, h.saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, h.iterations, h.memoryKiB, h.threads, h.keyLength)
	encodedSalt := base64.RawStdEncoding.EncodeToString(salt)
	encodedHash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		h.memoryKiB,
		h.iterations,
		h.threads,
		encodedSalt,
		encodedHash,
	), nil
}

func (h *argon2idHasher) Verify(encodedHash, password string) (bool, error) {
	params, salt, hash, err := decodeArgon2idHash(encodedHash)
	if err != nil {
		return false, err
	}

	comparisonHash := argon2.IDKey([]byte(password), salt, params.iterations, params.memoryKiB, params.threads, uint32(len(hash)))
	return subtle.ConstantTimeCompare(hash, comparisonHash) == 1, nil
}

type argon2idHashParams struct {
	memoryKiB  uint32
	iterations uint32
	threads    uint8
}

func decodeArgon2idHash(encoded string) (argon2idHashParams, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return argon2idHashParams{}, nil, nil, errInvalidPasswordHash
	}

	versionRaw, ok := strings.CutPrefix(parts[2], "v=")
	if !ok {
		return argon2idHashParams{}, nil, nil, errInvalidPasswordHash
	}
	version, err := strconv.Atoi(versionRaw)
	if err != nil || version != argon2.Version {
		return argon2idHashParams{}, nil, nil, errInvalidPasswordHash
	}

	params, err := parseArgon2idParams(parts[3])
	if err != nil {
		return argon2idHashParams{}, nil, nil, err
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return argon2idHashParams{}, nil, nil, errInvalidPasswordHash
	}

	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(hash) == 0 {
		return argon2idHashParams{}, nil, nil, errInvalidPasswordHash
	}

	return params, salt, hash, nil
}

func parseArgon2idParams(raw string) (argon2idHashParams, error) {
	paramMap := map[string]string{}
	for _, entry := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" || value == "" {
			return argon2idHashParams{}, errInvalidPasswordHash
		}
		paramMap[key] = value
	}

	memory64, err := strconv.ParseUint(paramMap["m"], 10, 32)
	if err != nil || memory64 == 0 {
		return argon2idHashParams{}, errInvalidPasswordHash
	}
	iterations64, err := strconv.ParseUint(paramMap["t"], 10, 32)
	if err != nil || iterations64 == 0 {
		return argon2idHashParams{}, errInvalidPasswordHash
	}
	threads64, err := strconv.ParseUint(paramMap["p"], 10, 8)
	if err != nil || threads64 == 0 {
		return argon2idHashParams{}, errInvalidPasswordHash
	}

	return argon2idHashParams{
		memoryKiB:  uint32(memory64),
		iterations: uint32(iterations64),
		threads:    uint8(threads64),
	}, nil
}
