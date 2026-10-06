package user

import "golang.org/x/crypto/bcrypt"

// bcryptCost matches the production recommendation in Docs/DATABASE.md.
const bcryptCost = 12

func hashPassword(password string) (string, error) {
	digest, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(digest), nil
}

func checkPassword(passwordHash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) == nil
}
