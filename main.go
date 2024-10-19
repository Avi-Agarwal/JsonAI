package main

import (
	"JsonAI/server"
)

func main() {
	jaiServer := server.NewServer()
	err := jaiServer.Serve()
	if err != nil {
		return
	}
}
