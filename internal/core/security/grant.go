package security

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
)

// Grant is a single-use authorization for one exact invocation snapshot.
// Its state is private so callers cannot manufacture an allowed grant by
// setting a public flag.
type Grant struct {
	actionDigest      string
	policyDigest      string
	environmentDigest string
	inputDigest       string
	consumed          atomic.Bool
}

// NewGrant records the action, policy, environment, working-directory identity,
// and reviewed input identity that were present at authorization time.
func NewGrant(action Action, policy Policy, environment []string, input []byte) (*Grant, error) {
	state, err := snapshotDigests(action, policy, environment, input)
	if err != nil {
		return nil, err
	}
	return &Grant{
		actionDigest:      state.actionDigest,
		policyDigest:      state.policyDigest,
		environmentDigest: state.environmentDigest,
		inputDigest:       state.inputDigest,
	}, nil
}

// Consume validates the exact invocation snapshot and atomically consumes the
// grant. A changed action, policy, environment, working directory, or reviewed
// input is rejected, and a grant cannot authorize a second dispatch attempt.
func (g *Grant) Consume(action Action, policy Policy, environment []string, input []byte) error {
	if g == nil || g.actionDigest == "" {
		return errors.New("security grant is invalid")
	}
	state, err := snapshotDigests(action, policy, environment, input)
	if err != nil {
		return fmt.Errorf("validate security grant: %w", err)
	}
	if state.actionDigest != g.actionDigest || state.policyDigest != g.policyDigest || state.environmentDigest != g.environmentDigest || state.inputDigest != g.inputDigest {
		return errors.New("security grant does not match the invocation snapshot")
	}
	if !g.consumed.CompareAndSwap(false, true) {
		return errors.New("security grant was already consumed")
	}
	return nil
}

type grantDigests struct {
	actionDigest      string
	policyDigest      string
	environmentDigest string
	inputDigest       string
}

type actionSnapshot struct {
	Action         Action
	WorkingDirInfo string
}

type policySnapshot struct {
	Mode            Mode
	DefaultDecision Decision
	Rules           []Rule
	DeniedCommands  []string
	Workspace       string
	Home            string
}

func snapshotDigests(action Action, policy Policy, environment []string, input []byte) (grantDigests, error) {
	workingDirInfo, err := directoryIdentity(action.WorkingDir)
	if err != nil {
		return grantDigests{}, err
	}
	action.Argv = append([]string(nil), action.Argv...)
	action.Effects = append([]Effect(nil), action.Effects...)
	action.Paths = append([]string(nil), action.Paths...)
	actionSnapshotBytes, err := json.Marshal(actionSnapshot{Action: action, WorkingDirInfo: workingDirInfo})
	if err != nil {
		return grantDigests{}, fmt.Errorf("encode action snapshot: %w", err)
	}
	policyBytes, err := json.Marshal(policySnapshot(policy))
	if err != nil {
		return grantDigests{}, fmt.Errorf("encode policy snapshot: %w", err)
	}
	return grantDigests{
		actionDigest:      digest(actionSnapshotBytes),
		policyDigest:      digest(policyBytes),
		environmentDigest: digestBytes(environment),
		inputDigest:       digest(input),
	}, nil
}

func directoryIdentity(path string) (string, error) {
	if path == "" {
		return "", errors.New("working directory is required for a security grant")
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Resource adapters still reject a missing working directory. Keep a
			// distinct snapshot so a directory appearing between authorization and
			// dispatch invalidates the grant instead of silently changing identity.
			return "missing:" + path, nil
		}
		return "", fmt.Errorf("stat working directory: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("working directory is not a directory")
	}
	value, err := json.Marshal(struct {
		Path    string
		Mode    string
		Size    int64
		ModTime int64
		System  string
	}{Path: path, Mode: info.Mode().String(), Size: info.Size(), ModTime: info.ModTime().UnixNano(), System: fmt.Sprintf("%#v", info.Sys())})
	if err != nil {
		return "", fmt.Errorf("encode working directory identity: %w", err)
	}
	return string(value), nil
}

func digestBytes(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return digest([]byte(fmt.Sprintf("%T", value)))
	}
	return digest(encoded)
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
