package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

var urls = make(map[string]string)
var counter = 0

func handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if r.Method != http.MethodPost { // Логика для POST (ошибка в условии — исправь ниже)
			http.Error(w, "Method not allowed", 405)
			return
		}
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Bad request", 400)
			return
		}
		urlStr := string(body)
		if len(urlStr) == 0 || !strings.HasPrefix(urlStr, "http") {
			http.Error(w, "Invalid URL", 400)
			return
		}
		id := fmt.Sprintf("%08d", counter)
		counter++
		_, exists := urls[id]
		if exists {
			counter++
			id = fmt.Sprintf("%08d", counter)
		}
		urls[id] = urlStr
		w.Header().Set("content-type", "text/plain")
		w.WriteHeader(201)
		_, err = w.Write([]byte("http://localhost:8080/" + id))
		if err != nil {
			http.Error(w, "Write error", 500)
			return
		}
	} else if r.Method == http.MethodGet {
		id := strings.TrimPrefix(r.URL.Path, "/")
		if len(id) == 0 {
			http.Error(w, "Invalid ID", 400)
			return
		}
		original, ok := urls[id]
		if !ok {
			http.Error(w, "Invalid ID", 400)
			return
		}
		http.Redirect(w, r, original, http.StatusTemporaryRedirect)
	} else {
		http.Error(w, "Method not allowed", 405)
		return
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handler) // Один хендлер для всех методов
	err := http.ListenAndServe(":8080", mux)
	fmt.Println("Server stopped:", err)
	if err != nil {
		panic(err)
	}
}
