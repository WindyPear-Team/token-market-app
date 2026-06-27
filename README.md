# Token Market App Connector

Local connector for WindyPear advanced chat assistant mode.

The connector registers a local device with the backend, polls for approved
tool tasks, and sends task results back to the backend. The workspace directory
is selected in the web session settings.

## Run

Generate a connector token in Advanced Chat > Devices, then run:

```powershell
go run . -server http://localhost:8080 -token <connector-token>
```

## Permissions

The connector only handles tasks for the absolute workspace path selected in
the web session settings, and verifies that directory exists locally before any
tool runs. Paths from the model must be relative to the workspace root.

Read-only actions run directly:

- `list_files`
- `read_file`
- `web_search`

Editing actions require approval in the web frontend before the connector can
receive the task, unless automatic approval is enabled for the chat session:

- `write_file`
- `replace_text`

Command execution always requires approval unless the full command starts with
one of the prefixes allowed in the chat session settings:

- `run_command`

## Build

```powershell
go build -o token-market-app.exe .
```
