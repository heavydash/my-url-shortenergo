package generator

import (
	"crypto/rand"
	"log"
)

// ShortURLGen генерирует короткий идентификатор для URL (длина 8 символов).
//
// Рекомендуется использовать именно эту функцию для генерации поля ShortURL в URLModel.
// В будущем здесь можно изменить длину, алфавит или алгоритм генерации, не затрагивая другие ID.
func ShortURLGen() (string, error) {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	id := string(b)
	log.Printf("Generated ShortURL: %s", id)
	return id, nil
}
