package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

const verifyProtocolVersion = 1

// VerifyStep records one ACP lifecycle check.
type VerifyStep struct {
	Method     string        `json:"method"`
	Passed     bool          `json:"passed"`
	Duration   time.Duration `json:"duration"`
	Detail     string        `json:"detail,omitempty"`
	JSONRPCErr *JSONRPCError `json:"jsonrpc_error,omitempty"`
}

// VerifyResult describes the observed ACP contract for a runtime.
type VerifyResult struct {
	Transport       string         `json:"transport"`
	Command         string         `json:"command"`
	Args            []string       `json:"args,omitempty"`
	WorkingDir      string         `json:"working_dir,omitempty"`
	ProtocolVersion int            `json:"protocol_version"`
	SessionID       string         `json:"session_id,omitempty"`
	Capabilities    map[string]any `json:"capabilities,omitempty"`
	Steps           []VerifyStep   `json:"steps"`
	Stderr          string         `json:"stderr,omitempty"`
}

// VerifyOptions controls ACP runtime verification.
type VerifyOptions struct {
	Command    string
	Args       []string
	WorkingDir string
	Prompt     string
}

// VerifyRuntime executes the ACP handshake against a runtime over JSON-RPC/stdin.
func VerifyRuntime(ctx context.Context, opts VerifyOptions) (*VerifyResult, error) {
	if strings.TrimSpace(opts.Command) == "" {
		return nil, fmt.Errorf("command is required")
	}

	cmd := exec.CommandContext(ctx, opts.Command, opts.Args...)
	if opts.WorkingDir != "" {
		cmd.Dir = opts.WorkingDir
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting runtime: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	var stderrBuf strings.Builder
	stderrDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&stderrBuf, stderr)
		close(stderrDone)
	}()

	reader := bufio.NewReader(stdout)
	writer := json.NewEncoder(stdin)

	result := &VerifyResult{
		Transport:  "jsonrpc-stdio",
		Command:    opts.Command,
		Args:       append([]string(nil), opts.Args...),
		WorkingDir: opts.WorkingDir,
	}

	initResp, step, err := verifyRPCStep(ctx, writer, reader, &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params:  mustRaw(`{"protocolVersion":1}`),
	})
	result.Steps = append(result.Steps, step)
	if err != nil {
		<-stderrDone
		result.Stderr = strings.TrimSpace(stderrBuf.String())
		return result, err
	}

	var initResult struct {
		ProtocolVersion int            `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities"`
	}
	if err := json.Unmarshal(initResp.Result, &initResult); err != nil {
		<-stderrDone
		result.Stderr = strings.TrimSpace(stderrBuf.String())
		return result, fmt.Errorf("parsing initialize result: %w", err)
	}
	result.ProtocolVersion = initResult.ProtocolVersion
	result.Capabilities = initResult.Capabilities
	if result.ProtocolVersion < verifyProtocolVersion {
		<-stderrDone
		result.Stderr = strings.TrimSpace(stderrBuf.String())
		return result, fmt.Errorf("runtime protocol version %d is below required %d", result.ProtocolVersion, verifyProtocolVersion)
	}

	sessionResp, step, err := verifyRPCStep(ctx, writer, reader, &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "session/new",
	})
	result.Steps = append(result.Steps, step)
	if err != nil {
		<-stderrDone
		result.Stderr = strings.TrimSpace(stderrBuf.String())
		return result, err
	}

	var sessionResult SessionNewResult
	if err := json.Unmarshal(sessionResp.Result, &sessionResult); err != nil {
		<-stderrDone
		result.Stderr = strings.TrimSpace(stderrBuf.String())
		return result, fmt.Errorf("parsing session/new result: %w", err)
	}
	result.SessionID = strings.TrimSpace(sessionResult.SessionID)
	if result.SessionID == "" {
		<-stderrDone
		result.Stderr = strings.TrimSpace(stderrBuf.String())
		return result, fmt.Errorf("session/new did not return a sessionId")
	}

	if strings.TrimSpace(opts.Prompt) != "" {
		promptParams := map[string]any{
			"sessionId": result.SessionID,
			"prompt": []map[string]string{
				{
					"type": "text",
					"text": opts.Prompt,
				},
			},
		}
		promptPayload, err := json.Marshal(promptParams)
		if err != nil {
			<-stderrDone
			result.Stderr = strings.TrimSpace(stderrBuf.String())
			return result, fmt.Errorf("encoding session/prompt params: %w", err)
		}
		_, step, err = verifyRPCStep(ctx, writer, reader, &JSONRPCMessage{
			JSONRPC: "2.0",
			ID:      3,
			Method:  "session/prompt",
			Params:  promptPayload,
		})
		result.Steps = append(result.Steps, step)
		if err != nil {
			<-stderrDone
			result.Stderr = strings.TrimSpace(stderrBuf.String())
			return result, err
		}
	}

	_ = stdin.Close()
	<-stderrDone
	result.Stderr = strings.TrimSpace(stderrBuf.String())
	return result, nil
}

func verifyRPCStep(ctx context.Context, writer *json.Encoder, reader *bufio.Reader, req *JSONRPCMessage) (*JSONRPCMessage, VerifyStep, error) {
	start := time.Now()
	step := VerifyStep{Method: req.Method}

	if err := writer.Encode(req); err != nil {
		step.Duration = time.Since(start)
		step.Detail = err.Error()
		return nil, step, fmt.Errorf("sending %s: %w", req.Method, err)
	}

	msg, err := readVerifyResponse(ctx, reader)
	step.Duration = time.Since(start)
	if err != nil {
		step.Detail = err.Error()
		return nil, step, fmt.Errorf("reading %s response: %w", req.Method, err)
	}
	if msg.JSONRPC != "2.0" {
		step.Detail = "response did not declare jsonrpc=2.0"
		return nil, step, fmt.Errorf("%s response did not declare jsonrpc=2.0", req.Method)
	}
	if msg.Error != nil {
		step.JSONRPCErr = msg.Error
		step.Detail = msg.Error.Message
		return nil, step, fmt.Errorf("%s failed: %s", req.Method, msg.Error.Message)
	}

	step.Passed = true
	step.Detail = "ok"
	return msg, step, nil
}

func readVerifyResponse(ctx context.Context, reader *bufio.Reader) (*JSONRPCMessage, error) {
	type response struct {
		msg *JSONRPCMessage
		err error
	}
	done := make(chan response, 1)
	go func() {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			done <- response{err: err}
			return
		}
		var msg JSONRPCMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			done <- response{err: err}
			return
		}
		done <- response{msg: &msg}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-done:
		return res.msg, res.err
	}
}

func mustRaw(input string) json.RawMessage {
	return json.RawMessage(input)
}
