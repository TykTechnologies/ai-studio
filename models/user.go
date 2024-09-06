package models

import (
	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	ID       uint   `json:"id" gorm:"primaryKey"`
	Email    string `json:"email"`
	Name     string
	Password string `json:"password"`
}

type Users []User

func NewUser() *User {
	return &User{}
}

func (u *User) Get(db *gorm.DB, id uint) error {
	return db.First(u, id).Error
}

func (u *User) Create(db *gorm.DB) error {
	return db.Create(u).Error
}

func (u *User) Update(db *gorm.DB) error {
	return db.Save(u).Error
}

func (u *User) Delete(db *gorm.DB) error {
	return db.Delete(u).Error
}

func (u *User) GetByEmail(db *gorm.DB, email string) error {
	return db.Where("email = ?", email).First(u).Error
}

func (u *User) DoesPasswordMatch(password string) bool {
	// hash the password using bcrypt and compare it with the hashed password in the database
	return IsPasswordValid(password, u.Password)
}

func (u *User) SetPassword(password string) error {
	// hash the password using bcrypt
	hashed, err := HashPassword(password)
	if err != nil {
		return err
	}

	u.Password = hashed
	return nil
}

func (u *Users) GetAll(db *gorm.DB) error {
	return db.Find(u).Error
}

func (u *Users) GetByGroupID(db *gorm.DB, groupID uint) error {
	return db.Joins("JOIN user_groups ON user_groups.user_id = users.id").Where("user_groups.group_id = ?", groupID).Find(u).Error
}

func (u *Users) SearchByEmailStub(db *gorm.DB, emailStub string) error {
	return db.Where("email LIKE ?", emailStub+"%").Find(u).Error
}

func (u *User) GetAccessibleCatalogues(db *gorm.DB) ([]Catalogue, error) {
	var catalogues []Catalogue
	err := db.Table("catalogues").
		Joins("JOIN group_catalogues ON group_catalogues.catalogue_id = catalogues.id").
		Joins("JOIN user_groups ON user_groups.group_id = group_catalogues.group_id").
		Where("user_groups.user_id = ?", u.ID).
		Distinct().
		Find(&catalogues).Error
	return catalogues, err
}
