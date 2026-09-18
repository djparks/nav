package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// env turns a map into a Getenv function, so a test can describe the
// environment it wants without touching the real one.
func env(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

// writeConfig puts a configuration file in a temporary directory and returns
// Options that point Load at it, along with the file's path.
func writeConfig(t *testing.T, contents string, vars map[string]string) (Options, string) {
	t.Helper()

	home := t.TempDir()
	file := filepath.Join(home, "config")
	if err := os.WriteFile(file, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	if vars == nil {
		vars = map[string]string{}
	}
	vars[EnvConfig] = file

	return Options{
		Getenv:  env(vars),
		Home:    home,
		WorkDir: t.TempDir(), // no ./cheats in it, so the default is predictable
	}, file
}

func TestLoadWithNothingSetUsesTheDefault(t *testing.T) {
	home := t.TempDir()
	opts := Options{Getenv: env(nil), Home: home, WorkDir: t.TempDir()}

	cfg, err := Load(opts)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if want := filepath.Join(home, ".local", "share", AppDir, CheatDirName); cfg.Path != want {
		t.Errorf("Path = %q, want %q", cfg.Path, want)
	}
	if !strings.Contains(cfg.PathSource, "default") {
		t.Errorf("PathSource = %q, want it to mention the default", cfg.PathSource)
	}
	if cfg.File != "" {
		t.Errorf("File = %q, want it empty when there is no config file", cfg.File)
	}
}

func TestLoadReadsThePathFromTheConfigFile(t *testing.T) {
	opts, file := writeConfig(t, "# my cheats\npath = /srv/cheats\n", nil)

	cfg, err := Load(opts)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Path != "/srv/cheats" {
		t.Errorf("Path = %q, want /srv/cheats", cfg.Path)
	}
	if want := file + ":2"; cfg.PathSource != want {
		t.Errorf("PathSource = %q, want %q", cfg.PathSource, want)
	}
	if cfg.File != file {
		t.Errorf("File = %q, want %q", cfg.File, file)
	}
}

func TestEnvironmentBeatsTheConfigFile(t *testing.T) {
	opts, _ := writeConfig(t, "path = /from/file\n", map[string]string{EnvPath: "/from/env"})

	cfg, err := Load(opts)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Path != "/from/env" {
		t.Errorf("Path = %q, want /from/env", cfg.Path)
	}
	if want := "$" + EnvPath; cfg.PathSource != want {
		t.Errorf("PathSource = %q, want %q", cfg.PathSource, want)
	}
}

func TestLoadExpandsALeadingTilde(t *testing.T) {
	opts, _ := writeConfig(t, "path = ~/cheats\n", nil)
	home := opts.Home

	cfg, err := Load(opts)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if want := filepath.Join(home, "cheats"); cfg.Path != want {
		t.Errorf("Path = %q, want %q", cfg.Path, want)
	}
}

func TestLoadExpandsATildeFromTheEnvironmentToo(t *testing.T) {
	home := t.TempDir()
	opts := Options{Getenv: env(map[string]string{EnvPath: "~"}), Home: home, WorkDir: t.TempDir()}

	cfg, err := Load(opts)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Path != home {
		t.Errorf("Path = %q, want %q", cfg.Path, home)
	}
}

func TestLoadIgnoresAMissingConfigFile(t *testing.T) {
	dir := t.TempDir()
	opts := Options{
		Getenv:  env(map[string]string{EnvConfig: filepath.Join(dir, "nope")}),
		Home:    dir,
		WorkDir: t.TempDir(),
	}

	if _, err := Load(opts); err != nil {
		t.Errorf("Load() error = %v, want nil: a missing config file is normal", err)
	}
}

func TestLoadRejectsAnUnreadableConfigFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read anything")
	}

	opts, file := writeConfig(t, "path = /x\n", nil)
	if err := os.Chmod(file, 0o000); err != nil {
		t.Fatal(err)
	}

	_, err := Load(opts)
	if err == nil {
		t.Fatal("Load() error = nil, want an error for a file that cannot be read")
	}
	if !strings.Contains(err.Error(), file) {
		t.Errorf("error = %q, want it to name %q", err, file)
	}
}

func TestBadConfigFiles(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		want     string // a fragment the error must contain
	}{
		{"no equals sign", "path /x\n", "key = value"},
		{"unknown setting", "colour = blue\n", `unknown setting "colour"`},
		{"empty value", "path =\n", "no value"},
		{"empty value with a comment", "path = # todo\n", "no value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, file := writeConfig(t, tt.contents, nil)

			_, err := Load(opts)
			if err == nil {
				t.Fatalf("Load() error = nil, want an error for %q", tt.contents)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to contain %q", err, tt.want)
			}
			if !strings.Contains(err.Error(), file+":1") {
				t.Errorf("error = %q, want it to point at %s line 1", err, file)
			}
		})
	}
}

func TestConfigFileIgnoresCommentsAndBlankLines(t *testing.T) {
	opts, _ := writeConfig(t, "\n# a comment\n\n   \npath = /x   # trailing comment\n", nil)

	cfg, err := Load(opts)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Path != "/x" {
		t.Errorf("Path = %q, want /x", cfg.Path)
	}
}

func TestDefaultPathPrefersALocalCheatsDirectory(t *testing.T) {
	work := t.TempDir()
	if err := os.Mkdir(filepath.Join(work, CheatDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	opts := Options{Getenv: env(nil), Home: t.TempDir(), WorkDir: work}

	if want := filepath.Join(work, CheatDirName); DefaultPath(opts) != want {
		t.Errorf("DefaultPath() = %q, want %q", DefaultPath(opts), want)
	}
}

func TestDefaultPathIgnoresACheatsFileThatIsNotADirectory(t *testing.T) {
	work := t.TempDir()
	if err := os.WriteFile(filepath.Join(work, CheatDirName), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := Options{Getenv: env(nil), Home: t.TempDir(), WorkDir: work}

	if got, want := DefaultPath(opts), UserPath(opts); got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestXDGVariablesAreHonoured(t *testing.T) {
	opts := Options{
		Getenv: env(map[string]string{
			"XDG_CONFIG_HOME": "/cfg",
			"XDG_DATA_HOME":   "/data",
		}),
		Home:    t.TempDir(),
		WorkDir: t.TempDir(),
	}

	if got, want := ConfigFile(opts), filepath.Join("/cfg", AppDir, FileName); got != want {
		t.Errorf("ConfigFile() = %q, want %q", got, want)
	}
	if got, want := UserPath(opts), filepath.Join("/data", AppDir, CheatDirName); got != want {
		t.Errorf("UserPath() = %q, want %q", got, want)
	}
}

func TestOptionsFallBackToTheRealEnvironment(t *testing.T) {
	// The zero Options must work, since that is what nav itself passes.
	t.Setenv(EnvPath, "/from/the/real/environment")
	// Point at a file that does not exist, so whatever configuration the
	// machine running the test happens to have cannot affect it.
	t.Setenv(EnvConfig, filepath.Join(t.TempDir(), "absent"))

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Path != "/from/the/real/environment" {
		t.Errorf("Path = %q, want the value from the real environment", cfg.Path)
	}
}
