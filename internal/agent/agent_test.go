package agent

import "testing"

func TestNewAgentUsesGLM52CloudByDefault(t *testing.T) {
	agent, err := NewAgent(nil, "http://localhost")
	if err != nil {
		t.Fatal(err)
	}
	if agent.Model() != "glm-5.2:cloud" {
		t.Fatalf("Model = %q, want glm-5.2:cloud", agent.Model())
	}
}
