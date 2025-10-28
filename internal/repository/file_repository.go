package repository

import (
	"encoding/json"
	"fmt"
	"github.com/heavydash/my-url-shortenergo/internal/idgen"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"io"
	"log"
	"os"
)

type FileRepository struct {
	file    *os.File
	encoder *json.Encoder
	urls    map[string]model.URLModel
}

func NewFileRepository(filename string) (*FileRepository, error) {
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	log.Printf("FILE_REPO: File opened: %s", filename)

	encoder := json.NewEncoder(file)

	log.Printf("FILE_REPO: Encoder created")

	urls := make(map[string]model.URLModel)
	repo := &FileRepository{
		file:    file,
		encoder: encoder,
		urls:    urls,
	}
	_, err = repo.LoadURLs()
	if err != nil {
		file.Close()
		return nil, err
	}
	return repo, nil

}

func (r *FileRepository) NewFileRepository(filename string) (*FileRepository, error) {
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(file)
	var urls []model.URLModel
	err = decoder.Decode(&urls)
	if err != nil {
		return nil, err
	}
	for _, url := range urls {
		r.urls[url.UUID] = url
	}
	return nil, err
}

func (r *FileRepository) SaveURL(url model.URLModel) (model.URLModel, error) {
	currentmodel := url

	if url.UUID != "" {
		if _, ok := r.urls[url.UUID]; ok {
			return url, fmt.Errorf("url with id %s already exists", url.UUID)
		}
		currentmodel.UUID = url.UUID
		currentmodel.ShortURL = url.UUID
	} else {

		maxAttempts := 5
		for attempt := 0; attempt < maxAttempts; attempt++ {
			newID, err := idgen.IDGen()
			if err != nil {
				return currentmodel, err
			}
			if _, ok := r.urls[newID]; !ok {
				currentmodel.UUID = newID
				currentmodel.ShortURL = newID
				break
			}
		}
		if currentmodel.UUID == "" {
			return currentmodel, fmt.Errorf("failed to generate unique ID after %d attempts", maxAttempts)
		}
	}

	if err := r.encoder.Encode(currentmodel); err != nil {
		return currentmodel, err
	}

	if _, err := r.file.Write([]byte("\n")); err != nil {
		return currentmodel, err
	}
	r.urls[currentmodel.UUID] = currentmodel
	return currentmodel, nil
}

func (r *FileRepository) GetURL(uuid string) (model.URLModel, error) {
	if url, ok := r.urls[uuid]; ok {
		return url, nil
	}
	return model.URLModel{}, fmt.Errorf("not found")
}

func (r *FileRepository) Clear() {
	r.file.Close()
	os.Truncate(r.file.Name(), 0)
	file, err := os.OpenFile(r.file.Name(), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal(err)
	}
	r.file = file
	r.encoder = json.NewEncoder(r.file)
	r.urls = make(map[string]model.URLModel)
}

func (r *FileRepository) LoadURLs() ([]model.URLModel, error) {
	file, err := os.Open(r.file.Name())
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var urls []model.URLModel
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&urls)
	for err != nil && err != io.EOF {
		return nil, err
	}
	for _, url := range urls {
		r.urls[url.UUID] = url
	}
	return urls, nil
}
