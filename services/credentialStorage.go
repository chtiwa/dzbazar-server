package services

// Encrypt-on-write helpers for third-party API credentials, so controllers
// never write plaintext Pixel.AccessToken / DeliveryCompany.Token/MerchantID
// to the DB directly (see server/services/encryption.go for the cipher).
// These stay DB-agnostic (no *gorm.DB) so callers can use them inside an
// existing transaction rather than being forced onto a fresh connection.

// EncryptPixelAccessToken encrypts a non-empty access token for storage.
// Empty input (no token / clearing the token) passes through unchanged.
func EncryptPixelAccessToken(rawAccessToken string) (string, error) {
	if rawAccessToken == "" {
		return "", nil
	}
	return EncryptField(rawAccessToken)
}

// EncryptDeliveryCredentials encrypts a non-empty carrier token/merchant ID
// pair for storage. Empty input passes through unchanged.
func EncryptDeliveryCredentials(rawToken, rawMerchantID string) (token string, merchantID string, err error) {
	token, err = EncryptField(rawToken)
	if err != nil {
		return "", "", err
	}
	merchantID, err = EncryptField(rawMerchantID)
	if err != nil {
		return "", "", err
	}
	return token, merchantID, nil
}
