package engine

import (
	"encoding/json"
	"testing"
)

func TestApprovalSignatureKeepsShellCommandsExact(t *testing.T) {
	first := ApprovalSignature("shell.run", json.RawMessage(`{"command":"npm run build"}`))
	second := ApprovalSignature("shell.run", json.RawMessage(`{"command":"npm run deploy"}`))
	if first != "npm run build" || second != "npm run deploy" || first == second {
		t.Fatalf("remembered command signatures were not exact: %q, %q", first, second)
	}
}

func TestApprovalSignatureTrimsCommandWhitespaceAndKeepsExactFilePath(t *testing.T) {
	command := ApprovalSignature("shell.run", json.RawMessage(`{"command":"  npm test  "}`))
	path := ApprovalSignature("file.write", json.RawMessage(`{"path":"src/main.ts"}`))
	if command != "npm test" || path != "src/main.ts" {
		t.Fatalf("signatures = command %q, path %q", command, path)
	}
}
