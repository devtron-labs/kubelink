package main

import (
	_ "github.com/devtron-labs/common-lib/utils/k8s"
	"log"
)

func main() {
	app, err := InitializeApp()
	if err != nil {
		log.Panic(err)
	}
	app.Start()
}
