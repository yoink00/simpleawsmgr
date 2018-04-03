// +build mage

package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/magefile/mage/mg" // mg contains helpful utility functions, like Deps
	"path/filepath"
)

var Default = Build

func Deps() error {
	deps := []string{
		"github.com/kevinburke/go-bindata",
		"github.com/elazarl/go-bindata-assetfs",
		"github.com/aws/aws-sdk-go/aws/...",
		"github.com/gorilla/websocket",
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
	cmd := exec.Command("go-bindata", "-prefix", "./assets", "-o", "./assets/bindata.go", "-pkg", "assets", "./assets")
	return cmd.Run()
}

func Test() error {
	mg.Deps(Assets)
	fmt.Println("Testing...")
	cmd := exec.Command("go", "test", "./...")
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
