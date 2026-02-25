package repository

import "errors"

var ErrConflict = errors.New("url already exists")
var ErrNotFound = errors.New("url not found")
