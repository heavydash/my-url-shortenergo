package repository

func NewRepository(filePath string) URLRepository {
	if filePath == "" {
		return NewFileRepository(filePath)
	}
	return NewMemoryRepository()
}
