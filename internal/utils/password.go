package utils

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

// Argon2Params 是 Argon2id 密码哈希参数。
// 这些参数可在未来根据硬件能力和安全策略逐步升级。
type Argon2Params struct {
	Memory      uint32 // 内存成本（KB）
	Iterations  uint32 // 迭代次数
	Parallelism uint8  // 并行度
	SaltLength  uint32 // 盐长度
	KeyLength   uint32 // 派生密钥长度
}

var defaultArgon2Params = Argon2Params{
	Memory:      64 * 1024,
	Iterations:  3,
	Parallelism: 2,
	SaltLength:  16,
	KeyLength:   32,
}

// HashPasswordArgon2ID 使用 Argon2id 对密码进行哈希。
// 返回值为可自描述的 PHC 风格字符串，包含算法参数与盐值，便于后续验证。
func HashPasswordArgon2ID(password string) (string, error) {
	if password == "" {
		return "", errors.New("password is empty")
	}

	salt := make([]byte, defaultArgon2Params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		defaultArgon2Params.Iterations,
		defaultArgon2Params.Memory,
		defaultArgon2Params.Parallelism,
		defaultArgon2Params.KeyLength,
	)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	encoded := fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		defaultArgon2Params.Memory,
		defaultArgon2Params.Iterations,
		defaultArgon2Params.Parallelism,
		b64Salt,
		b64Hash,
	)
	return encoded, nil
}

// VerifyPasswordArgon2ID 校验明文密码与 Argon2id 哈希是否匹配。
func VerifyPasswordArgon2ID(password, encodedHash string) (bool, error) {
	p, salt, decodedHash, err := parseEncodedHash(encodedHash)
	if err != nil {
		return false, err
	}

	computedHash := argon2.IDKey(
		[]byte(password),
		salt,
		p.Iterations,
		p.Memory,
		p.Parallelism,
		p.KeyLength,
	)

	// 常量时间比较，降低计时侧信道风险。
	if subtle.ConstantTimeCompare(decodedHash, computedHash) == 1 {
		return true, nil
	}
	return false, nil
}

func parseEncodedHash(encodedHash string) (Argon2Params, []byte, []byte, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return Argon2Params{}, nil, nil, errors.New("invalid encoded hash format")
	}

	if parts[1] != "argon2id" {
		return Argon2Params{}, nil, nil, errors.New("unsupported algorithm")
	}

	versionPart := strings.TrimPrefix(parts[2], "v=")
	version, err := strconv.Atoi(versionPart)
	if err != nil || version != argon2.Version {
		return Argon2Params{}, nil, nil, errors.New("invalid argon2 version")
	}

	var memory uint32
	var iterations uint32
	var parallelism uint8
	_, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism)
	if err != nil {
		return Argon2Params{}, nil, nil, errors.New("invalid argon2 params")
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return Argon2Params{}, nil, nil, errors.New("invalid salt")
	}

	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return Argon2Params{}, nil, nil, errors.New("invalid hash")
	}

	params := Argon2Params{
		Memory:      memory,
		Iterations:  iterations,
		Parallelism: parallelism,
		SaltLength:  uint32(len(salt)),
		KeyLength:   uint32(len(hash)),
	}

	return params, salt, hash, nil
}
