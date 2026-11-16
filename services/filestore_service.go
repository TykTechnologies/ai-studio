package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// CreateFileStore creates a new filestore entry with validity checks
// workspaceID, when given, is the Workspace the entry lands in (0 or
// omitted: the Default).
func (s *Service) CreateFileStore(fileName, description, content string, length int, workspaceID ...uint) (*models.FileStore, error) {
	fileStore := &models.FileStore{
		FileName:    fileName,
		Description: description,
		Content:     content,
		Length:      length,
	}
	if len(workspaceID) > 0 {
		fileStore.WorkspaceID = workspaceID[0]
	}

	if err := fileStore.Create(s.DB); err != nil {
		return nil, err
	}

	return fileStore, nil
}

// UpdateFileStore updates an existing filestore entry with validity checks
func (s *Service) UpdateFileStore(id uint, fileName, description, content string, length int) (*models.FileStore, error) {
	fileStore, err := s.GetFileStoreByID(id)
	if err != nil {
		return nil, err
	}

	fileStore.FileName = fileName
	fileStore.Description = description
	fileStore.Content = content
	fileStore.Length = length

	if err := fileStore.Update(s.DB); err != nil {
		return nil, err
	}

	return fileStore, nil
}

// GetFileStoreByID retrieves a filestore entry by its ID
func (s *Service) GetFileStoreByID(id uint) (*models.FileStore, error) {
	fileStore := models.NewFileStore()
	if err := fileStore.Get(s.DB, id); err != nil {
		return nil, err
	}
	return fileStore, nil
}

// DeleteFileStore deletes a filestore entry
func (s *Service) DeleteFileStore(id uint) error {
	fileStore, err := s.GetFileStoreByID(id)
	if err != nil {
		return err
	}

	return fileStore.Delete(s.DB)
}

// GetFileStoreByFileName retrieves a filestore entry by its filename
func (s *Service) GetFileStoreByFileName(fileName string) (*models.FileStore, error) {
	fileStore := models.NewFileStore()
	if err := fileStore.GetByFileName(s.DB, fileName); err != nil {
		return nil, err
	}
	return fileStore, nil
}

// GetAllFileStores retrieves all filestore entries with pagination
// scopes narrow the list (the API's Workspace filter).
func (s *Service) GetAllFileStores(pageSize int, pageNumber int, all bool, scopes ...func(*gorm.DB) *gorm.DB) ([]models.FileStore, int64, int, error) {
	var fileStores models.FileStores
	totalCount, totalPages, err := fileStores.GetAll(s.DB.Scopes(scopes...), pageSize, pageNumber, all)
	if err != nil {
		return nil, 0, 0, err
	}
	return fileStores, totalCount, totalPages, nil
}

// SearchFileStores searches for filestore entries matching the given query in filename or description
func (s *Service) SearchFileStores(query string, scopes ...func(*gorm.DB) *gorm.DB) ([]models.FileStore, error) {
	var fileStores models.FileStores
	if err := fileStores.Search(s.DB.Scopes(scopes...), query); err != nil {
		return nil, err
	}
	return fileStores, nil
}
