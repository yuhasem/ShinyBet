// Package cli implements a basic command line interface for modifying the
// program during runtime.  This is intended for basic interactions while it's
// still being deployed non a not-headless server.
package cli

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

var LogLevel = new(slog.LevelVar)

func Loop() {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("> ")
		// TODO: breaking because it needs a double new line.  Can I find a way
		// to use Scan instead?  Are there other examples of a Go cli loop I can
		// steal?
		command, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("error: %v\n", err)
			continue
		}
		// The combination of these 2 should work on Linux/Windows environments.
		command = strings.TrimSuffix(command, "\n")
		command = strings.TrimSuffix(command, "\r")
		tokens := strings.Split(command, " ")
		if len(tokens) == 0 {
			fmt.Println("no command given")
			continue
		}
		switch tokens[0] {
		case "debug":
			handleDebug(tokens[1:]...)
		default:
			fmt.Printf("not a command: %s\n", tokens[0])
		}
	}
}

func handleDebug(args ...string) {
	if args[0] == "on" {
		LogLevel.Set(slog.LevelDebug)
		fmt.Println("logging now set to debug level")
	} else if args[0] == "off" {
		LogLevel.Set(slog.LevelInfo)
		fmt.Println("logging now set to info level")
	} else {
		fmt.Println("unknown arg " + args[0])
	}
}
