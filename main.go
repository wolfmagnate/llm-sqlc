package main

import (
	"fmt"
	"log"
	"os"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: go run main.go <command> <path-to-infra-go-file>")
		os.Exit(1)
	}

	command := os.Args[1]
	infraFile := os.Args[2]

	if command == "sql" {
		if err := GenerateSQL(infraFile); err != nil {
			log.Fatalf("failed to generate SQL: %v", err)

		}
	} else if command == "program" {
		if err := GenerateProgram(infraFile); err != nil {
			log.Fatalf("failed to generate program: %v", err)
		}
	} else {
		fmt.Printf("Unknown command: %s\n", command)
		fmt.Println("Available commands: sql, program, infra")
		os.Exit(1)
	}
}
