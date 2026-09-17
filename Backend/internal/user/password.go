package user

import "golang.org/x/crypto/bcrypt"

// bcryptCost matches the production recommendation in Docs/DATABASE.md.
const bcryptCost = 12

// hashPassword hashes a plaintext password with bcrypt. The returned
// string contains both the salt and the cost factor, so it can be stored
// as-is in the users.password_hash column.
func hashPassword(password string) (string, error) {
	digest, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(digest), nil
}

// checkPassword reports whether the plaintext password matches the hash.
func checkPassword(passwordHash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) == nil
}
