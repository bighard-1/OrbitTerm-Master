package service

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func ValidateRegistrationEmail(value string, allowedDomains []string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if strings.Count(value, "@") != 1 || len(value) > 254 {
		return false
	}
	parts := strings.SplitN(value, "@", 2)
	if parts[0] == "" || strings.ContainsAny(parts[0], " \t\r\n") {
		return false
	}
	domain := parts[1]
	for _, allowed := range allowedDomains {
		if domain == strings.ToLower(strings.TrimSpace(allowed)) {
			return true
		}
	}
	return false
}

func ValidateStrongPassword(password string, minLength int) bool {
	if minLength < 12 {
		minLength = 12
	}
	if utf8.RuneCountInString(password) < minLength || len(password) > 512 {
		return false
	}
	var upper, lower, digit, special bool
	for _, r := range password {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsDigit(r):
			digit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			special = true
		}
	}
	return upper && lower && digit && special
}
