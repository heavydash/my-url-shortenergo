// Package api содержит модели запросов и ответов для HTTP API.
//
// Эти структуры используются как для генерации Swagger-документации,
// так и в обработчиках. Все поля снабжены тегами json, example и validate
// для автоматической генерации спецификации OpenAPI.
package api

// ShortenRequest — запрос на создание короткой ссылки (POST /api/shorten).
//
// Используется в JSON-эндпоинте сокращения одной ссылки.
type ShortenRequest struct {
	// URL — полный оригинальный адрес, который нужно сократить.
	// Должен быть валидным URL (http/https).
	URL string `json:"url" example:"https://www.google.com" validate:"required,url"`
}

// ShortenResponse — успешный ответ при создании короткой ссылки.
//
// Возвращается с кодом 201 Created (или 409 Conflict, если ссылка уже существует).
type ShortenResponse struct {
	// Result — полный короткий URL, готовый для использования.
	// Включает базовый адрес из конфигурации (BASE_URL).
	Result string `json:"result" example:"http://localhost:8080/N2tTa3Kr"`
}

// ErrorResponse — стандартный формат ответа при любой ошибке.
//
// Используется во всех эндпоинтах при неуспешном результате (400, 401, 403, 404, 500 и т.д.).
type ErrorResponse struct {
	// Error — текстовое описание ошибки на русском языке.
	Error string `json:"error" example:"Invalid request"`
}

// BatchRequestItem — один элемент в пакетном запросе на сокращение ссылок.
//
// Используется в массиве для эндпоинта POST /api/shorten/batch.
type BatchRequestItem struct {
	// CorrelationID — уникальный идентификатор запроса в рамках батча.
	// Возвращается в ответе для сопоставления оригинального и сокращённого URL.
	CorrelationID string `json:"correlation_id" example:"req-1"`

	// OriginalURL — оригинальный URL, который нужно сократить.
	OriginalURL string `json:"original_url" example:"https://www.example.com/page1" validate:"required,url"`
}

// BatchResponseItem — один элемент в ответе на пакетный запрос.
//
// Соответствует элементу из BatchRequestItem по correlation_id.
type BatchResponseItem struct {
	// CorrelationID — тот же идентификатор, что был передан в запросе.
	CorrelationID string `json:"correlation_id" example:"req-1"`

	// ShortURL — полный короткий URL (с BASE_URL).
	ShortURL string `json:"short_url" example:"http://localhost:8080/abc123"`
}

// StatsResponse — ответ эндпоинта внутренней статистики (GET /api/internal/stats).
//
// Доступен только из доверенной подсети.
type StatsResponse struct {
	// URLs — общее количество сокращённых ссылок в базе.
	URLs int `json:"urls" example:"12345"`

	// Users — количество уникальных пользователей (по user_id).
	Users int `json:"users" example:"987"`
}
