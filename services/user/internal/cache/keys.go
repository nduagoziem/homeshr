package cache

import "strings"

// OTPCacheKey generates a unique cache key for storing the OTP secret associated with a user's email.
func OTPCacheKey(email string) string {
	return "auth:otp:secret:" + strings.ToLower(email)
}
