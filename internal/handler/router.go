package handler

import (
	"github.com/go-chi/chi/v5"
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"net/http"
)

// SetupRouter настраивает все маршруты приложения и возвращает готовый chi.Mux.
// Выносится из main.go для уменьшения сложности главной функции.
//
// Маршруты:
//   - GET /{id} — редирект по короткой ссылке
//   - GET /ping — health check
//   - GET / — домашняя страница
//   - POST / — создание короткой ссылки (plain text)
//   - POST /api/shorten — создание короткой ссылки (JSON)
//   - POST /api/shorten/batch — пакетное создание коротких ссылок
//   - GET /api/user/urls — получение всех ссылок пользователя
//   - DELETE /api/user/urls — удаление ссылок пользователя
//   - GET /api/internal/stats — внутренняя статистика (доступ по trusted_subnet)
//
// Все маршруты, кроме /{id} и /ping, требуют авторизации через cookie.
// Для сжатия ответов используется GzipMiddleware.
// Для логирования — LoggingMiddleware.
func SetupRouter(h *Handler) *chi.Mux {
	router := chi.NewRouter()

	// Глобальные middleware
	router.Use(middleware.Logging(h.logger))
	router.Use(middleware.GzipMiddleware(h.logger))

	// Публичный маршрут для редиректа
	router.Get("/{id}", h.RedirectURL)

	// Кастомная обработка 404
	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	// Группа с обязательной авторизацией (cookie)
	router.Group(func(r chi.Router) {
		r.Use(middleware.Auth(h.logger))
		r.Get("/api/user/urls", h.GetUserURLs)
		r.Delete("/api/user/urls", h.DeleteUrls)
	})

	// Публичные маршруты
	router.Get("/ping", h.PingHandler)
	router.Get("/", h.HomeHandler)

	// Маршруты с опциональной авторизацией
	router.Post("/", middleware.Auth(h.logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ShortenHandler(w, r, false)
	})).ServeHTTP)

	router.Post("/api/shorten", middleware.Auth(h.logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ShortenHandler(w, r, true)
	})).ServeHTTP)

	router.Post("/api/shorten/batch", middleware.Auth(h.logger)(http.HandlerFunc(h.BatchShortenHandler)).ServeHTTP)

	// Внутренний защищённый маршрут trusted_subnet
	router.Group(func(r chi.Router) {
		r.Get("/api/internal/stats", h.GetInternalStats)
	})

	return router
}
