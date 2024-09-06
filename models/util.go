package models

import "golang.org/x/crypto/bcrypt"

var HashPassword func(password string) (string, error) = hashPassword
var IsPasswordValid func(password, original string) bool = isPasswordValid

func hashPassword(password string) (string, error) {
	// Generate a bcrypt hash
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	// Convert the hashed bytes to a string
	hashedPassword := string(hashedBytes)

	return hashedPassword, nil
}

func isPasswordValid(password, original string) bool {
	// Compare the password with the hash
	err := bcrypt.CompareHashAndPassword([]byte(original), []byte(password))
	return err == nil
}
