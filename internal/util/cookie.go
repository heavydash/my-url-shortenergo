package util

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"github.com/google/uuid"
	"net/http"
	"os"
	"strings"
)

const (
	cookieName = "user_id"
)

// SecretKey
var secretKey = func() []byte {
	key := os.Getenv("COOKIE_SECRET")
	if key == "" {
		key = "yandex_practicum_test_secret_2025_1234567890123456"
	}
	if len(key) < 32 {
		panic("COOKIE_SECRET too short")
	}
	return []byte(key)
}()

// Кодировка для URL и Cookie
func encodeBase64(data []byte) string {
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(data)
}

// Декодирует base64, игнорируя padding
func decodeBase64(s string) ([]byte, error) {
	return base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(s)
}

// Создать и установить куку
func SetSignedCookie(w http.ResponseWriter, userID uuid.UUID) {
	// Превращаем UUID в []byte (16 байт)
	var userIDBytes [16]byte
	copy(userIDBytes[:], userID[:])

	//
	payload := encodeBase64(userIDBytes[:])

	// HMAc-SHA256 от payload
	mac := hmac.New(sha256.New, secretKey)
	mac.Write([]byte(payload))
	signature := encodeBase64(mac.Sum(nil))

	// Склеиваем payload и signature
	cookieValue := payload + "." + signature

	// Устанавливаем куку с максимальной безопасностью
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    cookieValue,
		Path:     "/",
		HttpOnly: true, // защита от XSS
		Secure:   true, // только https
		SameSite: http.SameSiteLaxMode,
		// Не ставим Expires/MaxAge → сессионная кука (до закрытия браузера)
		// Если хочешь постоянную — раскомментируй:
		// MaxAge: 365 * 24 * 60 * 60, // год
	})

}

func GetUserIDFromCookie(r *http.Request) (uuid.UUID, error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return uuid.Nil, err //нет куки
	}

	parts := strings.SplitN(cookie.Value, ".", 2)
	if len(parts) != 2 {
		return uuid.Nil, http.ErrNoCookie
	}

	payload := parts[0]
	receivedSignature := parts[1]

	// проверяем подпись
	mac := hmac.New(sha256.New, secretKey)
	mac.Write([]byte(payload))
	expectedSignature := encodeBase64(mac.Sum(nil))

	if !hmac.Equal([]byte(receivedSignature), []byte(expectedSignature)) {
		return uuid.Nil, http.ErrNoCookie // подпись не совпала
	}

	// Декодируем payload -> 16bytes -> UUID
	userIDBytes, err := decodeBase64(payload)
	if err != nil || len(userIDBytes) != 16 {
		return uuid.Nil, http.ErrNoCookie
	}

	userID, err := uuid.FromBytes(userIDBytes)
	if err != nil {
		return uuid.Nil, http.ErrNoCookie
	}

	return userID, nil
}
