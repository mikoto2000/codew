package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"ollama-codex-cli/internal/ollama"
	"ollama-codex-cli/internal/session"
)

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start an interactive chat session",
	RunE:  runChat,
}

func runChat(cmd *cobra.Command, _ []string) error {
	client := ollama.NewClient(chatHost, timeout)
	s := session.New(chatModel, systemText)

	fmt.Printf("Connected target: %s\n", chatHost)
	fmt.Printf("Model: %s\n", s.Model)
	fmt.Println("Commands: /exit, /model <name>, /system <text>, /reset, /help")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("you> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			return nil
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "/") {
			done, err := runCommand(line, s)
			if err != nil {
				fmt.Fprintf(os.Stderr, "command error: %v\n", err)
			}
			if done {
				return nil
			}
			continue
		}

		s.AddUser(line)
		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		fmt.Print("assistant> ")
		answer, err := client.ChatStream(ctx, s.Model, s.Messages(), func(chunk string) {
			fmt.Print(chunk)
		})
		cancel()

		if err != nil {
			fmt.Fprintf(os.Stderr, "\nrequest failed: %v\n", err)
			s.RollbackLastUser()
			continue
		}

		s.AddAssistant(answer)
		fmt.Println()
	}
}

func runCommand(line string, s *session.Session) (bool, error) {
	parts := strings.SplitN(line, " ", 2)
	name := parts[0]
	arg := ""
	if len(parts) == 2 {
		arg = strings.TrimSpace(parts[1])
	}

	switch name {
	case "/exit", "/quit":
		return true, nil
	case "/reset":
		s.Reset()
		fmt.Println("session reset")
		return false, nil
	case "/model":
		if arg == "" {
			return false, errors.New("usage: /model <name>")
		}
		s.Model = arg
		fmt.Printf("model changed to: %s\n", s.Model)
		return false, nil
	case "/system":
		if arg == "" {
			return false, errors.New("usage: /system <text>")
		}
		s.System = arg
		s.Reset()
		fmt.Println("system prompt updated and history reset")
		return false, nil
	case "/help":
		fmt.Println("/exit | /quit  : end session")
		fmt.Println("/model <name>  : switch model")
		fmt.Println("/system <text> : change system prompt and reset history")
		fmt.Println("/reset         : clear chat history")
		return false, nil
	default:
		return false, fmt.Errorf("unknown command: %s", name)
	}
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
