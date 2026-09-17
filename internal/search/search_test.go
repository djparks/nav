package search

import (
	"testing"

	"nav/internal/cheat"
)

// sample cheats used across the tests. Each field carries a distinctive word
// so a test can prove which field a match came from:
//
//	tags        "git", "branch"
//	description "Show the current branch name"
//	command     "git rev-parse --abbrev-ref HEAD"
var (
	gitBranch = cheat.Cheat{
		Tags:        []string{"git", "branch"},
		Description: "Show the current branch name",
		Command:     "git rev-parse --abbrev-ref HEAD",
	}
	dockerPs = cheat.Cheat{
		Tags:        []string{"docker", "containers"},
		Description: "List running containers",
		Command:     `docker ps --format "<format>"`,
	}
	kubectlPods = cheat.Cheat{
		Tags:        []string{"kubernetes"},
		Description: "List pods in a namespace",
		Command:     "kubectl get pods -n <namespace>",
	}
	all = []cheat.Cheat{gitBranch, dockerPs, kubectlPods}
)

func TestFilterByTag(t *testing.T) {
	// "kubernetes" appears only in the tags of kubectlPods.
	got := Filter(all, "kubernetes")
	if len(got) != 1 || got[0].Command != kubectlPods.Command {
		t.Fatalf("got %d matches (%v), want just the kubectl cheat", len(got), got)
	}
}

func TestFilterByDescription(t *testing.T) {
	// "running" appears only in dockerPs's description.
	got := Filter(all, "running")
	if len(got) != 1 || got[0].Command != dockerPs.Command {
		t.Fatalf("got %d matches (%v), want just the docker ps cheat", len(got), got)
	}
}

func TestFilterByCommandText(t *testing.T) {
	// "rev-parse" appears only in gitBranch's command.
	got := Filter(all, "rev-parse")
	if len(got) != 1 || got[0].Command != gitBranch.Command {
		t.Fatalf("got %d matches (%v), want just the git branch cheat", len(got), got)
	}
}

func TestFilterIsCaseInsensitive(t *testing.T) {
	for _, q := range []string{"DOCKER", "Docker", "dOcKeR"} {
		if got := Filter(all, q); len(got) != 1 {
			t.Errorf("Filter(%q) returned %d matches, want 1", q, len(got))
		}
	}
}

func TestFilterRequiresEveryTerm(t *testing.T) {
	// Both terms are present, in different fields: "docker" in the tags,
	// "format" in the command.
	if got := Filter(all, "docker format"); len(got) != 1 {
		t.Errorf(`Filter("docker format") returned %d matches, want 1`, len(got))
	}
	// "docker" matches, "zebra" does not, so the cheat is excluded.
	if got := Filter(all, "docker zebra"); len(got) != 0 {
		t.Errorf(`Filter("docker zebra") returned %d matches, want 0`, len(got))
	}
}

func TestFilterIgnoresTermOrder(t *testing.T) {
	a := Filter(all, "list containers")
	b := Filter(all, "containers list")
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("got %d and %d matches, want 1 each", len(a), len(b))
	}
}

func TestFilterEmptyQueryMatchesEverything(t *testing.T) {
	for _, q := range []string{"", "   ", "\t"} {
		got := Filter(all, q)
		if len(got) != len(all) {
			t.Errorf("Filter(%q) returned %d matches, want all %d", q, len(got), len(all))
		}
	}
	if !Parse("  ").IsEmpty() {
		t.Error("Parse(\"  \").IsEmpty() = false, want true")
	}
}

func TestFilterPreservesOrder(t *testing.T) {
	got := Filter(all, "list")
	if len(got) != 2 {
		t.Fatalf("got %d matches, want 2", len(got))
	}
	if got[0].Command != dockerPs.Command || got[1].Command != kubectlPods.Command {
		t.Error("matches are not in their original order")
	}
}

func TestFilterNoMatchReturnsEmptyNotNil(t *testing.T) {
	got := Filter(all, "definitely-not-present")
	if got == nil {
		t.Fatal("Filter returned nil, want an empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("got %d matches, want 0", len(got))
	}
}

func TestFieldScopedTerms(t *testing.T) {
	tests := []struct {
		query string
		want  int
		why   string
	}{
		// "git" is in gitBranch's tags and also in its command, so an
		// unscoped term and a tag-scoped term both find it.
		{"git", 1, "unscoped git"},
		{"tag:git", 1, "git is a tag"},
		{"tags:git", 1, "tags: is accepted too"},
		// "rev-parse" is only in the command, so scoping to tags finds
		// nothing even though the word exists in the cheat.
		{"tag:rev-parse", 0, "rev-parse is not a tag"},
		{"cmd:rev-parse", 1, "rev-parse is in the command"},
		{"command:rev-parse", 1, "command: is accepted too"},
		// "current" is only in the description.
		{"desc:current", 1, "current is in the description"},
		{"cmd:current", 0, "current is not in the command"},
		{"description:current", 1, "description: is accepted too"},
		// Scoped and unscoped terms combine.
		{"tag:git branch", 1, "both terms match the git cheat"},
		{"tag:docker rev-parse", 0, "no single cheat satisfies both"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			if got := Filter(all, tt.query); len(got) != tt.want {
				t.Errorf("Filter(%q) returned %d matches, want %d (%s)",
					tt.query, len(got), tt.want, tt.why)
			}
		})
	}
}

func TestBarePrefixIsIgnored(t *testing.T) {
	// "tag:" with nothing after it restricts nothing, so it is dropped and
	// the query behaves as if it were empty.
	q := Parse("tag:")
	if !q.IsEmpty() {
		t.Error("Parse(\"tag:\").IsEmpty() = false, want true")
	}
	if got := Filter(all, "tag:"); len(got) != len(all) {
		t.Errorf("Filter(\"tag:\") returned %d matches, want all %d", len(got), len(all))
	}
}

func TestQueryReuse(t *testing.T) {
	// A parsed query can be applied more than once, which is what the
	// interactive selector relies on.
	q := Parse("docker")
	first := q.Filter(all)
	second := q.Filter(all)
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("got %d and %d matches, want 1 each", len(first), len(second))
	}
	if !q.Matches(dockerPs) {
		t.Error("Matches(dockerPs) = false, want true")
	}
	if q.Matches(gitBranch) {
		t.Error("Matches(gitBranch) = true, want false")
	}
}
