// Command seedsuperadmin promotes an existing user to the super_admin
// platform role. There is no signup-flow path to this role on purpose —
// run this once per operator you want to grant platform-wide access to.
//
// Usage (from the server/ directory):
//
//	go run ./cmd/seedsuperadmin someone@email.com
package main

import (
	"fmt"
	"os"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
)

func init() {
	initializers.LoadEnvVars()
	initializers.ConnectToDB()
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: go run ./cmd/seedsuperadmin <email>")
		os.Exit(1)
	}
	email := os.Args[1]

	var user models.User
	if err := initializers.DB.Where("email = ?", email).First(&user).Error; err != nil {
		fmt.Printf("no user found with email %q: %v\n", email, err)
		os.Exit(1)
	}

	if err := initializers.DB.Model(&user).Update("platform_role", "super_admin").Error; err != nil {
		fmt.Printf("failed to promote user: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%s (%s) is now a super_admin\n", email, user.ID)
}
