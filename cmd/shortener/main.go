package main

import (
	"fmt"
	"github.com/heavydash/my-url-shortenergo/internal/handler"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
	"net/http"
)

func main() {
	repo := repository.NewMemoryRepository()
	h := handler.NewHandler(repo)
	mux := http.NewServeMux()
	mux.HandleFunc("/", h.ServeHTTP)
	err := http.ListenAndServe(":8080", mux)
	fmt.Println("Server stopped:", err)
	if err != nil {
		panic(err)
	}
}
