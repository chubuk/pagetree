package pagetree

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestClientTypesContract: 10 шаблонов, обязательные плейсхолдеры, файлы у кого положено.
func TestClientTypesContract(t *testing.T) {
	list := GetClientTypes()
	if len(list) != 10 {
		t.Fatalf("len(clientTypes) = %d, want 10", len(list))
	}
	seen := map[string]bool{}
	for _, ct := range list {
		if ct.Name == "" {
			t.Error("пустой name")
		}
		if seen[ct.Name] {
			t.Errorf("дубль name %q", ct.Name)
		}
		seen[ct.Name] = true
		for _, ph := range []string{"{{KEY_NAME}}", "{{URL}}", "{{API_KEY}}"} {
			if !strings.Contains(ct.ConfigTemplate, ph) {
				t.Errorf("%s: шаблон без %s", ct.Name, ph)
			}
		}
		if ct.FileDescription == "" {
			t.Errorf("%s: пустой file_description", ct.Name)
		}
	}
	// Файловые/безфайловые (контракт с mcp-is сидом)
	for _, name := range []string{"Open WebUI", "Cherry Studio"} {
		for _, ct := range list {
			if ct.Name == name && ct.FileName != "" {
				t.Errorf("%s: file_name должен быть пустым", name)
			}
		}
	}
	for _, name := range []string{"Claude Desktop", "Cursor", "VS Code (Copilot)", "MiMoCode", "OpenCode", "LibreChat", "Cline", "Continue"} {
		found := false
		for _, ct := range list {
			if ct.Name == name {
				found = true
				if ct.FileName == "" {
					t.Errorf("%s: file_name не должен быть пустым", name)
				}
			}
		}
		if !found {
			t.Errorf("нет клиента %q", name)
		}
	}
}

// TestClientTypesJSONKeys: точные snake_case-ключи ответа API.
func TestClientTypesJSONKeys(t *testing.T) {
	b, err := json.Marshal(GetClientTypes()[0])
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	want := []string{"name", "config_template", "file_name", "file_description"}
	for _, k := range want {
		if _, ok := m[k]; !ok {
			t.Errorf("нет JSON-ключа %q в ответе: %s", k, b)
		}
	}
	if len(m) != len(want) {
		t.Errorf("лишние ключи в ответе: %v", m)
	}
}
