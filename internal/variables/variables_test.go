package variables

import (
	"reflect"
	"testing"
)

func TestNames(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    []string
	}{
		{
			name:    "no variables",
			command: "docker ps -a",
			want:    nil,
		},
		{
			name:    "one variable",
			command: `docker ps --format "<format>"`,
			want:    []string{"format"},
		},
		{
			name:    "several variables in order of appearance",
			command: "git log -n <count> <branch>",
			want:    []string{"count", "branch"},
		},
		{
			name:    "a repeated variable is reported once",
			command: "cp <file> <file>.bak",
			want:    []string{"file"},
		},
		{
			name:    "order follows first appearance, not repeats",
			command: "diff <b> <a> <b>",
			want:    []string{"b", "a"},
		},
		{
			name:    "underscores and hyphens are part of a name",
			command: "kubectl -n <name_space> --context <my-ctx> get pods",
			want:    []string{"name_space", "my-ctx"},
		},
		{
			name:    "a variable can be glued to other text",
			command: "ssh user@<host>:<port>",
			want:    []string{"host", "port"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Names(tt.command); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Names(%q) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}

// Shell syntax contains plenty of angle brackets that are not variables.
// Mistaking one for a variable would make nav prompt for nonsense, so these
// are worth pinning down.
func TestNamesIgnoresShellSyntax(t *testing.T) {
	commands := []string{
		"sort < input.txt",
		"command > output.txt 2>&1",
		"diff <(sort a) <(sort b)",
		"[ $a -lt $b ]",
		"echo a <> b",
		"tr a-z A-Z < in > out",
		// A name may not start with a digit or hyphen.
		"echo <1> <-x>",
		// Brackets with a space in them are not a name.
		"echo < format >",
		// An unclosed bracket is not a variable.
		"echo <format",
	}

	for _, command := range commands {
		if got := Names(command); got != nil {
			t.Errorf("Names(%q) = %v, want none", command, got)
		}
	}
}

func TestSubstitute(t *testing.T) {
	tests := []struct {
		name    string
		command string
		values  map[string]string
		want    string
	}{
		{
			name:    "single variable",
			command: `docker ps --format "<format>"`,
			values:  map[string]string{"format": "json"},
			want:    `docker ps --format "json"`,
		},
		{
			name:    "several variables",
			command: "git log -n <count> <branch>",
			values:  map[string]string{"count": "5", "branch": "main"},
			want:    "git log -n 5 main",
		},
		{
			name:    "every occurrence of a repeated variable is replaced",
			command: "cp <file> <file>.bak",
			values:  map[string]string{"file": "notes.txt"},
			want:    "cp notes.txt notes.txt.bak",
		},
		{
			name:    "an unknown placeholder is left alone",
			command: "git log -n <count> <branch>",
			values:  map[string]string{"count": "5"},
			want:    "git log -n 5 <branch>",
		},
		{
			name:    "no values leaves the command untouched",
			command: "git log -n <count>",
			values:  nil,
			want:    "git log -n <count>",
		},
		{
			name:    "a command with no variables is untouched",
			command: "docker ps -a",
			values:  map[string]string{"format": "json"},
			want:    "docker ps -a",
		},
		{
			name:    "an empty value erases the placeholder",
			command: "ls <flags> .",
			values:  map[string]string{"flags": ""},
			want:    "ls  .",
		},
		{
			name:    "a value containing angle brackets is not re-scanned",
			command: "echo <text>",
			values:  map[string]string{"text": "<other>"},
			want:    "echo <other>",
		},
		{
			name:    "a value may contain the delimiters a list format would need",
			command: `docker ps --format "<format>"`,
			values:  map[string]string{"format": "table {{.Names}}|{{.Status}}, plus"},
			want:    `docker ps --format "table {{.Names}}|{{.Status}}, plus"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Substitute(tt.command, tt.values); got != tt.want {
				t.Errorf("Substitute(%q, %v) = %q, want %q", tt.command, tt.values, got, tt.want)
			}
		})
	}
}

func TestIsValidName(t *testing.T) {
	valid := []string{"format", "f", "_x", "my-ctx", "name_space", "a1", "A"}
	for _, s := range valid {
		if !IsValidName(s) {
			t.Errorf("IsValidName(%q) = false, want true", s)
		}
	}

	invalid := []string{"", "1abc", "-x", "a b", "a:b", "a.b", "<a>", "a\n"}
	for _, s := range invalid {
		if IsValidName(s) {
			t.Errorf("IsValidName(%q) = true, want false", s)
		}
	}
}

// The syntax accepted by IsValidName and the syntax Names finds must agree,
// or the cheatsheet parser would accept a `$ name:` block for a variable
// that can never be detected in a command.
func TestNamesAndIsValidNameAgree(t *testing.T) {
	for _, name := range []string{"format", "_x", "my-ctx", "name_space", "a1"} {
		if !IsValidName(name) {
			t.Fatalf("IsValidName(%q) = false", name)
		}
		got := Names("cmd <" + name + ">")
		if len(got) != 1 || got[0] != name {
			t.Errorf("Names found %v for the valid name %q, want just it", got, name)
		}
	}
}
