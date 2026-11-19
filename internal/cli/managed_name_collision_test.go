package cli

import (
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/domain"
	aliasrepo "github.com/ra1phdd/zev/internal/entities/alias/repository"
	aliasservice "github.com/ra1phdd/zev/internal/entities/alias/service"
	scriptrepo "github.com/ra1phdd/zev/internal/entities/script/repository"
	scriptservice "github.com/ra1phdd/zev/internal/entities/script/service"
)

const (
	managedCollisionFixture      = "shared"
	managedBuiltinVersionFixture = "version"
)

func TestManagedCreateRejectsAliasScriptAndBuiltinNameCollisions(t *testing.T) {
	for _, tc := range []struct {
		name string
		seed func(*testing.T, string)
		args []string
		want string
	}{
		{
			name: "script conflicts with alias",
			seed: func(t *testing.T, dir string) {
				t.Helper()
				if err := aliasservice.New(aliasrepo.New(dir)).Add(managedCollisionFixture, "echo alias"); err != nil {
					t.Fatal(err)
				}
			},
			args: []string{"script", "create", managedCollisionFixture, "--body-stdin"},
			want: `name "shared" is already used by an alias`,
		},
		{
			name: "alias conflicts with script",
			seed: func(t *testing.T, dir string) {
				t.Helper()
				if err := scriptservice.New(scriptrepo.New(dir)).Create(t.Context(), domain.Script{Name: managedCollisionFixture, ScriptFile: "shared.sh"}, []byte("exit 0\n")); err != nil {
					t.Fatal(err)
				}
			},
			args: []string{"alias", "add", managedCollisionFixture, "--template", managedEchoFixture, managedNoMetadataFlag},
			want: `name "shared" is already used by a script`,
		},
		{
			name: "script conflicts with builtin",
			args: []string{"script", "create", managedBuiltinVersionFixture, "--body-stdin"},
			want: `name "version" conflicts with a built-in command`,
		},
		{
			name: "alias conflicts with builtin",
			args: []string{"alias", "add", "doctor", "--template", managedEchoFixture, managedNoMetadataFlag},
			want: `name "doctor" conflicts with a built-in command`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			writeCLIConfig(t, home, config.DefaultConfig())
			dir, err := config.GetZevDir()
			if err != nil {
				t.Fatal(err)
			}
			if tc.seed != nil {
				tc.seed(t, dir)
			}
			_, err = managedCommand(t, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("create error = %v, want message containing %q", err, tc.want)
			}
		})
	}
}
