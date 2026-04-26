package utils

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// GenerateVerificationCode generates a random 6-digit verification code
func GenerateVerificationCode() string {
	code := make([]byte, 3)
	rand.Read(code)
	return fmt.Sprintf("%06d", int(code[0])<<16|int(code[1])<<8|int(code[2]))[:6]
}

// GenerateJWT generates a JWT token for the given staff member
func GenerateJWT(staffID, firstName, lastName, secretKey string) (string, error) {
	claims := jwt.MapClaims{
		"staff_id":   staffID,
		"first_name": firstName,
		"last_name":  lastName,
		"exp":        time.Now().Add(time.Hour * 24).Unix(),
		"iat":        time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secretKey))
}

// ValidateJWT validates a JWT token and returns the claims
func ValidateJWT(tokenString, secretKey string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secretKey), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// GetCurrentTimestamp returns current Unix timestamp
func GetCurrentTimestamp() int64 {
	return time.Now().Unix()
}
