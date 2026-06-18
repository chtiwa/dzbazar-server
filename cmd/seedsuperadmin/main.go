// Command seedsuperadmin creates or updates the platform's single super
// admin account. Only one super_admin may exist at a time — enforced both
// here (any other holder is demoted in the same transaction) and at the DB
// level via a partial unique index on users.platform_role.
//
// Usage (from the server/ directory):
//
//	go run ./cmd/seedsuperadmin <email> <password>
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/migrate"
	"github.com/chtiwa/dzbazar-server/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func init() {
	initializers.LoadEnvVars()
	initializers.ConnectToDB()
	migrate.Migrate()
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: go run ./cmd/seedsuperadmin <email> <password>")
		os.Exit(1)
	}
	email := strings.TrimSpace(os.Args[1])
	password := os.Args[2]

	hash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		fmt.Printf("failed to hash password: %v\n", err)
		os.Exit(1)
	}

	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		var user models.User
		err := tx.Where("email = ?", email).First(&user).Error

		switch {
		case err == nil:
			// Existing user — set the password and ensure they can log in.
			user.Password = string(hash)
			user.IsVerified = true
			user.IsSuspended = false
			if err := tx.Save(&user).Error; err != nil {
				return err
			}
		case gorm.ErrRecordNotFound == err:
			user = models.User{
				FirstName:  "Super",
				LastName:   "Admin",
				Email:      email,
				Password:   string(hash),
				IsVerified: true,
			}
			if err := tx.Create(&user).Error; err != nil {
				return err
			}
		default:
			return err
		}

		// Only one super_admin allowed — demote anyone else currently holding it.
		if err := tx.Model(&models.User{}).
			Where("platform_role = ? AND id != ?", "super_admin", user.ID).
			Update("platform_role", "").Error; err != nil {
			return err
		}

		return tx.Model(&user).Update("platform_role", "super_admin").Error
	})

	if err != nil {
		fmt.Printf("failed to set up super admin: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%s is now the platform's super_admin\n", email)
}
