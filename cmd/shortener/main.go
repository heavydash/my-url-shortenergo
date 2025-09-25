package main

import (
	"github.com/go-chi/chi"
	"log"
	"net/http"

	"github.com/heavydash/my-url-shortenergo/internal/handler"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
)

func main() {
	repo := repository.NewMemoryRepository()
	h := handler.NewHandler(repo)
	r := chi.NewRouter()
	h.SetupRoutes(r)
	log.Fatal(http.ListenAndServe(":8080", r))
}
