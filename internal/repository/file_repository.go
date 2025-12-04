package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/heavydash/my-url-shortenergo/internal/idgen"
	"github.com/heavydash/my-url-shortenergo/internal/model"
)

type FileRepository struct {
	file    *os.File
	encoder *json.Encoder
	urls    map[string]model.URLModel
}

func NewFileRepository(path string) *FileRepository {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		panic(err)
	}
	repo := &FileRepository{
		file:    file,
		encoder: json.NewEncoder(file),
		urls:    make(map[string]model.URLModel),
	}
	repo.loadFromFile()
	return repo
}

func (r *FileRepository) loadFromFile() {
	r.file.Seek(0, 0)
	dec := json.NewDecoder(r.file)
	for {
		var u model.URLModel
		if err := dec.Decode(&u); err != nil {
			if err == io.EOF {
				break
			}
			continue
		}
		if u.UUID != "" {
			r.urls[u.UUID] = u
		}
	}
}

func (r *FileRepository) SaveURL(url model.URLModel) (model.URLModel, error) {
	if url.UUID == "" {
		id, _ := idgen.IDGen()
		url.UUID = id
		url.ShortURL = id
	}
	if _, ok := r.urls[url.UUID]; ok {
		return url, fmt.Errorf("id already exists")
	}
	if err := r.encoder.Encode(url); err != nil {
		return url, err
	}
	r.file.Write([]byte("\n"))
	r.urls[url.UUID] = url
	return url, nil
}

func (r *FileRepository) GetURL(id string) (model.URLModel, error) {
	if url, ok := r.urls[id]; ok {
		return url, nil
	}
	return model.URLModel{}, fmt.Errorf("not found")
}

func (r *FileRepository) Clear() {
	r.file.Truncate(0)
	r.file.Seek(0, 0)
	clear(r.urls)
}
func (r *FileRepository) Ping(ctx context.Context) error {
	return nil
}
