package cli

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/filestore"
)

const (
	runSessionIDLength  = 10
	runSessionAlphabet  = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	runSessionFileName  = "run-sessions.json"
	runSessionFileLimit = 1 << 20
)

var errRunSessionNotFound = errors.New("run session not found")

//nolint:tagliatelle // These snake_case names preserve the persisted registry JSON contract.
type runSessionRecord struct {
	ConversationID string    `json:"conversation_id"`
	CommandID      string    `json:"command_id"`
	CreatedAt      time.Time `json:"created_at"`
}

type runSessionRegistry struct {
	Sessions map[string]runSessionRecord `json:"sessions"`
}

func registerRunSession(ctx context.Context, conversationID, commandID string) (sessionID string, err error) {
	if err := validateRunSessionReference(conversationID, commandID); err != nil {
		return "", err
	}
	path, err := runSessionPath()
	if err != nil {
		return "", err
	}
	err = filestore.WithLockContext(ctx, path, func() error {
		registry, err := readRunSessionRegistry(path)
		if err != nil {
			return err
		}
		for id, record := range registry.Sessions {
			if record.ConversationID == conversationID && record.CommandID == commandID {
				sessionID = id
				return nil
			}
		}
		for {
			sessionID, err = newRunSessionID()
			if err != nil {
				return err
			}
			if _, exists := registry.Sessions[sessionID]; !exists {
				break
			}
		}
		registry.Sessions[sessionID] = runSessionRecord{ConversationID: conversationID, CommandID: commandID, CreatedAt: time.Now().UTC()}
		return writeRunSessionRegistry(path, registry)
	})
	if err != nil {
		return "", fmt.Errorf("register run session: %w", err)
	}
	return sessionID, nil
}

func resolveRunSession(ctx context.Context, sessionID string) (conversationID, commandID string, err error) {
	if err := validateRunSessionID(sessionID); err != nil {
		return "", "", err
	}
	path, err := runSessionPath()
	if err != nil {
		return "", "", err
	}
	err = filestore.WithLockContext(ctx, path, func() error {
		registry, err := readRunSessionRegistry(path)
		if err != nil {
			return err
		}
		record, ok := registry.Sessions[sessionID]
		if !ok {
			return errRunSessionNotFound
		}
		conversationID, commandID = record.ConversationID, record.CommandID
		return nil
	})
	if err != nil {
		return "", "", fmt.Errorf("resolve run session: %w", err)
	}
	return conversationID, commandID, nil
}

func validateRunSessionReference(conversationID, commandID string) error {
	for name, value := range map[string]string{"conversation": conversationID, "command": commandID} {
		parsed, err := uuid.Parse(value)
		if err != nil || parsed.String() != value {
			return fmt.Errorf("run wrapper session requires a canonical %s UUID", name)
		}
	}
	return nil
}

func validateRunSessionID(value string) error {
	if len(value) != runSessionIDLength {
		return fmt.Errorf("run session must be exactly %d characters", runSessionIDLength)
	}
	for _, char := range value {
		if !containsRunSessionChar(char) {
			return errors.New("run session contains an invalid character")
		}
	}
	return nil
}

func containsRunSessionChar(char rune) bool {
	for _, allowed := range runSessionAlphabet {
		if allowed == char {
			return true
		}
	}
	return false
}

func newRunSessionID() (string, error) {
	result := make([]byte, runSessionIDLength)
	buffer := make([]byte, runSessionIDLength)
	for filled := 0; filled < len(result); {
		if _, err := rand.Read(buffer); err != nil {
			return "", fmt.Errorf("generate run session ID: %w", err)
		}
		for _, value := range buffer {
			if value >= 248 {
				continue
			}
			result[filled] = runSessionAlphabet[int(value)%len(runSessionAlphabet)]
			filled++
			if filled == len(result) {
				break
			}
		}
	}
	return string(result), nil
}

func runSessionPath() (string, error) {
	dir, err := config.GetZevDir()
	if err != nil {
		return "", fmt.Errorf("resolve Zev directory for run session: %w", err)
	}
	return filepath.Join(dir, runSessionFileName), nil
}

func readRunSessionRegistry(path string) (runSessionRegistry, error) {
	data, err := filestore.ReadFileLimit(path, runSessionFileLimit)
	if errors.Is(err, os.ErrNotExist) {
		return runSessionRegistry{Sessions: make(map[string]runSessionRecord)}, nil
	}
	if err != nil {
		return runSessionRegistry{}, fmt.Errorf("read run session registry: %w", err)
	}
	var registry runSessionRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return runSessionRegistry{}, fmt.Errorf("decode run session registry: %w", err)
	}
	if registry.Sessions == nil {
		registry.Sessions = make(map[string]runSessionRecord)
	}
	for id, record := range registry.Sessions {
		if err := validateRunSessionID(id); err != nil {
			return runSessionRegistry{}, fmt.Errorf("validate run session registry ID: %w", err)
		}
		if err := validateRunSessionReference(record.ConversationID, record.CommandID); err != nil {
			return runSessionRegistry{}, fmt.Errorf("validate run session registry reference: %w", err)
		}
	}
	return registry, nil
}

func writeRunSessionRegistry(path string, registry runSessionRegistry) error {
	data, err := json.Marshal(registry)
	if err != nil {
		return fmt.Errorf("encode run session registry: %w", err)
	}
	if len(data) > runSessionFileLimit {
		return errors.New("run session registry exceeds its size limit")
	}
	if err := filestore.AtomicWrite(path, data, 0o600); err != nil {
		return fmt.Errorf("write run session registry: %w", err)
	}
	return nil
}
