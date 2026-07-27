// Пакет util содержит низкоуровневые вспомогательные функции и утилиты,
// используемые в разных слоях приложения.
//
// Основная ответственность пакета — безопасная работа с пользовательскими
// идентификаторами в cookies и токенах: подписывание, проверка подписи и
// преобразование между UUID и строковым представлением.
//
// # Безопасность
//
//   - Все cookies и токены подписываются с помощью HMAC-SHA256.
//   - Используется отдельный секретный ключ COOKIE_SECRET из окружения.
//   - При отсутствии ключа используется тестовое значение (только для разработки).
//   - Base64 используется в URL-safe варианте без padding.

package util

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"
)

const (
	// cookieName — имя куки, в которой хранится signed userID.
	cookieName = "user_id"
)

// secretKey — секретный ключ для подписи cookies и токенов авторизации.
//
// Ключ берётся из переменной окружения COOKIE_SECRET. Если переменная не задана,
// используется тестовое значение (подходит только для разработки и тестов).
// При длине ключа менее 32 байт приложение паникует на старте.
//
// secretKey инициализируется один раз при первом обращении через closure.
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

// SetSignedCookie создаёт и устанавливает в ответ signed cookie с идентификатором пользователя.
//
// Функция:
//   - кодирует UUID в base64 (URL-safe),
//   - вычисляет HMAC-SHA256 подпись от payload,
//   - объединяет payload и signature через точку,
//   - устанавливает cookie с флагами HttpOnly, SameSite=Lax и Path=/.
//
// Куки является сессионной (не устанавливается MaxAge/Expires).
//
// Параметры:
//   - w      — http.ResponseWriter, в который будет установлена кука.
//   - userID — идентификатор пользователя для подписи.
func SetSignedCookie(w http.ResponseWriter, userID uuid.UUID) {
	userIDBytes := userID[:]

	payload := encodeBase64(userIDBytes)

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
		HttpOnly: true,  // защита от XSS
		Secure:   false, // только https
		SameSite: http.SameSiteLaxMode,
		// Не ставим Expires/MaxAge → сессионная кука (до закрытия браузера)
		// Если хочешь постоянную — раскомментируй:
		// MaxAge: 365 * 24 * 60 * 60, // год
	})

}

// GetUserIDFromCookie извлекает и проверяет userID из cookie "user_id" запроса.
//
// Функция:
//   - читает куку,
//   - разделяет значение на payload.signature,
//   - проверяет HMAC-подпись,
//   - декодирует payload обратно в UUID.
//
// Возвращает ошибку http.ErrNoCookie при отсутствии куки, неверном формате или
// несовпадении подписи.
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

	var userID uuid.UUID
	copy(userID[:], userIDBytes)
	return userID, nil
}

// GetUserIDFromToken парсит signed token и возвращает userID.
//
// Поддерживает форматы:
//   - "Bearer <signed-token>"
//   - просто "<signed-token>"
//
// Выполняет те же проверки, что и GetUserIDFromCookie: валидация формата,
// проверка HMAC-подписи и декодирование payload в UUID.
//
// При любой ошибке (пустой токен, неверный формат, неверная подпись) функция
// возвращает новый UUID и nil-ошибку. Такое поведение позволяет stateless
// создавать нового пользователя при проблемах с токеном.
func GetUserIDFromToken(token string) (uuid.UUID, error) {
	if token == "" {
		return uuid.Nil, errors.New("empty token")
	}

	// Убираем префикс "Bearer ", если он есть
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimSpace(token)

	if token == "" {
		return uuid.Nil, errors.New("empty token after Bearer prefix")
	}

	// Разделяем payload.signature
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return uuid.New(), nil // невалидный формат — создаём нового пользователя
	}

	payload := parts[0]
	receivedSignature := parts[1]

	// Проверяем подпись
	mac := hmac.New(sha256.New, secretKey)
	mac.Write([]byte(payload))
	expectedSignature := encodeBase64(mac.Sum(nil))

	if !hmac.Equal([]byte(receivedSignature), []byte(expectedSignature)) {
		return uuid.New(), nil // неверная подпись — новый пользователь
	}

	// Декодируем payload в UUID
	userIDBytes, err := decodeBase64(payload)
	if err != nil || len(userIDBytes) != 16 {
		return uuid.New(), nil
	}

	var userID uuid.UUID
	copy(userID[:], userIDBytes)

	return userID, nil
}
