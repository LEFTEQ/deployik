package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/LEFTEQ/lovinka-deployik/internal/backup"
)

func main() {
	log.SetFlags(0)

	command := "create"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	switch command {
	case "create":
		runCreate(os.Args[2:])
	case "verify":
		runVerify(os.Args[2:])
	case "-h", "--help", "help":
		printUsage()
	default:
		log.Fatalf("unknown command %q\n\n%s", command, usageText)
	}
}

func runCreate(args []string) {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	databasePath := fs.String("database", envOrDefault("DATABASE_PATH", "data/deployik.db"), "Path to the live SQLite database")
	outputPath := fs.String("output", "", "Where to write the backup snapshot")
	fs.Parse(args)

	if *outputPath == "" {
		log.Fatal("missing required --output flag")
	}

	if err := backup.CreateSQLiteSnapshot(*databasePath, *outputPath); err != nil {
		log.Fatalf("create backup: %v", err)
	}

	fmt.Printf("backup created at %s\n", *outputPath)
}

func runVerify(args []string) {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	filePath := fs.String("file", "", "SQLite file to verify")
	fs.Parse(args)

	if *filePath == "" {
		log.Fatal("missing required --file flag")
	}

	if err := backup.VerifySQLiteDatabase(*filePath); err != nil {
		log.Fatalf("verify backup: %v", err)
	}

	fmt.Printf("backup verified: %s\n", *filePath)
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func printUsage() {
	fmt.Print(usageText)
}

const usageText = `Deployik backup utility

Usage:
  deployik-backup create --output <path> [--database <path>]
  deployik-backup verify --file <path>
`
