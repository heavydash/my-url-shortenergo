package repository

import "errors"

var ErrConflict = errors.New("url already exists")
