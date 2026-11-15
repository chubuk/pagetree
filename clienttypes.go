package pagetree

// ClientType — конфигурационный шаблон MCP-клиента (статический, без БД).
// Тексты шаблонов — 1:1 с сидом mcp-is (mcp_client_type_repo.go): плейсхолдеры
// {{KEY_NAME}}, {{URL}}, {{API_KEY}} заменяются на клиенте.
type ClientType struct {
	Name           string `json:"name"`
	ConfigTemplate string `json:"config_template"`
	FileName       string `json:"file_name"`       // "" = без файла (только hint)
	FileDescription string `json:"file_description"`
}

// clientTypes — порядок = порядок отображения в UI.
var clientTypes = []ClientType{
	{
		Name: "Claude Desktop",
		FileName:        "claude_desktop_config.json",
		FileDescription: `~/Library/Application Support/Claude/ (macOS) или %APPDATA%\Claude\ (Windows)`,
		ConfigTemplate: `{
  "mcpServers": {
    "{{KEY_NAME}}": {
      "url": "{{URL}}",
      "headers": {
        "Authorization": "Bearer {{API_KEY}}"
      }
    }
  }
}`,
	},
	{
		Name: "Cursor",
		FileName:        "mcp.json",
		FileDescription: "~/.cursor/mcp.json",
		ConfigTemplate: `{
  "mcpServers": {
    "{{KEY_NAME}}": {
      "url": "{{URL}}",
      "headers": {
        "Authorization": "Bearer {{API_KEY}}"
      }
    }
  }
}`,
	},
	{
		Name: "VS Code (Copilot)",
		FileName:        "mcp.json",
		FileDescription: ".vscode/mcp.json (в проекте)",
		ConfigTemplate: `{
  "servers": {
    "{{KEY_NAME}}": {
      "type": "http",
      "url": "{{URL}}",
      "headers": {
        "Authorization": "Bearer {{API_KEY}}"
      }
    }
  }
}`,
	},
	{
		Name: "MiMoCode",
		FileName:        "mcp.json",
		FileDescription: ".mimocode/mcp.json (в проекте)",
		ConfigTemplate: `{
  "mcp": {
    "{{KEY_NAME}}": {
      "type": "remote",
      "url": "{{URL}}",
      "headers": {
        "Authorization": "Bearer {{API_KEY}}"
      }
    }
  }
}`,
	},
	{
		Name: "OpenCode",
		FileName:        "config.json",
		FileDescription: ".opencode/config.json",
		ConfigTemplate: `{
  "mcpServers": {
    "{{KEY_NAME}}": {
      "url": "{{URL}}",
      "headers": {
        "Authorization": "Bearer {{API_KEY}}"
      }
    }
  }
}`,
	},
	{
		Name: "LibreChat",
		FileName:        "librechat.yaml",
		FileDescription: "корень проекта LibreChat",
		ConfigTemplate: `mcpServers:
  {{KEY_NAME}}:
    type: streamable-http
    url: {{URL}}
    headers:
      Authorization: Bearer {{API_KEY}}`,
	},
	{
		Name: "Open WebUI",
		FileName:        "",
		FileDescription: "Через Admin Panel → Settings → MCP",
		ConfigTemplate: `{
  "mcpServers": {
    "{{KEY_NAME}}": {
      "url": "{{URL}}",
      "headers": {
        "Authorization": "Bearer {{API_KEY}}"
      }
    }
  }
}`,
	},
	{
		Name: "Cline",
		FileName:        "cline_mcp_settings.json",
		FileDescription: `%APPDATA%\cline\ (Windows) или ~/.config/cline/ (Linux/macOS)`,
		ConfigTemplate: `{
  "mcpServers": {
    "{{KEY_NAME}}": {
      "url": "{{URL}}",
      "headers": {
        "Authorization": "Bearer {{API_KEY}}"
      }
    }
  }
}`,
	},
	{
		Name: "Cherry Studio",
		FileName:        "",
		FileDescription: "Через Settings → MCP Servers",
		ConfigTemplate: `{
  "mcpServers": {
    "{{KEY_NAME}}": {
      "url": "{{URL}}",
      "headers": {
        "Authorization": "Bearer {{API_KEY}}"
      }
    }
  }
}`,
	},
	{
		Name: "Continue",
		FileName:        "config.json",
		FileDescription: "~/.continue/config.json",
		ConfigTemplate: `{
  "mcpServers": {
    "{{KEY_NAME}}": {
      "url": "{{URL}}",
      "headers": {
        "Authorization": "Bearer {{API_KEY}}"
      }
    }
  }
}`,
	},
}

// GetClientTypes returns config templates for supported MCP clients.
func GetClientTypes() []ClientType { return clientTypes }
