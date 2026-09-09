package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/nduagoziem/homeshr/services/user/internal/cache"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/resend/resend-go/v4"
)

const (
	otpIssuer = "homeshr.com"
	otpPeriod = 300 // 5 minutes
	otpTTL    = 5 * time.Minute
)

var otpValidateOpts = totp.ValidateOpts{
	Period:    otpPeriod,
	Skew:      1,
	Digits:    otp.DigitsSix,
	Algorithm: otp.AlgorithmSHA256,
}

// generateOTP generates an OTP code, stores its TOTP secret briefly, and returns
// the code that should be sent to the user.
func generateOTP(ctx context.Context, email string, redis cache.Cache) (string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      otpIssuer,
		AccountName: email,
		Period:      otpPeriod,
		Algorithm:   otp.AlgorithmSHA256,
	})
	if err != nil {
		return "", err
	}

	code, err := totp.GenerateCodeCustom(key.Secret(), time.Now().UTC(), otpValidateOpts)
	if err != nil {
		return "", err
	}

	if err := redis.Set(ctx, cache.OTPCacheKey(email), key.Secret(), otpTTL); err != nil {
		return "", err
	}

	return code, nil
}

func validateOTP(ctx context.Context, code, email string, redis cache.Cache) (bool, error) {
	secret, err := redis.Get(ctx, cache.OTPCacheKey(email))
	if err != nil {
		return false, err
	}

	valid, err := totp.ValidateCustom(code, secret, time.Now().UTC(), otpValidateOpts)
	if err != nil {
		// ValidateCustom only errors on malformed passcode input (e.g. wrong
		// length); for our purposes that is simply an invalid code, not a
		// system failure — so report it as invalid rather than propagating.
		return false, nil
	}
	if !valid {
		return false, nil
	}

	if err := redis.Delete(ctx, cache.OTPCacheKey(email)); err != nil {
		return false, err
	}

	return true, nil
}

type sendOTPParams struct {
	ctx      context.Context
	redis    cache.Cache
	apiKey   string
	sender   string // homeshr mail address
	receiver string // user mail
}

func sendOTP(params sendOTPParams) error {

	otp, err := generateOTP(params.ctx, params.receiver, params.redis)
	if err != nil {
		return err
	}

	client := resend.NewClient(params.apiKey)

	sendEmail := &resend.SendEmailRequest{
		To:   []string{params.receiver},
		From: params.sender,
		Text: fmt.Sprintf(
			"Your Homeshr verification code is: %s \nThis code expires in %d minutes. \nHomeshr will never ask you for this code.",
			otp, int(otpTTL.Minutes())),
		Subject: "Your Homeshr verification code",
	}

	_, err = client.Emails.Send(sendEmail)
	if err != nil {
		return err
	}

	return nil
}
