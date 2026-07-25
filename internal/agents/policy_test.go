package agents

import "testing"

func TestAssessCommandDetectsCriticalOperations(t *testing.T) {
	assessment := AssessCommand("git reset --hard HEAD", "/tmp/project")
	if assessment.Level != "critical" {
		t.Fatalf("expected critical risk, got %q", assessment.Level)
	}
	if assessment.Reversible {
		t.Fatal("critical destructive command must not be marked reversible")
	}
}

func TestAssessCommandFlagsOutsideProjectPaths(t *testing.T) {
	assessment := AssessCommand("cp ./config /etc/example", "/tmp/project")
	if !assessment.OutsideProject {
		t.Fatal("expected outside-project path warning")
	}
}
