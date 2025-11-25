package repository

import "errors"

var ErrAlreadyExists = errors.New("url already exists")

type ErrURLAlreadyExists struct {
	ShortURL string
}

func (e *ErrURLAlreadyExists) Error() string {
	return ErrAlreadyExists.Error()
}

// Для errors.IS()
func (e *ErrURLAlreadyExists) Is(target error) bool {
	return target == ErrAlreadyExists
}
