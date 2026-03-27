package auth

import (
	"fmt"
	"github.com/zalando/go-keyring"
)

const serviceName = "chaind-cli"

// SaveCredential stores a sensitive token securely in the system keyring
func SaveCredential(account, token string) error {
	err := keyring.Set(serviceName, account, token)
	if err != nil {
		return fmt.Errorf("failed to save credential to keyring: %w", err)
	}
	return nil
}

// GetCredential retrieves a sensitive token from the system keyring
func GetCredential(account string) (string, error) {
	token, err := keyring.Get(serviceName, account)
	if err != nil {
		return "", fmt.Errorf("credential not found: %w", err)
	}
	return token, nil
}

// DeleteCredential removes the token from the system keyring
func DeleteCredential(account string) error {
	return keyring.Delete(serviceName, account)
}
