package main

import (
	"fmt"
	"net/http"
	"time"
)

func (client connectorClient) pollLoop() {
	for {
		var envelope taskEnvelope
		err := client.doJSON(http.MethodGet, "/api/advanced-chat/connectors/tasks/next", nil, &envelope)
		if err != nil {
			fmt.Printf("poll failed: %v\n", err)
			time.Sleep(3 * time.Second)
			continue
		}
		if envelope.Task == nil || envelope.Task.ID == "" {
			continue
		}
		fmt.Printf("\nTask %s: %s\n", envelope.Task.ID, envelope.Task.Action)
		result, execErr := client.executeTask(*envelope.Task)
		payload := result.payload(execErr == nil)
		if execErr != nil {
			payload["error"] = execErr.Error()
			fmt.Printf("Task failed: %v\n", execErr)
		} else {
			fmt.Println("Task completed")
		}
		path := "/api/advanced-chat/connectors/tasks/" + envelope.Task.ID + "/result"
		if err := client.doJSON(http.MethodPost, path, payload, nil); err != nil {
			fmt.Printf("result upload failed: %v\n", err)
		}
	}
}

type taskResult struct {
	Result   string
	Output   string
	Stdout   string
	Stderr   string
	ExitCode *int
}

func textTaskResult(text string) taskResult {
	return taskResult{Result: text}
}

func commandTaskResult(result commandResult) taskResult {
	return taskResult{
		Result:   result.Result,
		Output:   result.Output,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
	}
}

func (result taskResult) payload(success bool) map[string]interface{} {
	payload := map[string]interface{}{
		"success": success,
		"result":  result.Result,
	}
	if result.Output != "" {
		payload["output"] = result.Output
	}
	if result.Stdout != "" {
		payload["stdout"] = result.Stdout
	}
	if result.Stderr != "" {
		payload["stderr"] = result.Stderr
	}
	if result.ExitCode != nil {
		payload["exit_code"] = *result.ExitCode
	}
	return payload
}

func (client connectorClient) executeTask(task connectorTask) (taskResult, error) {
	workspace, err := client.workspaceRoot(task.WorkspacePath)
	if err != nil {
		return taskResult{}, err
	}
	switch task.Action {
	case "list_files":
		result, err := listFiles(workspace, stringArg(task.Payload, "path"), intArg(task.Payload, "max_entries", 100))
		return textTaskResult(result), err
	case "read_file":
		result, err := readFile(workspace, stringArg(task.Payload, "path"), intArg(task.Payload, "max_bytes", 120000))
		return textTaskResult(result), err
	case "write_file":
		result, err := writeFile(workspace, stringArg(task.Payload, "path"), stringArg(task.Payload, "content"), boolArg(task.Payload, "overwrite"), boolArg(task.Payload, "create_dirs"))
		return textTaskResult(result), err
	case "replace_text":
		result, err := replaceText(workspace, stringArg(task.Payload, "path"), stringArg(task.Payload, "old_text"), stringArg(task.Payload, "new_text"))
		return textTaskResult(result), err
	case "run_command":
		result, err := runCommand(workspace, stringArg(task.Payload, "command"), intArg(task.Payload, "timeout_sec", 30))
		return commandTaskResult(result), err
	default:
		return taskResult{}, fmt.Errorf("unsupported action %q", task.Action)
	}
}
