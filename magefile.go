// +build mage

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	"path/filepath"

	"github.com/magefile/mage/mg" // mg contains helpful utility functions, like Deps
)

var Default = Build
var isTravis = false

func init() {
	travisStr := os.Getenv("TRAVIS")
	if travisStr == "true" {
		log.Println("Running build in Travis")
		isTravis = true
	}
}

func Deps() error {
	// All code dependencies are managed by Manul and are vendored.
	// These dependencies are build dependencies.
	deps := []string{
		"github.com/kovetskiy/manul",
		"github.com/kevinburke/go-bindata/...",
	}

	if isTravis {
		log.Println("Running in Travis. Installiing goveralls")
		deps = append(deps, "github.com/mattn/goveralls")
	}

	for _, dep := range deps {
		fmt.Println("Installing", dep)
		cmd := exec.Command("go", "get", dep)
		err := cmd.Run()
		if err != nil {
			return err
		}
	}

	return nil
}

func Assets() error {
	mg.Deps(Deps)
	fmt.Println("Building assets...")
	cmd := exec.Command(filepath.Join(os.Getenv("GOPATH"), "bin", "go-bindata"), "-prefix", "./assets", "-o", "./assets/bindata.go", "-pkg", "assets", "./assets")
	return cmd.Run()
}

func Test() error {
	mg.Deps(Assets)
	log.Println("Testing...")
	var cmd *exec.Cmd
	if isTravis {
		log.Println("Running tests in Travis. Using Goveralls.")
		cmd = exec.Command(filepath.Join(os.Getenv("GOPATH"), "bin", "goveralls"), "-service=travis-ci")
	} else {
		log.Println("Running tests in outside Travis. Using go test.")
		cmd = exec.Command("go", "test", "./...")
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func Build() error {
	mg.Deps(Test)
	fmt.Println("Installing...")
	cmd := exec.Command("go", "install", "./...")
	return cmd.Run()
}

func Clean() {
	fmt.Println("Cleaning...")
	os.Remove("./assets/bindata.go")
	os.Remove(filepath.Join(os.Getenv("GOPATH"), "bin", "simpleawsmgr"))
}
