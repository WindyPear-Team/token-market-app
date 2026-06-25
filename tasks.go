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
		payload := map[string]interface{}{"success": execErr == nil, "result": result}
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

func (client connectorClient) executeTask(task connectorTask) (string, error) {
	workspace, err := client.workspaceRoot(task.WorkspacePath)
	if err != nil {
		return "", err
	}
	switch task.Action {
	case "list_files":
		return listFiles(workspace, stringArg(task.Payload, "path"), intArg(task.Payload, "max_entries", 100))
	case "read_file":
		return readFile(workspace, stringArg(task.Payload, "path"), intArg(task.Payload, "max_bytes", 120000))
	case "write_file":
		return writeFile(workspace, stringArg(task.Payload, "path"), stringArg(task.Payload, "content"), boolArg(task.Payload, "overwrite"), boolArg(task.Payload, "create_dirs"))
	case "replace_text":
		return replaceText(workspace, stringArg(task.Payload, "path"), stringArg(task.Payload, "old_text"), stringArg(task.Payload, "new_text"))
	case "run_command":
		return runCommand(workspace, stringArg(task.Payload, "command"), intArg(task.Payload, "timeout_sec", 30))
	default:
		return "", fmt.Errorf("unsupported action %q", task.Action)
	}
}
